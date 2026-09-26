package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

// inteiroQuery lê um inteiro da query (padrão quando vazio).
func inteiroQuery(r *http.Request, nome string, padrao int64) (int64, error) {
	v := r.URL.Query().Get(nome)
	if v == "" {
		return padrao, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, invalido(nome + " inválido")
	}
	return n, nil
}

// percentualQuery lê "7,5" ou "7.5" (por cento) como fração; ok=false quando ausente.
func percentualQuery(r *http.Request, nome string) (float64, bool, error) {
	v := r.URL.Query().Get(nome)
	if v == "" {
		return 0, false, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < -100 || f > 1000 {
		return 0, false, invalido(nome + " inválido")
	}
	return f / 100, true, nil
}

// GET /api/projecoes/fluxo?dias=90: saldo das contas do dia a dia com a faixa de confiança.
func (s *Servidor) projecaoFluxo(w http.ResponseWriter, r *http.Request) {
	var f financeiro.FluxoProjetado
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		dias, err := inteiroQuery(r, "dias", 90)
		if err != nil {
			return err
		}
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		f, err = financeiro.ProjetarFluxo(ctx, tx, ents, financeiro.Hoje(), int(dias), nil)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, f)
}

// GET /api/projecoes/futuro?aporte=&anos=&custo=&retirada=&perfil=&r_acao=&v_acao=...:
// Monte Carlo do patrimônio investível e a independência financeira.
func (s *Servidor) projecaoFuturo(w http.ResponseWriter, r *http.Request) {
	var f financeiro.Futuro
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var e financeiro.EntradaFuturo
		aporte, err := inteiroQuery(r, "aporte", 0)
		if err != nil {
			return err
		}
		anos, err := inteiroQuery(r, "anos", 10)
		if err != nil {
			return err
		}
		custo, err := inteiroQuery(r, "custo", 0)
		if err != nil {
			return err
		}
		retirada, _, err := percentualQuery(r, "retirada")
		if err != nil {
			return err
		}
		e.Aporte, e.Anos, e.Custo, e.TaxaRetirada = core.Centavos(aporte), int(anos), core.Centavos(custo), retirada
		e.Perfil = r.URL.Query().Get("perfil")
		e.Premissas = map[string]core.PremissaClasse{}
		for classe, padrao := range core.PremissasPadrao {
			p := padrao
			rv, okR, err := percentualQuery(r, "r_"+classe)
			if err != nil {
				return err
			}
			vv, okV, err := percentualQuery(r, "v_"+classe)
			if err != nil {
				return err
			}
			if okR {
				p.Retorno = rv
			}
			if okV {
				if vv < 0 || vv > 2 {
					return invalido("volatilidade de 0 a 200 %")
				}
				p.Volatilidade = vv
			}
			e.Premissas[classe] = p
		}
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		f, err = financeiro.SimularFuturo(ctx, tx, ents, financeiro.Hoje(), e)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, f)
}

// POST /api/simulacoes/compra: "posso comprar X em N×?"
func (s *Servidor) simularCompra(w http.ResponseWriter, r *http.Request) {
	var d struct {
		EntidadeID string `json:"entidade_id"`
		Valor      int64  `json:"valor_centavos"`
		Parcelas   int    `json:"parcelas"`
		Primeira   string `json:"primeira"`
	}
	if !lerJSON(w, r, &d) {
		return
	}
	var res financeiro.SimulacaoCompra
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		primeira, err := time.Parse("2006-01-02", d.Primeira)
		if err != nil {
			return invalido("data da primeira parcela inválida")
		}
		ents, err := financeiro.Entidades(ctx, tx, d.EntidadeID)
		if err != nil {
			return err
		}
		res, err = financeiro.SimularCompra(ctx, tx, ents, financeiro.Hoje(), core.Centavos(d.Valor), d.Parcelas, primeira)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, res)
}

// POST /api/simulacoes/financiamento: parcelar ou pagar à vista.
func (s *Servidor) simularFinanciamento(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Avista        int64    `json:"avista_centavos"`
		Parcela       int64    `json:"parcela_centavos"`
		Parcelas      int      `json:"parcelas"`
		Primeira      int      `json:"primeira_em_meses"`
		RendimentoMes *float64 `json:"rendimento_mes"`
	}
	if !lerJSON(w, r, &d) {
		return
	}
	var res struct {
		core.ComparacaoFinanciamento
		RendimentoMes float64 `json:"rendimento_mes"`
		DoCDI         bool    `json:"do_cdi"`
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if d.RendimentoMes != nil {
			if *d.RendimentoMes < 0 || *d.RendimentoMes > 0.2 {
				return invalido("rendimento de 0 a 20 % ao mês")
			}
			res.RendimentoMes = *d.RendimentoMes
		} else {
			cdi, err := financeiro.CDIMensal(ctx, tx)
			if err != nil {
				return err
			}
			res.RendimentoMes, res.DoCDI = cdi, true
		}
		c, err := core.CompararFinanciamento(core.Centavos(d.Avista), core.Centavos(d.Parcela), d.Parcelas, d.Primeira, res.RendimentoMes)
		if err != nil {
			return invalido(err.Error())
		}
		res.ComparacaoFinanciamento = c
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, res)
}

// POST /api/simulacoes/quitar: pagar parte de uma dívida antes.
func (s *Servidor) simularQuitacao(w http.ResponseWriter, r *http.Request) {
	var d struct {
		DividaID string `json:"divida_id"`
		Valor    int64  `json:"valor_centavos"`
		Modo     string `json:"modo"`
	}
	if !lerJSON(w, r, &d) {
		return
	}
	var res core.SimulacaoQuitacao
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		res, err = financeiro.SimularQuitacao(ctx, tx, d.DividaID, financeiro.Hoje(), core.Centavos(d.Valor), d.Modo)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, res)
}
