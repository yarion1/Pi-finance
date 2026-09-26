package financeiro

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

const formatoData = "2006-01-02"

func dia(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }

// Entidades do painel: as de que o usuário é dono, ou só a pedida (se ele a vê).
func Entidades(ctx context.Context, tx pgx.Tx, entidadeID string) ([]string, error) {
	if entidadeID != "" {
		var id string
		if err := tx.QueryRow(ctx, "select id from entidades where id = $1", entidadeID).Scan(&id); err != nil {
			return nil, err
		}
		return []string{id}, nil
	}
	linhas, err := tx.Query(ctx, "select id from entidades where dono_id = app_usuario_id()")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, pgx.RowTo[string])
}

// ---------------------------------------------------------------------------
// Cartões: faturas e parcelas futuras
// ---------------------------------------------------------------------------

// Fatura de um cartão.
type Fatura struct {
	Vencimento string `json:"vencimento"`
	Fechamento string `json:"fechamento"`
	Status     string `json:"status"`             // aberta, futura, fechada, anterior
	Compras    int64  `json:"compras_centavos"`   // gastos − estornos (positivo)
	Projetado  int64  `json:"projetado_centavos"` // parcelas importadas que ainda vão cair
	Pagamentos int64  `json:"pagamentos_centavos"`
	Transacoes int    `json:"transacoes"`
	Paga       bool   `json:"paga"`
}

type contaCartao struct {
	id, nome               string
	fechamento, vencimento int
}

func cartoes(ctx context.Context, tx pgx.Tx, ents []string, contaID string) ([]contaCartao, error) {
	consulta := `select id, nome, fechamento, vencimento from contas
		where tipo = 'cartao' and fechamento is not null and vencimento is not null and not arquivada and entidade_id = any($1)`
	args := []any{ents}
	if contaID != "" {
		consulta += " and id = $2"
		args = append(args, contaID)
	}
	linhas, err := tx.Query(ctx, consulta, args...)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var out []contaCartao
	for linhas.Next() {
		var c contaCartao
		var f, v int16
		if err := linhas.Scan(&c.id, &c.nome, &f, &v); err != nil {
			return nil, err
		}
		c.fechamento, c.vencimento = int(f), int(v)
		out = append(out, c)
	}
	return out, linhas.Err()
}

// ParcelaFutura ainda vai pesar no orçamento: lançada com data futura (compra
// parcelada manual) ou projetada a partir da última parcela importada.
type ParcelaFutura struct {
	ContaID   string `json:"conta_id"`
	Conta     string `json:"conta"`
	Descricao string `json:"descricao"`
	Parcela   int    `json:"parcela"`
	Total     int    `json:"total"`
	Valor     int64  `json:"valor_centavos"`
	Mes       string `json:"mes"` // mês em que é paga (fatura ou data)
	Data      string `json:"data"`
	FaturaEm  string `json:"fatura_em,omitempty"`
	Projetada bool   `json:"projetada"`
}

// ParcelasFuturas das entidades (ou de um cartão).
func ParcelasFuturas(ctx context.Context, tx pgx.Tx, ents []string, contaID string, hoje time.Time) ([]ParcelaFutura, error) {
	hoje = dia(hoje)
	dias := map[string]contaCartao{}
	cs, err := cartoes(ctx, tx, ents, contaID)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		dias[c.id] = c
	}

	filtro, args := "t.entidade_id = any($1)", []any{ents}
	if contaID != "" {
		filtro, args = filtro+" and t.conta_id = $2", append(args, contaID)
	}
	linhas, err := tx.Query(ctx, `select t.conta_id, c.nome, t.descricao, t.parcela_n, t.parcela_total, t.valor_centavos,
			t.data, t.fatura_em, t.origem::text
		from transacoes t join contas c on c.id = t.conta_id
		where `+filtro+` and t.parcela_total > 1 and t.parcela_n is not null
		order by t.data`, args...)
	if err != nil {
		return nil, err
	}
	type linha struct {
		ParcelaFutura
		data   time.Time
		fatura *time.Time
		origem string
	}
	var todas []linha
	for linhas.Next() {
		var l linha
		var n, total int16
		if err := linhas.Scan(&l.ContaID, &l.Conta, &l.Descricao, &n, &total, &l.Valor, &l.data, &l.fatura, &l.origem); err != nil {
			linhas.Close()
			return nil, err
		}
		l.Parcela, l.Total = int(n), int(total)
		todas = append(todas, l)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, err
	}

	mesDe := func(data time.Time, fatura *time.Time) (string, string) {
		if fatura != nil {
			return fatura.Format("2006-01"), fatura.Format(formatoData)
		}
		return data.Format("2006-01"), ""
	}

	var out []ParcelaFutura
	// a última parcela vista de cada compra importada projeta as que faltam
	type chave struct {
		conta, base string
		valor       int64
		total       int
	}
	ultimas := map[chave]linha{}
	for _, l := range todas {
		pendente := (l.fatura != nil && l.fatura.After(hoje)) || (l.fatura == nil && l.data.After(hoje))
		if pendente {
			p := l.ParcelaFutura
			p.Mes, p.FaturaEm = mesDe(l.data, l.fatura)
			p.Data = l.data.Format(formatoData)
			out = append(out, p)
		}
		if l.origem == "manual" {
			continue // compras parceladas manuais já têm todas as parcelas lançadas
		}
		base, _, _, _ := core.ExtrairParcela(l.Descricao)
		k := chave{l.ContaID, core.NormalizarDescricao(base), l.Valor, l.Total}
		if u, ok := ultimas[k]; !ok || l.Parcela > u.Parcela {
			ultimas[k] = l
		}
	}
	for _, u := range ultimas {
		base, _, _, _ := core.ExtrairParcela(u.Descricao)
		for n := u.Parcela + 1; n <= u.Total; n++ {
			data := core.SomarMeses(u.data, n-u.Parcela)
			var fatura *time.Time
			if c, ok := dias[u.ContaID]; ok {
				_, v := core.FaturaDaCompra(data, c.fechamento, c.vencimento)
				fatura = &v
			}
			if (fatura != nil && !fatura.After(hoje)) || (fatura == nil && !data.After(hoje)) {
				continue
			}
			p := ParcelaFutura{ContaID: u.ContaID, Conta: u.Conta, Descricao: base, Parcela: n, Total: u.Total,
				Valor: u.Valor, Data: data.Format(formatoData), Projetada: true}
			p.Mes, p.FaturaEm = mesDe(data, fatura)
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mes != out[j].Mes {
			return out[i].Mes < out[j].Mes
		}
		return out[i].Descricao < out[j].Descricao
	})
	return out, nil
}

// Faturas de um cartão: as que têm lançamento, a aberta e as futuras com parcelas.
func Faturas(ctx context.Context, tx pgx.Tx, contaID string, hoje time.Time) ([]Fatura, error) {
	hoje = dia(hoje)
	var ent string
	var f, v *int16
	if err := tx.QueryRow(ctx, "select entidade_id, fechamento, vencimento from contas where id = $1 and tipo = 'cartao'", contaID).
		Scan(&ent, &f, &v); err != nil {
		return nil, err
	}
	if f == nil || v == nil {
		return []Fatura{}, nil
	}
	fech, venc := int(*f), int(*v)

	porVenc := map[string]*Fatura{}
	obter := func(vencimento time.Time) *Fatura {
		k := vencimento.Format(formatoData)
		if porVenc[k] == nil {
			porVenc[k] = &Fatura{Vencimento: k, Fechamento: core.FechamentoDaFatura(vencimento, fech, venc).Format(formatoData)}
		}
		return porVenc[k]
	}
	_, aberta := core.FaturaDaCompra(hoje, fech, venc)
	obter(aberta)

	linhas, err := tx.Query(ctx, `select fatura_em,
			coalesce(-sum(valor_centavos) filter (where tipo in ('gasto', 'estorno')), 0),
			coalesce(sum(valor_centavos) filter (where tipo = 'transferencia' and valor_centavos > 0), 0),
			count(*)
		from transacoes where conta_id = $1 and fatura_em is not null group by fatura_em`, contaID)
	if err != nil {
		return nil, err
	}
	for linhas.Next() {
		var d time.Time
		var compras, pagamentos int64
		var n int
		if err := linhas.Scan(&d, &compras, &pagamentos, &n); err != nil {
			linhas.Close()
			return nil, err
		}
		fat := obter(d)
		fat.Compras, fat.Pagamentos, fat.Transacoes = compras, pagamentos, n
	}
	linhas.Close()

	futuras, err := ParcelasFuturas(ctx, tx, []string{ent}, contaID, hoje)
	if err != nil {
		return nil, err
	}
	for _, p := range futuras {
		if p.Projetada && p.FaturaEm != "" {
			d, _ := time.Parse(formatoData, p.FaturaEm)
			obter(d).Projetado += -p.Valor
		}
	}

	// paga: pagamento (entrada no cartão) entre o fechamento e alguns dias depois do vencimento
	lista := make([]Fatura, 0, len(porVenc))
	for _, fat := range porVenc {
		fechamento, _ := time.Parse(formatoData, fat.Fechamento)
		vencimento, _ := time.Parse(formatoData, fat.Vencimento)
		switch {
		case fechamento.After(hoje) && fat.Vencimento == aberta.Format(formatoData):
			fat.Status = "aberta"
		case fechamento.After(hoje):
			fat.Status = "futura"
		case !vencimento.Before(hoje):
			fat.Status = "fechada"
		default:
			fat.Status = "anterior"
		}
		if fat.Compras > 0 && !fechamento.After(hoje) {
			var pago int64
			_ = tx.QueryRow(ctx, `select coalesce(sum(valor_centavos), 0) from transacoes
				where conta_id = $1 and valor_centavos > 0 and tipo = 'transferencia' and data between $2 and $3`,
				contaID, fechamento, vencimento.AddDate(0, 0, 10)).Scan(&pago)
			fat.Paga = pago*100 >= fat.Compras*95
		}
		lista = append(lista, *fat)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Vencimento > lista[j].Vencimento })
	return lista, nil
}

// ---------------------------------------------------------------------------
// Dívidas (financiamentos e empréstimos)
// ---------------------------------------------------------------------------

// Divida com a tabela calculada.
type Divida struct {
	ID                 string               `json:"id"`
	EntidadeID         string               `json:"entidade_id"`
	Nome               string               `json:"nome"`
	Tipo               string               `json:"tipo"`
	Sistema            string               `json:"sistema"`
	Principal          int64                `json:"principal_centavos"`
	TaxaMensal         string               `json:"taxa_mensal"`
	Prazo              int                  `json:"prazo_meses"`
	PrimeiroVencimento string               `json:"primeiro_vencimento"`
	ContaID            *string              `json:"conta_id"`
	BemID              *string              `json:"bem_id"`
	SaldoDevedor       int64                `json:"saldo_devedor_centavos"`
	ProximaParcela     *core.ParcelaDivida  `json:"proxima_parcela"`
	ParcelasRestantes  int                  `json:"parcelas_restantes"`
	tabela             []core.ParcelaDivida `json:"-"`
	inicio             time.Time            `json:"-"`
}

// Tabela da dívida (cronograma completo).
func (d Divida) Tabela() []core.ParcelaDivida { return d.tabela }

// Existia na data (a dívida começa um mês antes da primeira parcela).
func (d Divida) ExistiaEm(t time.Time) bool { return !t.Before(core.SomarMeses(d.inicio, -1)) }

// SaldoEm é o saldo devedor na data.
func (d Divida) SaldoEm(t time.Time) int64 {
	if !d.ExistiaEm(t) {
		return 0
	}
	return int64(core.SaldoDevedorEm(d.tabela, core.Centavos(d.Principal), t))
}

// Dividas das entidades, com saldo devedor e próxima parcela em relação a hoje.
func Dividas(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time) ([]Divida, error) {
	linhas, err := tx.Query(ctx, `select id, entidade_id, nome, tipo, sistema, principal_centavos, taxa_mensal::text, prazo_meses,
			primeiro_vencimento, conta_id, bem_id
		from dividas where entidade_id = any($1) order by nome`, ents)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var out []Divida
	for linhas.Next() {
		var d Divida
		if err := linhas.Scan(&d.ID, &d.EntidadeID, &d.Nome, &d.Tipo, &d.Sistema, &d.Principal, &d.TaxaMensal, &d.Prazo,
			&d.inicio, &d.ContaID, &d.BemID); err != nil {
			return nil, err
		}
		d.PrimeiroVencimento = d.inicio.Format(formatoData)
		taxa, err := core.LerTaxa(d.TaxaMensal)
		if err != nil {
			return nil, err
		}
		if d.tabela, err = core.TabelaAmortizacao(d.Sistema, core.Centavos(d.Principal), taxa, d.Prazo, d.inicio); err != nil {
			return nil, err
		}
		d.SaldoDevedor = d.SaldoEm(hoje)
		for i := range d.tabela {
			if d.tabela[i].Vencimento.After(dia(hoje)) {
				p := d.tabela[i]
				d.ProximaParcela = &p
				d.ParcelasRestantes = len(d.tabela) - i
				break
			}
		}
		out = append(out, d)
	}
	return out, linhas.Err()
}

// ---------------------------------------------------------------------------
// Recorrências
// ---------------------------------------------------------------------------

// Recorrencia cadastrada ou detectada, com a próxima data calculada.
type Recorrencia struct {
	ID            string  `json:"id"`
	EntidadeID    string  `json:"entidade_id"`
	ContaID       *string `json:"conta_id"`
	Conta         *string `json:"conta"`
	ContaCartao   bool    `json:"conta_cartao"`
	Descricao     string  `json:"descricao"`
	CategoriaID   *string `json:"categoria_id"`
	Categoria     *string `json:"categoria"`
	Valor         int64   `json:"valor_centavos"`
	ValorAnterior *int64  `json:"valor_anterior_centavos"`
	Frequencia    string  `json:"frequencia"`
	Dia           int     `json:"dia"`
	UltimaData    *string `json:"ultima_data"`
	Proxima       string  `json:"proxima"`
	Atrasada      bool    `json:"atrasada"` // a cobrança esperada não apareceu (cancelada? esquecida?)
	Tipo          string  `json:"tipo"`
	Ativa         bool    `json:"ativa"`
	Detectada     bool    `json:"detectada"`
	Subiu         bool    `json:"subiu"`
	ultima        *time.Time
}

// Ocorrencias da recorrência entre (de, ate].
func (r Recorrencia) Ocorrencias(de, ate time.Time) []time.Time {
	var out []time.Time
	base := core.DiaNoMes(de.Year(), de.Month()-1, r.Dia)
	if r.ultima != nil && r.ultima.After(base) {
		base = *r.ultima
	}
	d := base
	for i := 0; i < 400; i++ {
		d = core.ProximaOcorrencia(d, r.Frequencia, r.Dia, d)
		if d.After(ate) {
			break
		}
		if d.After(de) {
			out = append(out, d)
		}
	}
	return out
}

// Recorrencias das entidades.
func Recorrencias(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time) ([]Recorrencia, error) {
	hoje = dia(hoje)
	linhas, err := tx.Query(ctx, `select r.id, r.entidade_id, r.conta_id, c.nome, coalesce(c.tipo = 'cartao', false), r.descricao,
			r.categoria_id, cat.nome, r.valor_centavos, r.valor_anterior_centavos, r.frequencia::text, r.dia, r.ultima_data,
			r.tipo, r.ativa, r.detectada
		from recorrencias r
		left join contas c on c.id = r.conta_id
		left join categorias cat on cat.id = r.categoria_id
		where r.entidade_id = any($1)
		order by r.ativa desc, r.descricao`, ents)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var out []Recorrencia
	for linhas.Next() {
		var r Recorrencia
		var d int16
		if err := linhas.Scan(&r.ID, &r.EntidadeID, &r.ContaID, &r.Conta, &r.ContaCartao, &r.Descricao, &r.CategoriaID, &r.Categoria,
			&r.Valor, &r.ValorAnterior, &r.Frequencia, &d, &r.ultima, &r.Tipo, &r.Ativa, &r.Detectada); err != nil {
			return nil, err
		}
		r.Dia = int(d)
		if r.ultima != nil {
			u := r.ultima.Format(formatoData)
			r.UltimaData = &u
			esperada := core.ProximaOcorrencia(*r.ultima, r.Frequencia, r.Dia, *r.ultima)
			// só as detectadas têm cobrança real para conferir
			r.Atrasada = r.Ativa && r.Detectada && esperada.Before(hoje.AddDate(0, 0, -7))
		}
		if prox := r.Ocorrencias(hoje.AddDate(0, 0, -1), hoje.AddDate(2, 0, 0)); len(prox) > 0 {
			r.Proxima = prox[0].Format(formatoData)
		}
		if r.ValorAnterior != nil {
			a, v := *r.ValorAnterior, r.Valor
			if a < 0 {
				a, v = -a, -v
			}
			r.Subiu = a > 0 && v*100 >= a*105
		}
		out = append(out, r)
	}
	return out, linhas.Err()
}

// DetectarRecorrencias procura contas fixas e assinaturas nas transações dos
// últimos 13 meses da entidade. Cria as novas, atualiza as conhecidas e avisa o
// usuário quando uma assinatura sobe de preço. Devolve quantas novas achou.
func DetectarRecorrencias(ctx context.Context, tx pgx.Tx, entidadeID string, hoje time.Time) (int, error) {
	linhas, err := tx.Query(ctx, `select t.descricao, t.valor_centavos, t.data, t.conta_id, t.categoria_id, cat.nome
		from transacoes t left join categorias cat on cat.id = t.categoria_id
		where t.entidade_id = $1 and t.tipo in ('gasto', 'receita') and t.parcela_total is null
		  and t.data between $2 and $3
		order by t.data`, entidadeID, dia(hoje).AddDate(0, -13, 0), dia(hoje))
	if err != nil {
		return 0, err
	}
	type grupo struct {
		ocs       []core.Ocorrencia
		descricao string
		conta     string
		categoria *string
		catNome   *string
	}
	grupos := map[string]*grupo{}
	for linhas.Next() {
		var desc, conta string
		var valor int64
		var data time.Time
		var cat, catNome *string
		if err := linhas.Scan(&desc, &valor, &data, &conta, &cat, &catNome); err != nil {
			linhas.Close()
			return 0, err
		}
		chave := core.ChaveHistorico(desc)
		if chave == "" {
			continue
		}
		if valor > 0 {
			chave = "+" + chave
		}
		g := grupos[chave]
		if g == nil {
			g = &grupo{}
			grupos[chave] = g
		}
		g.ocs = append(g.ocs, core.Ocorrencia{Data: data, Valor: core.Centavos(valor)})
		g.descricao, g.conta, g.categoria, g.catNome = desc, conta, cat, catNome
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return 0, err
	}

	novas := 0
	for chave, g := range grupos {
		rec, ok := core.DetectarRecorrencia(g.ocs)
		if !ok {
			continue
		}
		tipo := "conta_fixa"
		switch {
		case rec.Valor > 0:
			tipo = "receita"
		case g.catNome != nil && (*g.catNome == "Streaming" || *g.catNome == "Assinaturas"):
			tipo = "assinatura"
		}
		var id string
		var nova bool
		err := tx.QueryRow(ctx, `insert into recorrencias (entidade_id, conta_id, descricao, chave, categoria_id, valor_centavos,
				valor_anterior_centavos, frequencia, dia, ultima_data, tipo, detectada)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, true)
			on conflict (entidade_id, chave) do update set
				valor_centavos = excluded.valor_centavos, valor_anterior_centavos = excluded.valor_anterior_centavos,
				ultima_data = excluded.ultima_data,
				dia = case when recorrencias.detectada then excluded.dia else recorrencias.dia end
			returning id, (xmax = 0)`,
			entidadeID, g.conta, g.descricao, chave, g.categoria, int64(rec.Valor), int64(rec.ValorAnterior),
			rec.Frequencia, rec.Dia, rec.Ultima, tipo).Scan(&id, &nova)
		if err != nil {
			return novas, err
		}
		if nova {
			novas++
		}
		if rec.Subiu {
			if err := alertar(ctx, tx, "recorrencia_subiu", map[string]any{
				"recorrencia_id": id, "descricao": g.descricao, "de_centavos": int64(rec.ValorAnterior),
				"para_centavos": int64(rec.Valor), "data": rec.Ultima.Format(formatoData),
			}, "recorrencia_id", "data"); err != nil {
				return novas, err
			}
		}
		if nova && tipo == "assinatura" {
			if err := alertar(ctx, tx, "assinatura_detectada", map[string]any{
				"recorrencia_id": id, "descricao": g.descricao, "valor_centavos": int64(rec.Valor),
			}, "recorrencia_id"); err != nil {
				return novas, err
			}
		}
	}
	return novas, nil
}

// alertar cria um alerta para o usuário da sessão, sem repetir o mesmo (chaves iguais).
func alertar(ctx context.Context, tx pgx.Tx, tipo string, dados map[string]any, chaves ...string) error {
	consulta := "select exists (select 1 from alertas where usuario_id = app_usuario_id() and tipo = $1"
	args := []any{tipo}
	for _, k := range chaves {
		args = append(args, k, fmtValor(dados[k]))
		consulta += " and dados->>$" + itoa(len(args)-1) + " = $" + itoa(len(args))
	}
	var existe bool
	if err := tx.QueryRow(ctx, consulta+")", args...).Scan(&existe); err != nil || existe {
		return err
	}
	_, err := tx.Exec(ctx, "insert into alertas (usuario_id, tipo, dados) values (app_usuario_id(), $1, $2)", tipo, dados)
	return err
}

// Alertar cria um alerta para o usuário da sessão; não repete um igual (mesmo tipo
// e mesmos valores nas chaves dadas).
func Alertar(ctx context.Context, tx pgx.Tx, tipo string, dados map[string]any, chaves ...string) error {
	return alertar(ctx, tx, tipo, dados, chaves...)
}

// FaturaBanco: fatura do cartão como o banco fechou (Open Finance).
type FaturaBanco struct {
	Vencimento string  `json:"vencimento"`
	Fechamento *string `json:"fechamento"`
	Total      int64   `json:"total_centavos"`
	Minimo     *int64  `json:"minimo_centavos"`
	Encargos   int64   `json:"encargos_centavos"`
	Moeda      string  `json:"moeda"`
}

// FaturasDoBanco do cartão, as mais recentes primeiro (até 12).
func FaturasDoBanco(ctx context.Context, tx pgx.Tx, contaID string) ([]FaturaBanco, error) {
	linhas, err := tx.Query(ctx, `select to_char(vencimento, 'YYYY-MM-DD'), to_char(fechamento, 'YYYY-MM-DD'), total_centavos,
			minimo_centavos, encargos_centavos, moeda
		from faturas_banco where conta_id = $1 order by vencimento desc limit 12`, contaID)
	if err != nil {
		return nil, err
	}
	lista, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[FaturaBanco])
	if lista == nil {
		lista = []FaturaBanco{}
	}
	return lista, err
}

// AlertarTransacoes roda os alertas inteligentes (core.DetectarAlertas) nas transações
// novas, comparando com os gastos dos últimos 180 dias das mesmas entidades.
func AlertarTransacoes(ctx context.Context, tx pgx.Tx, ids []string, hoje time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	const colunas = `select t.id, t.conta_id, coalesce(t.categoria_id::text, ''), t.descricao, t.valor_centavos, t.data,
		t.parcela_n is not null from transacoes t`
	ler := func(consulta string, args ...any) ([]core.TransacaoAlerta, error) {
		linhas, err := tx.Query(ctx, consulta, args...)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(linhas, func(l pgx.CollectableRow) (core.TransacaoAlerta, error) {
			var t core.TransacaoAlerta
			var v int64
			err := l.Scan(&t.ID, &t.Conta, &t.Categoria, &t.Descricao, &v, &t.Data, &t.Parcela)
			t.Valor = core.Centavos(v)
			return t, err
		})
	}
	novas, err := ler(colunas+" where t.id = any($1) and t.tipo = 'gasto'", ids)
	if err != nil || len(novas) == 0 {
		return err
	}
	hist, err := ler(colunas+` where t.tipo = 'gasto' and t.data >= $2 and not (t.id = any($1))
		and t.entidade_id in (select entidade_id from transacoes where id = any($1))`, ids, hoje.AddDate(0, 0, -180))
	if err != nil {
		return err
	}
	for _, a := range core.DetectarAlertas(novas, hist, hoje) {
		t := a.Transacao
		dados := map[string]any{"transacao_id": t.ID, "descricao": t.Descricao, "valor_centavos": int64(t.Valor),
			"data": t.Data.Format(formatoData)}
		var conta string
		if err := tx.QueryRow(ctx, "select nome from contas where id = $1", t.Conta).Scan(&conta); err == nil {
			dados["conta"] = conta
		}
		switch a.Tipo {
		case "gasto_fora_do_padrao":
			dados["referencia_centavos"] = int64(a.Referencia)
			var cat string
			if err := tx.QueryRow(ctx, "select nome from categorias where id = $1", t.Categoria).Scan(&cat); err == nil {
				dados["categoria"] = cat
			}
		case "cobranca_duplicada":
			dados["outra_id"] = a.Outra
		}
		if err := alertar(ctx, tx, a.Tipo, dados, "transacao_id"); err != nil {
			return err
		}
	}
	return nil
}
