package financeiro

import (
	"context"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// HistoricoInvestimentos: retorno acumulado da carteira contra CDI, IPCA e Ibovespa no fim
// de cada mês, e os proventos de cada mês.
type HistoricoInvestimentos struct {
	Datas     []string   `json:"datas"`
	Valores   []int64    `json:"valores_centavos"` // da parte com histórico (sem Open Finance)
	Carteira  []float64  `json:"carteira"`
	CDI       []*float64 `json:"cdi"`
	IPCA      []*float64 `json:"ipca"`
	Ibovespa  []*float64 `json:"ibovespa"`
	Proventos []struct {
		Mes   string `json:"mes"`
		Valor int64  `json:"valor_centavos"`
	} `json:"proventos"`
	ForaDoHistorico int `json:"fora_do_historico"` // ativos do Open Finance (o banco não dá o histórico)
}

// CalcularHistoricoInvestimentos dos últimos `meses` meses até hoje.
func CalcularHistoricoInvestimentos(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, meses int) (HistoricoInvestimentos, error) {
	var h HistoricoInvestimentos
	var datas []time.Time
	for i := meses; i >= 1; i-- {
		datas = append(datas, fimDoMes(core.DiaNoMes(hoje.Year(), hoje.Month()-time.Month(i), 1)))
	}
	datas = append(datas, dia(hoje))

	for _, d := range datas {
		c, err := CalcularCarteira(ctx, tx, ents, d)
		if err != nil {
			return h, err
		}
		var v core.Centavos
		for _, a := range c.Ativos {
			if a.Origem != "pluggy" {
				v += a.Valor
			}
		}
		h.Datas = append(h.Datas, d.Format(formatoData))
		h.Valores = append(h.Valores, int64(v))
	}

	// fluxos do investidor nos ativos com histórico
	linhas, err := tx.Query(ctx, `select id, origem = 'pluggy' from ativos where entidade_id = any($1)`, ents)
	if err != nil {
		return h, err
	}
	type ativo struct {
		id     string
		pluggy bool
	}
	ativos, err := pgx.CollectRows(linhas, func(r pgx.CollectableRow) (ativo, error) {
		var a ativo
		return a, r.Scan(&a.id, &a.pluggy)
	})
	if err != nil {
		return h, err
	}
	var fluxos []core.Fluxo
	for _, a := range ativos {
		if a.pluggy {
			h.ForaDoHistorico++
			continue
		}
		ops, _, err := operacoesDoAtivo(ctx, tx, a.id)
		if err != nil {
			return h, err
		}
		fluxos = append(fluxos, core.FluxosDasOperacoes(ops, 0, dia(hoje))...)
	}
	valores := make([]core.Centavos, len(h.Valores))
	for i, v := range h.Valores {
		valores[i] = core.Centavos(v)
	}
	h.Carteira = core.SerieRetorno(datas, valores, fluxos)

	ix := &indicesCarteira{}
	if err := ix.carregar(ctx, tx); err != nil {
		return h, err
	}
	h.CDI = core.CDIAcumulado(datas, ix.cdi)
	h.IPCA = core.IPCAAcumulado(datas, ix.ipca)
	linhas, err = tx.Query(ctx, "select data, preco::text from cotacoes where codigo = '^BVSP' order by data")
	if err != nil {
		return h, err
	}
	var precos []core.PrecoData
	for linhas.Next() {
		var p core.PrecoData
		var texto string
		if err := linhas.Scan(&p.Data, &texto); err != nil {
			linhas.Close()
			return h, err
		}
		if r, ok := new(big.Rat).SetString(texto); ok {
			p.Preco, _ = r.Float64()
			precos = append(precos, p)
		}
	}
	linhas.Close()
	h.Ibovespa = core.PrecoAcumulado(datas, precos)

	// proventos (líquidos do IR) por mês
	linhas, err = tx.Query(ctx, `select to_char(o.data, 'YYYY-MM'), sum(o.valor_centavos - o.ir_retido_centavos)
		from operacoes o where o.entidade_id = any($1) and o.tipo in ('provento', 'juros') and o.data > $2 and o.data <= $3
		group by 1 order by 1`, ents, datas[0], datas[len(datas)-1])
	if err != nil {
		return h, err
	}
	defer linhas.Close()
	porMes := map[string]int64{}
	for linhas.Next() {
		var m string
		var v int64
		if err := linhas.Scan(&m, &v); err != nil {
			return h, err
		}
		porMes[m] = v
	}
	for _, d := range datas[1:] {
		m := d.Format("2006-01")
		h.Proventos = append(h.Proventos, struct {
			Mes   string `json:"mes"`
			Valor int64  `json:"valor_centavos"`
		}{m, porMes[m]})
	}
	return h, linhas.Err()
}

// AlocacaoAlvo: fração por classe (soma 1). Guardada em percentual na tabela da fase 4.
type AlocacaoAlvo map[string]string

// LerAlocacaoAlvo das entidades (a primeira que tiver).
func LerAlocacaoAlvo(ctx context.Context, tx pgx.Tx, entidadeID string) (AlocacaoAlvo, error) {
	linhas, err := tx.Query(ctx, `select chave, (percentual / 100)::numeric(5, 4)::text from alocacao_alvo
		where entidade_id = $1 and dimensao = 'classe'`, entidadeID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	out := AlocacaoAlvo{}
	for linhas.Next() {
		var c, f string
		if err := linhas.Scan(&c, &f); err != nil {
			return nil, err
		}
		out[c] = f
	}
	return out, linhas.Err()
}

// SalvarAlocacaoAlvo troca a alocação alvo da entidade (frações que somam 100 %).
func SalvarAlocacaoAlvo(ctx context.Context, tx pgx.Tx, entidadeID string, alvo AlocacaoAlvo) error {
	if err := PodeEditarEntidade(ctx, tx, entidadeID); err != nil {
		return err
	}
	soma := new(big.Rat)
	for classe, f := range alvo {
		r, ok := new(big.Rat).SetString(f)
		if !ok || r.Sign() <= 0 || r.Cmp(big.NewRat(1, 1)) > 0 || !classeValida(classe) {
			return ErrCampo{"cada classe com uma fração entre 0 e 1"}
		}
		soma.Add(soma, r)
	}
	if len(alvo) > 0 && soma.Cmp(big.NewRat(1, 1)) != 0 {
		return ErrCampo{"a alocação alvo precisa somar 100 %"}
	}
	if _, err := tx.Exec(ctx, "delete from alocacao_alvo where entidade_id = $1 and dimensao = 'classe'", entidadeID); err != nil {
		return err
	}
	for classe, f := range alvo {
		if _, err := tx.Exec(ctx, `insert into alocacao_alvo (entidade_id, dimensao, chave, percentual)
			values ($1, 'classe', $2, $3::numeric * 100)`, entidadeID, classe, f); err != nil {
			return err
		}
	}
	return nil
}

var classesAtivo = map[string]bool{"acao": true, "fii": true, "etf": true, "bdr": true, "renda_fixa": true, "tesouro": true,
	"fundo": true, "cripto": true, "exterior": true, "previdencia": true, "outro": true}

func classeValida(c string) bool { return classesAtivo[c] }
