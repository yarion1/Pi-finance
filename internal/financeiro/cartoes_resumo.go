package financeiro

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// Visão geral dos cartões: quanto se deve em cada um e quanto cai em cada mês. O valor
// de uma fatura vem dos lançamentos; quem não lança compra por compra informa o total e o
// painel grava a diferença como um ajuste na própria fatura.

// MesesResumoCartoes: quantos meses a visão geral mostra.
const MesesResumoCartoes = 6

// MarcaAjusteFatura identifica o lançamento de ajuste (descrição original fixa).
const MarcaAjusteFatura = "AJUSTE DA FATURA"

// FaturaResumo de um cartão num mês.
type FaturaResumo struct {
	Vencimento string `json:"vencimento"`
	Valor      int64  `json:"valor_centavos"` // a pagar (0 se paga)
	Status     string `json:"status"`         // fechada, aberta, futura
	Paga       bool   `json:"paga"`
	Informado  bool   `json:"informado"` // tem ajuste de valor informado
}

// CartaoResumo: um cartão na visão geral.
type CartaoResumo struct {
	ID      string         `json:"id"`
	Nome    string         `json:"nome"`
	Moeda   string         `json:"moeda"`
	Limite  *int64         `json:"limite_centavos"`
	Devendo int64          `json:"devendo_centavos"` // todas as faturas a pagar, inclusive parcelas além dos 6 meses
	SemDias bool           `json:"sem_dias"`
	Faturas []FaturaResumo `json:"faturas"` // os próximos vencimentos
}

// ResumoCartoes da pessoa.
type ResumoCartoes struct {
	Meses        []string       `json:"meses"` // AAAA-MM
	TotalPorMes  []int64        `json:"total_por_mes_centavos"`
	TotalDevendo int64          `json:"total_devendo_centavos"`
	Cartoes      []CartaoResumo `json:"cartoes"`
}

// MontarResumoCartoes das entidades (cartões não arquivados, em reais).
func MontarResumoCartoes(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time) (ResumoCartoes, error) {
	hoje = dia(hoje)
	r := ResumoCartoes{Cartoes: []CartaoResumo{}}
	inicio := time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC)
	idx := map[string]int{}
	for i := range MesesResumoCartoes {
		m := inicio.AddDate(0, i, 0).Format("2006-01")
		idx[m] = i
		r.Meses = append(r.Meses, m)
	}
	r.TotalPorMes = make([]int64, MesesResumoCartoes)

	linhas, err := tx.Query(ctx, `select id, nome, moeda, limite_centavos, fechamento, vencimento,
			greatest(-coalesce(app_saldo_conta(id), 0), 0)
		from contas where tipo = 'cartao' and not arquivada and entidade_id = any($1) order by nome`, ents)
	if err != nil {
		return r, err
	}
	type cartao struct {
		CartaoResumo
		fech, venc *int16
		usado      int64
	}
	var lista []cartao
	for linhas.Next() {
		var c cartao
		if err := linhas.Scan(&c.ID, &c.Nome, &c.Moeda, &c.Limite, &c.fech, &c.venc, &c.usado); err != nil {
			linhas.Close()
			return r, err
		}
		lista = append(lista, c)
	}
	linhas.Close()

	for _, c := range lista {
		c.Faturas = []FaturaResumo{}
		if c.fech == nil || c.venc == nil {
			// sem os dias não há fatura: o que se deve é o saldo negativo do cartão
			c.SemDias, c.Devendo = true, c.usado
		} else {
			faturas, err := Faturas(ctx, tx, c.ID, hoje)
			if err != nil {
				return r, err
			}
			porVenc := map[string]Fatura{}
			for _, f := range faturas {
				porVenc[f.Vencimento] = f
				if f.Status != "anterior" && !f.Paga {
					c.Devendo += max(f.Compras+f.Projetado, 0)
				}
			}
			informados, err := vencimentosInformados(ctx, tx, c.ID)
			if err != nil {
				return r, err
			}
			abertaVista := false
			for _, v := range core.ProximosVencimentos(hoje, int(*c.fech), int(*c.venc), MesesResumoCartoes) {
				k := v.Format(formatoData)
				f, tem := porVenc[k]
				fr := FaturaResumo{Vencimento: k, Informado: informados[k]}
				switch {
				case !core.FechamentoDaFatura(v, int(*c.fech), int(*c.venc)).After(hoje):
					fr.Status = "fechada"
				case !abertaVista:
					fr.Status, abertaVista = "aberta", true
				default:
					fr.Status = "futura"
				}
				if tem {
					fr.Paga = f.Paga
					if !f.Paga {
						fr.Valor = max(f.Compras+f.Projetado, 0)
					}
				}
				c.Faturas = append(c.Faturas, fr)
				if i, ok := idx[k[:7]]; ok && c.Moeda == "BRL" {
					r.TotalPorMes[i] += fr.Valor
				}
			}
		}
		if c.Moeda == "BRL" {
			r.TotalDevendo += c.Devendo
		}
		r.Cartoes = append(r.Cartoes, c.CartaoResumo)
	}
	return r, nil
}

func vencimentosInformados(ctx context.Context, tx pgx.Tx, contaID string) (map[string]bool, error) {
	linhas, err := tx.Query(ctx, `select distinct to_char(fatura_em, 'YYYY-MM-DD') from transacoes
		where conta_id = $1 and descricao_original = $2 and fatura_em is not null`, contaID, MarcaAjusteFatura)
	if err != nil {
		return nil, err
	}
	vs, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	out := map[string]bool{}
	for _, v := range vs {
		out[v] = true
	}
	return out, err
}

// InformarValorFatura faz a fatura do vencimento valer o total informado: apaga o ajuste
// anterior dela (se houver) e grava a diferença para os lançamentos que existem.
func InformarValorFatura(ctx context.Context, tx pgx.Tx, contaID string, vencimento time.Time, valor int64, hoje time.Time) error {
	c, err := InfoContaEditavel(ctx, tx, contaID)
	if err != nil {
		return err
	}
	if !c.EhCartao() {
		return ErrCampo{"informe os dias de fechamento e vencimento do cartão em Contas"}
	}
	fech, venc := int(*c.Fechamento), int(*c.Vencimento)
	if !core.DiaNoMes(vencimento.Year(), vencimento.Month(), venc).Equal(vencimento) {
		return ErrCampo{"essa data não é um vencimento deste cartão"}
	}
	if valor < 0 || valor > 100_000_000_00 {
		return ErrCampo{"valor da fatura inválido"}
	}
	if _, err := tx.Exec(ctx, "delete from transacoes where conta_id = $1 and fatura_em = $2 and descricao_original = $3",
		contaID, vencimento, MarcaAjusteFatura); err != nil {
		return err
	}
	faturas, err := Faturas(ctx, tx, contaID, hoje)
	if err != nil {
		return err
	}
	var atual int64
	k := vencimento.Format(formatoData)
	for _, f := range faturas {
		if f.Vencimento == k {
			atual = max(f.Compras+f.Projetado, 0)
		}
	}
	tipo, ajuste := core.AjusteDaFatura(core.Centavos(valor), core.Centavos(atual))
	if tipo == "" {
		return nil
	}
	// o ajuste fica no último dia do período da fatura, para cair nela
	data := core.FechamentoDaFatura(vencimento, fech, venc).AddDate(0, 0, -1)
	_, err = tx.Exec(ctx, `insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao, valor_centavos,
			moeda, tipo, origem, notas, fatura_em)
		values ($1, $2, $3, $4, 'Ajuste da fatura (valor informado)', $5, $6, $7, 'manual',
			'Diferença entre o valor informado da fatura e os lançamentos dela', $8)`,
		c.EntidadeID, contaID, data, MarcaAjusteFatura, int64(ajuste), c.Moeda, tipo, vencimento)
	return err
}
