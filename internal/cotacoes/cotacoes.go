// Package cotacoes busca preços e índices nas fontes públicas (SPEC §5): brapi para
// ações, FIIs, ETFs e BDRs; CoinGecko para cripto; séries do Banco Central (SGS) para
// CDI, IPCA e dólar. O worker grava em cotacoes e indices (tabelas globais, sem dado de
// ninguém: só o código do ativo sai do Pi).
package cotacoes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yarion1/pi-finance/internal/core"
)

// Séries do SGS do Banco Central.
var SeriesBCB = map[string]int{
	"cdi":     12,  // % ao dia
	"ipca":    433, // % no mês
	"usd_brl": 1,   // PTAX venda
}

// Fontes com os endereços (trocados nos testes).
type Fontes struct {
	HTTP       *http.Client
	Brapi      string
	BrapiToken string
	CoinGecko  string
	BCB        string
	Agora      func() time.Time
}

// Padrao: os endereços de produção.
func Padrao(tokenBrapi string) *Fontes {
	return &Fontes{
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		Brapi:      "https://brapi.dev",
		BrapiToken: tokenBrapi,
		CoinGecko:  "https://api.coingecko.com",
		BCB:        "https://api.bcb.gov.br",
	}
}

// Cotacao de um ativo num dia.
type Cotacao struct {
	Codigo string
	Data   time.Time
	Preco  core.Dec8
	Moeda  string
	Fonte  string
}

// Indice de uma série num dia (valor em texto, como o numeric do banco).
type Indice struct {
	Serie string
	Data  time.Time
	Valor string
}

var sp = func() *time.Location {
	l, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return l
}()

func (f *Fontes) agora() time.Time {
	if f.Agora != nil {
		return f.Agora().In(sp)
	}
	return time.Now().In(sp)
}

func diaSP(t time.Time) time.Time {
	t = t.In(sp)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (f *Fontes) obter(ctx context.Context, endereco string, destino any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endereco, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "financas-pi/1")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	corpo, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s respondeu %d", req.URL.Host, resp.StatusCode)
	}
	d := json.NewDecoder(strings.NewReader(string(corpo)))
	d.UseNumber()
	return d.Decode(destino)
}

// Acao busca a cotação de um ticker na brapi (uma requisição por ticker, como o plano
// gratuito permite).
func (f *Fontes) Acao(ctx context.Context, codigo string) (Cotacao, error) {
	q := url.Values{}
	if f.BrapiToken != "" {
		q.Set("token", f.BrapiToken)
	}
	endereco := fmt.Sprintf("%s/api/quote/%s", strings.TrimRight(f.Brapi, "/"), url.PathEscape(codigo))
	if len(q) > 0 {
		endereco += "?" + q.Encode()
	}
	var r struct {
		Results []struct {
			Symbol             string      `json:"symbol"`
			Currency           string      `json:"currency"`
			RegularMarketPrice json.Number `json:"regularMarketPrice"`
			RegularMarketTime  string      `json:"regularMarketTime"`
		} `json:"results"`
	}
	if err := f.obter(ctx, endereco, &r); err != nil {
		return Cotacao{}, err
	}
	if len(r.Results) == 0 || r.Results[0].RegularMarketPrice == "" {
		return Cotacao{}, fmt.Errorf("brapi sem cotação para %s", codigo)
	}
	x := r.Results[0]
	preco, err := core.ParseDec8(x.RegularMarketPrice.String())
	if err != nil || preco <= 0 {
		return Cotacao{}, fmt.Errorf("brapi: preço inválido para %s: %q", codigo, x.RegularMarketPrice)
	}
	data := diaSP(f.agora())
	if t, err := time.Parse(time.RFC3339, x.RegularMarketTime); err == nil {
		data = diaSP(t)
	}
	moeda := strings.ToUpper(x.Currency)
	if len(moeda) != 3 {
		moeda = "BRL"
	}
	return Cotacao{Codigo: codigo, Data: data, Preco: preco, Moeda: moeda, Fonte: "brapi"}, nil
}

// Cripto busca o preço em reais de vários ids da CoinGecko ("bitcoin", "ethereum").
func (f *Fontes) Cripto(ctx context.Context, ids []string) ([]Cotacao, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := url.Values{"ids": {strings.Join(ids, ",")}, "vs_currencies": {"brl"}, "include_last_updated_at": {"true"}}
	var r map[string]struct {
		BRL         json.Number `json:"brl"`
		Atualizacao json.Number `json:"last_updated_at"`
	}
	if err := f.obter(ctx, strings.TrimRight(f.CoinGecko, "/")+"/api/v3/simple/price?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	var out []Cotacao
	for _, id := range ids {
		x, ok := r[id]
		if !ok || x.BRL == "" {
			continue
		}
		preco, err := core.ParseDec8(decimalSemExpoente(x.BRL.String()))
		if err != nil || preco <= 0 {
			continue
		}
		data := diaSP(f.agora())
		if s, err := x.Atualizacao.Int64(); err == nil && s > 0 {
			data = diaSP(time.Unix(s, 0))
		}
		out = append(out, Cotacao{Codigo: id, Data: data, Preco: preco, Moeda: "BRL", Fonte: "coingecko"})
	}
	return out, nil
}

// decimalSemExpoente: a CoinGecko pode mandar "1.2e-05" para moedas baratas.
func decimalSemExpoente(s string) string {
	if !strings.ContainsAny(s, "eE") {
		return s
	}
	v, err := json.Number(s).Float64()
	if err != nil {
		return s
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.8f", v), "0"), ".")
}

// SerieBCB lê uma série do SGS entre duas datas (o SGS limita a 10 anos por consulta).
func (f *Fontes) SerieBCB(ctx context.Context, serie string, de, ate time.Time) ([]Indice, error) {
	codigo, ok := SeriesBCB[serie]
	if !ok {
		return nil, fmt.Errorf("série desconhecida: %s", serie)
	}
	q := url.Values{"formato": {"json"}, "dataInicial": {de.Format("02/01/2006")}, "dataFinal": {ate.Format("02/01/2006")}}
	endereco := fmt.Sprintf("%s/dados/serie/bcdata.sgs.%d/dados?%s", strings.TrimRight(f.BCB, "/"), codigo, q.Encode())
	var r []struct {
		Data  string `json:"data"`
		Valor string `json:"valor"`
	}
	if err := f.obter(ctx, endereco, &r); err != nil {
		// sem dados no intervalo o SGS responde 404
		if strings.Contains(err.Error(), "respondeu 404") {
			return nil, nil
		}
		return nil, err
	}
	var out []Indice
	for _, x := range r {
		d, err := time.Parse("02/01/2006", x.Data)
		if err != nil {
			continue
		}
		v := strings.TrimSpace(strings.ReplaceAll(x.Valor, ",", "."))
		if !numeroValido(v) {
			continue
		}
		out = append(out, Indice{Serie: serie, Data: d, Valor: v})
	}
	return out, nil
}

func numeroValido(s string) bool {
	_, err := json.Number(s).Float64()
	return err == nil
}

// Relatorio de uma atualização.
type Relatorio struct {
	Cotacoes int      `json:"cotacoes"`
	Indices  int      `json:"indices"`
	Erros    []string `json:"erros"`
}

// Atualizar busca as cotações dos códigos em uso e os índices desde o último guardado.
// O erro de uma fonte não para as outras; os erros ficam no relatório.
func Atualizar(ctx context.Context, pool *pgxpool.Pool, f *Fontes) (Relatorio, error) {
	rel := Relatorio{Erros: []string{}}
	linhas, err := pool.Query(ctx, "select codigo, classe from app_codigos_para_cotar()")
	if err != nil {
		return rel, err
	}
	type codigo struct{ codigo, classe string }
	cods, err := pgx.CollectRows(linhas, func(r pgx.CollectableRow) (codigo, error) {
		var c codigo
		return c, r.Scan(&c.codigo, &c.classe)
	})
	if err != nil {
		return rel, err
	}
	var cotas []Cotacao
	var criptos []string
	for _, c := range cods {
		switch c.classe {
		case core.ClasseAcao, core.ClasseFII, core.ClasseETF, core.ClasseBDR:
			x, err := f.Acao(ctx, c.codigo)
			if err != nil {
				rel.Erros = append(rel.Erros, err.Error())
				continue
			}
			cotas = append(cotas, x)
		case "cripto":
			criptos = append(criptos, c.codigo)
		}
	}
	if len(criptos) > 0 {
		sort.Strings(criptos)
		x, err := f.Cripto(ctx, criptos)
		if err != nil {
			rel.Erros = append(rel.Erros, err.Error())
		}
		cotas = append(cotas, x...)
	}
	for _, c := range cotas {
		_, err := pool.Exec(ctx, `insert into cotacoes (codigo, data, preco, moeda, fonte) values ($1, $2, $3::numeric, $4, $5)
			on conflict (codigo, data) do update set preco = excluded.preco, moeda = excluded.moeda, fonte = excluded.fonte`,
			c.Codigo, c.Data, c.Preco.String(), c.Moeda, c.Fonte)
		if err != nil {
			return rel, err
		}
		rel.Cotacoes++
	}

	hoje := diaSP(f.agora())
	series := make([]string, 0, len(SeriesBCB))
	for s := range SeriesBCB {
		series = append(series, s)
	}
	sort.Strings(series)
	for _, s := range series {
		var ultima *time.Time
		if err := pool.QueryRow(ctx, "select max(data) from indices where serie = $1", s).Scan(&ultima); err != nil {
			return rel, err
		}
		de := hoje.AddDate(-5, 0, 0) // histórico de 5 anos na primeira vez
		if ultima != nil {
			de = ultima.AddDate(0, 0, 1)
		}
		if de.After(hoje) {
			continue
		}
		idx, err := f.SerieBCB(ctx, s, de, hoje)
		if err != nil {
			rel.Erros = append(rel.Erros, fmt.Sprintf("%s: %v", s, err))
			continue
		}
		for _, i := range idx {
			_, err := pool.Exec(ctx, `insert into indices (serie, data, valor) values ($1, $2, $3::numeric)
				on conflict (serie, data) do update set valor = excluded.valor`, i.Serie, i.Data, i.Valor)
			if err != nil {
				return rel, err
			}
			rel.Indices++
		}
	}
	if len(rel.Erros) > 0 {
		slog.Warn("cotações: fontes com erro", "erros", rel.Erros)
	}
	return rel, nil
}
