package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/ia"
)

func (s *Servidor) configIA(w http.ResponseWriter, r *http.Request) {
	var c financeiro.ConfigIA
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		c, err = financeiro.LerConfigIA(ctx, tx, financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	c.Servidor = s.IA != nil
	escreverJSON(w, http.StatusOK, c)
}

// salvarConfigIA: ligar a IA manda dados para fora do Pi, então pede a senha de novo.
func (s *Servidor) salvarConfigIA(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Ativa bool  `json:"ativa"`
		Teto  int64 `json:"teto_mensal_microdolares"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if c.Ativa && !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := financeiro.SalvarConfigIA(ctx, tx, c.Ativa, c.Teto); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "ia_config", sessaoDe(r).UsuarioID, map[string]any{"ativa": c.Ativa, "teto": c.Teto})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) exigirIA(w http.ResponseWriter, r *http.Request) bool {
	if s.IA == nil {
		erroJSON(w, http.StatusServiceUnavailable, "ia_indisponivel", "a IA não está configurada neste servidor")
		return false
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.PodeUsarIA(ctx, tx, financeiro.Hoje())
	})
	if err != nil {
		falhar(w, r, err)
		return false
	}
	return true
}

// categorizarIA: as transações sem categoria que regras e histórico não resolveram.
func (s *Servidor) categorizarIA(w http.ResponseWriter, r *http.Request) {
	if !s.exigirIA(w, r) {
		return
	}
	// vários lotes podem passar do limite de escrita padrão do servidor
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Minute))
	ctx, cancelar := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancelar()
	rel, err := financeiro.CategorizarComIA(ctx, s.IA, func(fn func(ctx context.Context, tx pgx.Tx) error) error {
		return s.comUsuario(r, fn)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, rel)
}

// chatIA: "Pergunte às suas finanças". O histórico fica no navegador; o servidor não
// guarda as conversas.
func (s *Servidor) chatIA(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Mensagens []ia.Mensagem `json:"mensagens"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if len(c.Mensagens) == 0 || len(c.Mensagens) > 20 || c.Mensagens[len(c.Mensagens)-1].Papel != "usuario" {
		falhar(w, r, invalido("envie de 1 a 20 mensagens, a última da pessoa"))
		return
	}
	for _, m := range c.Mensagens {
		if (m.Papel != "usuario" && m.Papel != "assistente") || strings.TrimSpace(m.Texto) == "" || len([]rune(m.Texto)) > 4000 {
			falhar(w, r, invalido("mensagem vazia, longa demais ou de papel inválido"))
			return
		}
	}
	if !s.exigirIA(w, r) {
		return
	}
	hoje := financeiro.Hoje()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Minute))
	ctx, cancelar := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancelar()
	resp, err := s.IA.Conversar(ctx, hoje.Format("2006-01-02"), c.Mensagens, s.ferramentasChat(r))
	if errGrava := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.RegistrarUsoIA(ctx, tx, hoje, resp.Uso)
	}); errGrava != nil && err == nil {
		err = errGrava
	}
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Ferramentas do chat: só leitura, cada chamada numa transação com o RLS do usuário
// ---------------------------------------------------------------------------

type periodoIA struct {
	Mes string `json:"mes"`
	De  string `json:"de"`
	Ate string `json:"ate"`
}

// resolver: mês "AAAA-MM" (até hoje, se for o mês corrente, igual à tela) ou de/até.
func (p periodoIA) resolver(hoje time.Time) (time.Time, time.Time, error) {
	if p.Mes != "" {
		de, ate, _, _, err := PeriodoMes(p.Mes, hoje)
		return de, ate, err
	}
	de, err1 := time.Parse("2006-01-02", p.De)
	ate, err2 := time.Parse("2006-01-02", p.Ate)
	if err1 != nil || err2 != nil || ate.Before(de) || ate.Sub(de) > 5*366*24*time.Hour {
		return de, ate, errors.New("informe mes (AAAA-MM) ou de e ate (AAAA-MM-DD), até 5 anos")
	}
	return de, ate, nil
}

var esquemaPeriodo = map[string]any{
	"mes": map[string]any{"type": "string", "description": "mês AAAA-MM (no mês corrente, vai até hoje, como na tela)"},
	"de":  map[string]any{"type": "string", "description": "início AAAA-MM-DD, se não usar mes"},
	"ate": map[string]any{"type": "string", "description": "fim AAAA-MM-DD, se não usar mes"},
}

func reais(c int64) string { return core.FormatarBRL(core.Centavos(c)) }

func (s *Servidor) ferramentasChat(r *http.Request) []ia.Ferramenta {
	hoje := financeiro.Hoje()
	// ler roda fn com as entidades de que a pessoa é dona (as mesmas das telas)
	ler := func(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx, ents []string) (any, error)) (any, error) {
		var out any
		err := s.comUsuario(r, func(c context.Context, tx pgx.Tx) error {
			ents, err := financeiro.Entidades(c, tx, "")
			if err != nil {
				return err
			}
			out, err = fn(c, tx, ents)
			return err
		})
		return out, err
	}
	return []ia.Ferramenta{
		{
			Nome:       "gastos_por_categoria",
			Descricao:  "Gastos por categoria no período (os mesmos números da tela Gastos). Transferências entre contas próprias não contam; estornos abatem.",
			Parametros: esquemaPeriodo,
			Rodar: func(ctx context.Context, e json.RawMessage) (any, error) {
				var p periodoIA
				_ = json.Unmarshal(e, &p)
				de, ate, err := p.resolver(hoje)
				if err != nil {
					return nil, err
				}
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					cats, err := financeiro.GastosPorCategoria(ctx, tx, ents, de, ate)
					if err != nil {
						return nil, err
					}
					type item struct {
						Categoria string `json:"categoria"`
						Total     string `json:"total"`
						Centavos  int64  `json:"centavos"`
					}
					var total int64
					lista := []item{}
					for _, c := range cats {
						total += c.Total
						lista = append(lista, item{c.Nome, reais(c.Total), c.Total})
					}
					return map[string]any{"de": de.Format("2006-01-02"), "ate": ate.Format("2006-01-02"),
						"total": reais(total), "categorias": lista}, nil
				})
			},
		},
		{
			Nome:       "resumo_periodo",
			Descricao:  "Total de receitas e de gastos no período e a diferença.",
			Parametros: esquemaPeriodo,
			Rodar: func(ctx context.Context, e json.RawMessage) (any, error) {
				var p periodoIA
				_ = json.Unmarshal(e, &p)
				de, ate, err := p.resolver(hoje)
				if err != nil {
					return nil, err
				}
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					rec, gas, err := financeiro.TotaisPeriodo(ctx, tx, ents, de, ate)
					return map[string]any{"de": de.Format("2006-01-02"), "ate": ate.Format("2006-01-02"),
						"receitas": reais(rec), "gastos": reais(gas), "diferenca": reais(rec - gas)}, err
				})
			},
		},
		{
			Nome:      "buscar_transacoes",
			Descricao: "Procura transações pela descrição e/ou pelo nome da categoria no período (padrão: últimos 90 dias). Devolve data, descrição, valor, conta e categoria.",
			Parametros: map[string]any{
				"texto":     map[string]any{"type": "string", "description": "trecho da descrição, ex.: uber, ifood"},
				"categoria": map[string]any{"type": "string", "description": "nome exato da categoria ou da mãe"},
				"de":        map[string]any{"type": "string"}, "ate": map[string]any{"type": "string"},
				"limite": map[string]any{"type": "integer", "description": "até 100"},
			},
			Rodar: func(ctx context.Context, e json.RawMessage) (any, error) {
				var p struct {
					Texto, Categoria, De, Ate string
					Limite                    int
				}
				_ = json.Unmarshal(e, &p)
				de, ate := hoje.AddDate(0, 0, -90), hoje
				if p.De != "" || p.Ate != "" {
					var err error
					if de, ate, err = (periodoIA{De: p.De, Ate: p.Ate}).resolver(hoje); err != nil {
						return nil, err
					}
				}
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					lista, err := financeiro.BuscarTransacoes(ctx, tx, ents, p.Texto, p.Categoria, de, ate, min(max(p.Limite, 1), 100))
					if err != nil {
						return nil, err
					}
					type item struct {
						Data, Descricao, Valor, Conta string
						Categoria                     *string
					}
					out := []item{}
					var soma int64
					for _, t := range lista {
						soma += t.Valor
						out = append(out, item{t.Data, t.Descricao, reais(t.Valor), t.Conta, t.Categoria})
					}
					return map[string]any{"de": de.Format("2006-01-02"), "ate": ate.Format("2006-01-02"),
						"quantidade": len(out), "soma": reais(soma), "transacoes": out}, nil
				})
			},
		},
		{
			Nome:       "saldos",
			Descricao:  "Saldo de hoje de cada conta (cartão de crédito aparece como dívida, negativo).",
			Parametros: map[string]any{},
			Rodar: func(ctx context.Context, _ json.RawMessage) (any, error) {
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					linhas, err := tx.Query(ctx, `select c.nome, c.tipo::text, c.moeda, app_saldo_conta(c.id, $2)
						from contas c where c.entidade_id = any($1) and not c.arquivada order by c.nome`, ents, hoje)
					if err != nil {
						return nil, err
					}
					type item struct{ Conta, Tipo, Moeda, Saldo string }
					out := []item{}
					for linhas.Next() {
						var nome, tipo, moeda string
						var saldo int64
						if err := linhas.Scan(&nome, &tipo, &moeda, &saldo); err != nil {
							linhas.Close()
							return nil, err
						}
						out = append(out, item{nome, tipo, moeda, reais(saldo)})
					}
					linhas.Close()
					return out, linhas.Err()
				})
			},
		},
		{
			Nome:       "proximas_contas",
			Descricao:  "Contas a pagar e a receber dos próximos dias (recorrências, faturas, parcelas, DAS) e o menor saldo previsto.",
			Parametros: map[string]any{"dias": map[string]any{"type": "integer", "description": "1 a 90 (padrão 30)"}},
			Rodar: func(ctx context.Context, e json.RawMessage) (any, error) {
				var p struct{ Dias int }
				_ = json.Unmarshal(e, &p)
				dias := min(max(p.Dias, 1), 90)
				if p.Dias == 0 {
					dias = 30
				}
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					ag, err := financeiro.CalcularAgenda(ctx, tx, ents, hoje, dias)
					if err != nil {
						return nil, err
					}
					type item struct{ Data, Descricao, Valor string }
					out := []item{}
					for _, it := range ag.Itens {
						if !it.Pago {
							out = append(out, item{it.Data, it.Descricao, reais(it.Valor)})
						}
					}
					return map[string]any{"itens": out, "saldo_hoje": reais(ag.SaldoInicial),
						"saldo_final": reais(ag.SaldoFinal), "menor_saldo": reais(ag.MenorSaldo.Saldo),
						"dia_do_menor_saldo": ag.MenorSaldo.Data}, nil
				})
			},
		},
		{
			Nome:       "carteira",
			Descricao:  "Investimentos de hoje: total, por classe e por ativo, com custo, rendimento e rentabilidade ao ano contra o CDI.",
			Parametros: map[string]any{},
			Rodar: func(ctx context.Context, _ json.RawMessage) (any, error) {
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					return financeiro.CalcularCarteira(ctx, tx, ents, hoje)
				})
			},
		},
		{
			Nome:       "patrimonio",
			Descricao:  "Patrimônio líquido de hoje (contas, investimentos, bens, menos dívidas e faturas).",
			Parametros: map[string]any{},
			Rodar: func(ctx context.Context, _ json.RawMessage) (any, error) {
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					return financeiro.CalcularPatrimonio(ctx, tx, ents, hoje, false)
				})
			},
		},
		{
			Nome:       "cnpj_resumo",
			Descricao:  "Situação de cada CNPJ: faturamento do ano, teto do MEI e projeção, DAS do mês, Fator R e alertas.",
			Parametros: map[string]any{},
			Rodar: func(ctx context.Context, _ json.RawMessage) (any, error) {
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					out := []financeiro.PainelCNPJ{}
					for _, id := range ents {
						p, err := financeiro.CalcularPainelCNPJ(ctx, tx, id, hoje)
						if errors.Is(err, financeiro.ErrNaoEPJ) {
							continue
						}
						if err != nil {
							return nil, err
						}
						out = append(out, p)
					}
					return out, nil
				})
			},
		},
		{
			Nome: "projetar_fluxo",
			Descricao: "Saldo previsto das contas do dia a dia nos próximos dias: o que já se sabe (contas, faturas, parcelas) " +
				"mais o gasto variável médio (sazonal), com faixa de 80 % de confiança.",
			Parametros: map[string]any{"dias": map[string]any{"type": "integer", "description": "1 a 180 (padrão 90)"}},
			Rodar: func(ctx context.Context, e json.RawMessage) (any, error) {
				var p struct{ Dias int }
				_ = json.Unmarshal(e, &p)
				dias := min(max(p.Dias, 1), 180)
				if p.Dias == 0 {
					dias = 90
				}
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					f, err := financeiro.ProjetarFluxo(ctx, tx, ents, hoje, dias, nil)
					if err != nil {
						return nil, err
					}
					return map[string]any{"saldo_hoje": reais(int64(f.SaldoInicial)), "gasto_variavel_mes": reais(int64(f.VariavelMensal)),
						"saldo_final": reais(int64(f.Final.Base)), "faixa_final": reais(int64(f.Final.Baixo)) + " a " + reais(int64(f.Final.Alto)),
						"menor_saldo": reais(int64(f.Menor.Base)), "dia_do_menor_saldo": f.Menor.Data.Format("2006-01-02")}, nil
				})
			},
		},
		{
			Nome:      "simular_compra",
			Descricao: "Simula uma compra parcelada no fluxo de caixa: cabe folgada, apertada ou não cabe, com o menor saldo com e sem ela.",
			Parametros: map[string]any{
				"valor":    map[string]any{"type": "string", "description": "valor total em reais, ex.: 1200.00"},
				"parcelas": map[string]any{"type": "integer", "description": "1 a 24"},
				"primeira": map[string]any{"type": "string", "description": "vencimento da primeira parcela, AAAA-MM-DD (padrão: daqui a 30 dias)"},
			},
			Requeridos: []string{"valor", "parcelas"},
			Rodar: func(ctx context.Context, e json.RawMessage) (any, error) {
				var p struct {
					Valor, Primeira string
					Parcelas        int
				}
				if err := json.Unmarshal(e, &p); err != nil {
					return nil, err
				}
				valor, err := core.ParseDecimalPonto(p.Valor)
				if err != nil {
					return nil, errors.New("valor em reais, ex.: 1200.00")
				}
				primeira := hoje.AddDate(0, 0, 30)
				if p.Primeira != "" {
					if primeira, err = time.Parse("2006-01-02", p.Primeira); err != nil {
						return nil, errors.New("primeira no formato AAAA-MM-DD")
					}
				}
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					s, err := financeiro.SimularCompra(ctx, tx, ents, hoje, valor, p.Parcelas, primeira)
					if err != nil {
						return nil, err
					}
					return map[string]any{"situacao": s.Situacao, "parcela": reais(int64(s.Parcela)), "primeira": s.PrimeiraData,
						"ultima": s.UltimaData, "menor_saldo_sem": reais(int64(s.Sem.Menor.Base)), "menor_saldo_com": reais(int64(s.Com.Menor.Base)),
						"menor_saldo_com_cenario_ruim": reais(int64(s.Com.MenorBaixo.Baixo))}, nil
				})
			},
		},
		{
			Nome: "monte_carlo",
			Descricao: "Simula o patrimônio investível (carteira + contas) em 5.000 cenários: percentis 10, 50 e 90 ano a ano, " +
				"em dinheiro de hoje, e quando chega à independência financeira (custo de vida ÷ 4 % ao ano).",
			Parametros: map[string]any{
				"aporte": map[string]any{"type": "string", "description": "aporte mensal em reais, ex.: 1500.00"},
				"anos":   map[string]any{"type": "integer", "description": "1 a 50 (padrão 10)"},
				"perfil": map[string]any{"type": "string", "enum": []string{"atual", "conservador", "moderado", "arrojado"},
					"description": "para onde vão os aportes"},
			},
			Rodar: func(ctx context.Context, e json.RawMessage) (any, error) {
				var p struct {
					Aporte, Perfil string
					Anos           int
				}
				_ = json.Unmarshal(e, &p)
				var aporte core.Centavos
				if p.Aporte != "" {
					v, err := core.ParseDecimalPonto(p.Aporte)
					if err != nil {
						return nil, errors.New("aporte em reais, ex.: 1500.00")
					}
					aporte = v
				}
				if p.Anos == 0 {
					p.Anos = 10
				}
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					f, err := financeiro.SimularFuturo(ctx, tx, ents, hoje, financeiro.EntradaFuturo{Aporte: aporte, Anos: p.Anos, Perfil: p.Perfil})
					if err != nil {
						return nil, err
					}
					type ano struct{ Ano, P10, P50, P90, ChanceIndependencia string }
					anos := []ano{}
					for _, pt := range f.Pontos {
						anos = append(anos, ano{strconv.Itoa(pt.Ano), reais(int64(pt.P10)), reais(int64(pt.P50)), reais(int64(pt.P90)),
							strconv.Itoa(int(math.Round(pt.ChanceAlvo*100))) + " %"})
					}
					return map[string]any{"investivel_hoje": reais(int64(f.Investivel)), "aporte_mensal": reais(int64(f.Aporte)),
						"custo_de_vida_mensal": reais(int64(f.Custo)), "patrimonio_para_independencia": reais(int64(f.Alvo)),
						"retorno_real_medio_aa": f.RetornoMedio, "anos": anos, "meses_ate_independencia": f.Cenarios}, nil
				})
			},
		},
		{
			Nome:       "metas",
			Descricao:  "Metas de dinheiro: quanto já tem, o alvo, o aporte necessário por mês e a data prevista.",
			Parametros: map[string]any{},
			Rodar: func(ctx context.Context, _ json.RawMessage) (any, error) {
				return ler(ctx, func(ctx context.Context, tx pgx.Tx, ents []string) (any, error) {
					custo, err := financeiro.CalcularCustoDeVida(ctx, tx, ents, hoje)
					if err != nil {
						return nil, err
					}
					return listarMetas(ctx, tx, ents, hoje, custo.Meses6)
				})
			},
		},
	}
}
