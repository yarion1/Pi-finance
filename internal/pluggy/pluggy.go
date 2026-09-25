// Package pluggy fala com a API da Pluggy (Meu Pluggy, Open Finance gratuito para
// uso pessoal). Cada pessoa cria a própria conta em meu.pluggy.ai, conecta os
// bancos lá e cola no painel o Client ID, o Client Secret e o id de cada item
// (banco conectado). Aqui só há leitura: nenhuma senha de banco passa pelo painel.
//
// Valores chegam como número JSON e viram centavos inteiros sem passar por float.
package pluggy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/importadores"
)

// URLPadrao é a API de produção; PLUGGY_URL troca (testes e e2e usam uma falsa).
const URLPadrao = "https://api.pluggy.ai"

var (
	// ErrCredenciais: Client ID ou Secret recusados.
	ErrCredenciais = errors.New("credenciais do Meu Pluggy recusadas")
	// ErrItemNaoEncontrado: o item não existe para essas credenciais.
	ErrItemNaoEncontrado = errors.New("item não encontrado no Meu Pluggy")
	// ErrIndisponivel: a Pluggy não respondeu ou respondeu algo inesperado.
	ErrIndisponivel = errors.New("Meu Pluggy indisponível")
	// ErrRecusada: a Pluggy recusou o pedido (400); a mensagem dela vem junto.
	ErrRecusada = errors.New("o Meu Pluggy recusou o pedido")
)

var fusoSP = func() *time.Location {
	l, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return l
}()

// Cliente da API.
type Cliente struct {
	Base string
	HTTP *http.Client
}

// Novo cliente; base vazia usa a produção.
func Novo(base string) *Cliente {
	if base == "" {
		base = URLPadrao
	}
	return &Cliente{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Autenticar troca as credenciais por uma chave de API (vale 2 h).
func (c *Cliente) Autenticar(ctx context.Context, clientID, clientSecret string) (string, error) {
	corpo, _ := json.Marshal(map[string]string{"clientId": clientID, "clientSecret": clientSecret})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/auth", bytes.NewReader(corpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		APIKey string `json:"apiKey"`
	}
	if err := c.fazer(req, &out); err != nil {
		if errors.Is(err, ErrItemNaoEncontrado) {
			return "", ErrCredenciais
		}
		return "", err
	}
	if out.APIKey == "" {
		return "", ErrCredenciais
	}
	return out.APIKey, nil
}

// Item é um banco conectado.
type Item struct {
	ID            string `json:"id"`
	Status        string `json:"status"` // UPDATED, UPDATING, LOGIN_ERROR, OUTDATED, WAITING_USER_INPUT
	LastUpdatedAt string `json:"lastUpdatedAt"`
	Connector     struct {
		Name string `json:"name"`
	} `json:"connector"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Problema devolve o texto do erro do item, vazio se está tudo bem.
func (i Item) Problema() string {
	switch i.Status {
	case "LOGIN_ERROR":
		return "o banco pediu login de novo: reconecte no Meu Pluggy"
	case "WAITING_USER_INPUT":
		return "o banco está esperando uma confirmação no Meu Pluggy"
	case "OUTDATED":
		if i.Error != nil && i.Error.Message != "" {
			return "desatualizado: " + i.Error.Message
		}
		return "desatualizado: o Meu Pluggy não conseguiu ler o banco"
	}
	return ""
}

// AtualizadoEm é quando a Pluggy leu o banco pela última vez.
func (i Item) AtualizadoEm() *time.Time {
	t, err := time.Parse(time.RFC3339, i.LastUpdatedAt)
	if err != nil {
		return nil
	}
	return &t
}

// Item busca um item pelo id.
func (c *Cliente) Item(ctx context.Context, chave, id string) (Item, error) {
	var out Item
	err := c.get(ctx, chave, "/items/"+url.PathEscape(id), &out)
	return out, err
}

// Conta de um item: conta bancária (BANK) ou cartão (CREDIT).
type Conta struct {
	ID            string      `json:"id"`
	Type          string      `json:"type"`
	Subtype       string      `json:"subtype"`
	Name          string      `json:"name"`
	MarketingName string      `json:"marketingName"`
	Number        string      `json:"number"`
	Balance       json.Number `json:"balance"`
	CurrencyCode  string      `json:"currencyCode"`
	CreditData    *struct {
		CreditLimit      json.Number `json:"creditLimit"`
		BalanceCloseDate string      `json:"balanceCloseDate"`
		BalanceDueDate   string      `json:"balanceDueDate"`
	} `json:"creditData"`
}

// Cartao diz se a conta é cartão de crédito (o sinal dos valores muda).
func (c Conta) Cartao() bool { return strings.EqualFold(c.Type, "CREDIT") }

// NomeExibido: o nome comercial, que é o que a pessoa vê no app do banco.
func (c Conta) NomeExibido() string {
	for _, v := range []string{c.MarketingName, c.Name, c.Number} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return c.ID
}

// TipoConta traduz para os tipos do painel.
func (c Conta) TipoConta() string {
	switch {
	case c.Cartao():
		return "cartao"
	case strings.EqualFold(c.Subtype, "SAVINGS_ACCOUNT"):
		return "poupanca"
	default:
		return "corrente"
	}
}

// Moeda da conta (BRL quando a Pluggy não diz).
func (c Conta) Moeda() string {
	if m := strings.ToUpper(strings.TrimSpace(c.CurrencyCode)); len(m) == 3 {
		return m
	}
	return "BRL"
}

// Saldo no formato do painel: numa conta, o saldo; num cartão, o que se deve
// fica negativo (a Pluggy manda a dívida como número positivo).
func (c Conta) Saldo() (core.Centavos, error) {
	v, err := Centavos(c.Balance)
	if err != nil {
		return 0, err
	}
	if c.Cartao() {
		return -v, nil
	}
	return v, nil
}

// Limite do cartão, se houver.
func (c Conta) Limite() *core.Centavos {
	if c.CreditData == nil {
		return nil
	}
	v, err := Centavos(c.CreditData.CreditLimit)
	if err != nil || v <= 0 {
		return nil
	}
	return &v
}

// DiasDoCartao: dias de fechamento e vencimento da fatura atual, se a Pluggy souber.
func (c Conta) DiasDoCartao() (fechamento, vencimento *int16) {
	if c.CreditData == nil {
		return nil, nil
	}
	dia := func(s string) *int16 {
		d, err := DataSP(s)
		if err != nil {
			return nil
		}
		n := int16(d.Day())
		return &n
	}
	return dia(c.CreditData.BalanceCloseDate), dia(c.CreditData.BalanceDueDate)
}

// Contas de um item.
func (c *Cliente) Contas(ctx context.Context, chave, itemID string) ([]Conta, error) {
	var out struct {
		Results []Conta `json:"results"`
	}
	err := c.get(ctx, chave, "/accounts?"+url.Values{"itemId": {itemID}}.Encode(), &out)
	return out.Results, err
}

// Transacao da Pluggy.
type Transacao struct {
	ID                 string      `json:"id"`
	Description        string      `json:"description"`
	DescriptionRaw     string      `json:"descriptionRaw"`
	Amount             json.Number `json:"amount"`
	Date               string      `json:"date"`
	Type               string      `json:"type"`   // DEBIT (sai) ou CREDIT (entra)
	Status             string      `json:"status"` // POSTED ou PENDING
	CurrencyCode       string      `json:"currencyCode"`
	CreditCardMetadata *struct {
		InstallmentNumber int    `json:"installmentNumber"`
		TotalInstallments int    `json:"totalInstallments"`
		PurchaseDate      string `json:"purchaseDate"`
	} `json:"creditCardMetadata"`
}

// Linha converte para a fila de importação. ok=false para lançamento pendente,
// que ainda pode mudar de valor ou sumir (entra quando for confirmado).
//
// Sinal: numa conta, quem manda é o tipo (DEBIT sai, CREDIT entra); num cartão, a
// Pluggy manda a compra positiva e o pagamento ou estorno negativo.
func (t Transacao) Linha(cartao bool) (importadores.Linha, bool, error) {
	if strings.EqualFold(t.Status, "PENDING") {
		return importadores.Linha{}, false, nil
	}
	v, err := Centavos(t.Amount)
	if err != nil {
		return importadores.Linha{}, false, err
	}
	modulo := v
	if modulo < 0 {
		modulo = -modulo
	}
	var sai bool
	switch {
	case cartao:
		sai = v > 0
	case strings.EqualFold(t.Type, "DEBIT"):
		sai = true
	case strings.EqualFold(t.Type, "CREDIT"):
		sai = false
	default:
		sai = v < 0
	}
	if sai {
		modulo = -modulo
	}

	data, err := DataSP(t.Date)
	if err != nil {
		return importadores.Linha{}, false, err
	}
	descricao := strings.TrimSpace(t.Description)
	if descricao == "" {
		descricao = strings.TrimSpace(t.DescriptionRaw)
	}
	if descricao == "" {
		descricao = "Sem descrição"
	}
	// parcela: "(3/12)" na descrição e a data do mês da parcela (DECISOES D16),
	// para cair na mesma fatura que uma parcela lançada à mão
	if m := t.CreditCardMetadata; m != nil && m.TotalInstallments > 1 && m.InstallmentNumber >= 1 {
		if _, _, _, ok := core.ExtrairParcela(descricao); !ok {
			descricao = fmt.Sprintf("%s (%d/%d)", descricao, m.InstallmentNumber, m.TotalInstallments)
		}
		if compra, err := DataSP(m.PurchaseDate); err == nil {
			data = core.SomarMeses(compra, m.InstallmentNumber-1)
		}
	}
	return importadores.Linha{Data: data, Descricao: descricao, Valor: modulo, IDExterno: t.ID}, true, nil
}

// Transacoes de uma conta desde uma data. Usa a v2 (cursor); se a Pluggy recusar,
// cai para a v1 (páginas numeradas), que vale até 31/12/2026.
func (c *Cliente) Transacoes(ctx context.Context, chave, contaID string, desde time.Time) ([]Transacao, error) {
	todas, err := c.transacoesV2(ctx, chave, contaID, desde)
	if errors.Is(err, ErrRecusada) {
		return c.transacoesV1(ctx, chave, contaID, desde)
	}
	return todas, err
}

func (c *Cliente) transacoesV2(ctx context.Context, chave, contaID string, desde time.Time) ([]Transacao, error) {
	q := url.Values{"accountId": {contaID}, "dateFrom": {desde.Format("2006-01-02")}}
	caminho := "/v2/transactions?" + q.Encode()
	var todas []Transacao
	for range 200 { // limite para um cursor que nunca acaba
		var out struct {
			Results []Transacao `json:"results"`
			Next    string      `json:"next"`
		}
		if err := c.get(ctx, chave, caminho, &out); err != nil {
			return nil, err
		}
		todas = append(todas, out.Results...)
		prox := strings.TrimLeft(strings.TrimSpace(out.Next), "?&")
		if prox == "" {
			return todas, nil
		}
		caminho = "/v2/transactions?" + prox
	}
	return todas, nil
}

func (c *Cliente) transacoesV1(ctx context.Context, chave, contaID string, desde time.Time) ([]Transacao, error) {
	var todas []Transacao
	for pagina := 1; pagina <= 200; pagina++ {
		q := url.Values{"accountId": {contaID}, "from": {desde.Format("2006-01-02")},
			"pageSize": {"500"}, "page": {strconv.Itoa(pagina)}}
		var out struct {
			Results    []Transacao `json:"results"`
			TotalPages int         `json:"totalPages"`
		}
		if err := c.get(ctx, chave, "/transactions?"+q.Encode(), &out); err != nil {
			return nil, err
		}
		todas = append(todas, out.Results...)
		if pagina >= out.TotalPages || len(out.Results) == 0 {
			break
		}
	}
	return todas, nil
}

// Centavos lê um número decimal da API ("-45.9", "1234.567") sem float.
func Centavos(n json.Number) (core.Centavos, error) {
	s := strings.TrimPrefix(strings.TrimSpace(n.String()), "+")
	if strings.ContainsAny(s, "eE") {
		return 0, fmt.Errorf("valor em notação científica: %q", s)
	}
	return core.ParseDecimalPonto(s)
}

// DataSP lê "2026-09-20" ou um instante ISO e devolve o dia em São Paulo.
func DataSP(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if d, err := time.Parse("2006-01-02", s); err == nil {
		return d, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("data inválida: %q", s)
	}
	t = t.In(fusoSP)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}

func (c *Cliente) get(ctx context.Context, chave, caminho string, destino any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+caminho, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", chave)
	return c.fazer(req, destino)
}

// mensagemDeErro tira o "message" do corpo de erro da Pluggy (curto, sem dados da pessoa).
func mensagemDeErro(corpo io.Reader) string {
	var e struct {
		Message string `json:"message"`
	}
	b, _ := io.ReadAll(io.LimitReader(corpo, 4096))
	if json.Unmarshal(b, &e) != nil || strings.TrimSpace(e.Message) == "" {
		return "sem detalhes"
	}
	m := []rune(strings.TrimSpace(e.Message))
	if len(m) > 200 {
		m = append(m[:200], '…')
	}
	return string(m)
}

func (c *Cliente) fazer(req *http.Request, destino any) error {
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrIndisponivel, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrCredenciais
	case resp.StatusCode == http.StatusNotFound:
		return ErrItemNaoEncontrado
	case resp.StatusCode == http.StatusBadRequest:
		return fmt.Errorf("%w: %s", ErrRecusada, mensagemDeErro(resp.Body))
	case resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated:
		return fmt.Errorf("%w: respondeu %d", ErrIndisponivel, resp.StatusCode)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 32<<20))
	dec.UseNumber()
	if err := dec.Decode(destino); err != nil {
		return fmt.Errorf("%w: resposta inesperada (%v)", ErrIndisponivel, err)
	}
	return nil
}
