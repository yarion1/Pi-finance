package financeiro

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// ItemOrcamento: uma categoria do orçamento do mês.
type ItemOrcamento struct {
	CategoriaID   string  `json:"categoria_id"`
	Nome          string  `json:"nome"`
	Pai           *string `json:"pai"`
	Cor           *string `json:"cor"`
	Limite        int64   `json:"limite_centavos"`
	SobraAnterior int64   `json:"sobra_anterior_centavos"`
	Disponivel    int64   `json:"disponivel_centavos"`
	Gasto         int64   `json:"gasto_centavos"`
	Ritmo         int64   `json:"ritmo_ideal_centavos"`
	Situacao      string  `json:"situacao"`
	Media3        int64   `json:"media_3_meses_centavos"`
}

// Grupo503020: alvo e valor de um grupo da regra 50/30/20.
type Grupo503020 struct {
	Alvo  int64 `json:"alvo_centavos"`
	Valor int64 `json:"valor_centavos"`
}

// Orcamento do mês de uma entidade.
type Orcamento struct {
	EntidadeID   string            `json:"entidade_id"`
	Mes          string            `json:"mes"`
	Modo         string            `json:"modo"`
	Grupos       map[string]string `json:"grupos"`
	DiasNoMes    int               `json:"dias_no_mes"`
	Dia          int               `json:"dia"`
	Itens        []ItemOrcamento   `json:"itens"`
	SemOrcamento []ItemOrcamento   `json:"sem_orcamento"`
	Totais       struct {
		Limite     int64 `json:"limite_centavos"`
		Disponivel int64 `json:"disponivel_centavos"`
		Gasto      int64 `json:"gasto_centavos"`
		Ritmo      int64 `json:"ritmo_ideal_centavos"`
	} `json:"totais"`
	Regra503020 *struct {
		Renda        int64       `json:"renda_centavos"`
		Necessidades Grupo503020 `json:"necessidades"`
		Desejos      Grupo503020 `json:"desejos"`
		Poupanca     Grupo503020 `json:"poupanca"`
	} `json:"regra_50_30_20"`
}

type infoCategoria struct {
	nome, tipo string
	pai        *string
	cor        *string
}

// EntidadeDoOrcamento: a pedida ou a PF do usuário (o orçamento é por entidade).
func EntidadeDoOrcamento(ctx context.Context, tx pgx.Tx, pedida string) (string, error) {
	if pedida != "" {
		ents, err := Entidades(ctx, tx, pedida)
		if err != nil {
			return "", err
		}
		return ents[0], nil
	}
	var id string
	err := tx.QueryRow(ctx, `select id from entidades where dono_id = app_usuario_id()
		order by tipo = 'PF' desc, criada_em limit 1`).Scan(&id)
	return id, err
}

// gastosPorCategoriaID de um período: cada categoria e, somado, na categoria mãe.
func gastosPorCategoriaID(ctx context.Context, tx pgx.Tx, entidade string, de, ate time.Time) (map[string]int64, error) {
	linhas, err := tx.Query(ctx, `select t.categoria_id, cat.pai_id, -sum(t.valor_centavos) from transacoes t
		join categorias cat on cat.id = t.categoria_id
		where t.entidade_id = $1 and t.tipo in ('gasto', 'estorno') and t.data between $2 and $3
		group by 1, 2`, entidade, de, ate)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	out := map[string]int64{}
	for linhas.Next() {
		var cat string
		var pai *string
		var v int64
		if err := linhas.Scan(&cat, &pai, &v); err != nil {
			return nil, err
		}
		out[cat] += v
		if pai != nil {
			out[*pai] += v
		}
	}
	return out, linhas.Err()
}

// grupoPadrao da regra 50/30/20 quando o usuário não escolheu.
func grupoPadrao(nome string, pai *string) string {
	raiz := nome
	if pai != nil {
		raiz = *pai
	}
	switch {
	case nome == "Mercado" || nome == "Farmácia" || nome == "Plano de saúde":
		return "necessidades"
	case raiz == "Alimentação" || raiz == "Lazer" || raiz == "Compras" || raiz == "Pessoal" || raiz == "Outros gastos":
		return "desejos"
	default:
		return "necessidades"
	}
}

// CalcularOrcamento do mês ("AAAA-MM", vazio = o de hoje) da entidade pedida (vazia = a PF
// do usuário): limites, sobras dos envelopes, gasto, ritmo e a regra 50/30/20.
func CalcularOrcamento(ctx context.Context, tx pgx.Tx, entidadePedida, mes string, hoje time.Time) (Orcamento, error) {
	if mes == "" {
		mes = hoje.Format("2006-01")
	}
	inicio, err := time.Parse("2006-01", mes)
	if err != nil {
		return Orcamento{}, ErrCampo{"mês no formato AAAA-MM"}
	}
	fim := inicio.AddDate(0, 1, -1)
	resp := Orcamento{Mes: mes, DiasNoMes: core.DiasNoMes(inicio.Year(), inicio.Month())}
	switch {
	case hoje.Before(inicio):
		resp.Dia = 0
	case hoje.After(fim):
		resp.Dia = resp.DiasNoMes
	default:
		resp.Dia = hoje.Day()
	}

	err = func() error {
		ent, err := EntidadeDoOrcamento(ctx, tx, entidadePedida)
		if err != nil {
			return err
		}
		resp.EntidadeID = ent
		var grupos []byte
		err = tx.QueryRow(ctx, "select modo, grupos from orcamento_config where entidade_id = $1", ent).Scan(&resp.Modo, &grupos)
		if errors.Is(err, pgx.ErrNoRows) {
			resp.Modo, grupos, err = "categoria", []byte("{}"), nil
		}
		if err != nil {
			return err
		}
		_ = json.Unmarshal(grupos, &resp.Grupos)

		cats := map[string]infoCategoria{}
		linhas, err := tx.Query(ctx, `select c.id, c.nome, c.tipo::text, p.nome, coalesce(c.cor, p.cor) from categorias c
			left join categorias p on p.id = c.pai_id where c.entidade_id is null or c.entidade_id = $1`, ent)
		if err != nil {
			return err
		}
		for linhas.Next() {
			var id string
			var c infoCategoria
			if err := linhas.Scan(&id, &c.nome, &c.tipo, &c.pai, &c.cor); err != nil {
				linhas.Close()
				return err
			}
			cats[id] = c
		}
		linhas.Close()

		gastos, err := gastosPorCategoriaID(ctx, tx, ent, inicio, fim)
		if err != nil {
			return err
		}
		media, err := gastosPorCategoriaID(ctx, tx, ent, inicio.AddDate(0, -3, 0), inicio.AddDate(0, 0, -1))
		if err != nil {
			return err
		}

		// limites do mês e, nos envelopes, a história para calcular o que sobrou
		type limiteMes struct {
			mes   time.Time
			cat   string
			valor int64
		}
		linhas, err = tx.Query(ctx, `select mes, categoria_id, limite_centavos from orcamentos
			where entidade_id = $1 and mes between $2 and $3 order by mes`, ent, inicio.AddDate(0, -24, 0), inicio)
		if err != nil {
			return err
		}
		var historico []limiteMes
		for linhas.Next() {
			var l limiteMes
			if err := linhas.Scan(&l.mes, &l.cat, &l.valor); err != nil {
				linhas.Close()
				return err
			}
			historico = append(historico, l)
		}
		linhas.Close()

		sobra := map[string]int64{}
		if resp.Modo == "envelopes" {
			var primeiro time.Time
			for _, l := range historico {
				if primeiro.IsZero() || l.mes.Before(primeiro) {
					primeiro = l.mes
				}
			}
			for m := primeiro; !primeiro.IsZero() && m.Before(inicio); m = m.AddDate(0, 1, 0) {
				gm, err := gastosPorCategoriaID(ctx, tx, ent, m, m.AddDate(0, 1, -1))
				if err != nil {
					return err
				}
				for _, l := range historico {
					if l.mes.Equal(m) {
						sobra[l.cat] += l.valor - gm[l.cat]
					}
				}
			}
		}

		orcadas := map[string]bool{}
		for _, l := range historico {
			if !l.mes.Equal(inicio) {
				continue
			}
			c, ok := cats[l.cat]
			if !ok {
				continue
			}
			orcadas[l.cat] = true
			it := ItemOrcamento{CategoriaID: l.cat, Nome: c.nome, Pai: c.pai, Cor: c.cor, Limite: l.valor,
				SobraAnterior: sobra[l.cat], Gasto: gastos[l.cat], Media3: media[l.cat] / 3}
			it.Disponivel = it.Limite + it.SobraAnterior
			it.Ritmo = int64(core.RitmoIdeal(core.Centavos(it.Disponivel), resp.Dia, resp.DiasNoMes))
			it.Situacao = core.SituacaoOrcamento(core.Centavos(it.Disponivel), core.Centavos(it.Gasto), core.Centavos(it.Ritmo))
			resp.Itens = append(resp.Itens, it)
			resp.Totais.Limite += it.Limite
			resp.Totais.Disponivel += it.Disponivel
			resp.Totais.Ritmo += it.Ritmo
		}
		// gasto total do mês e categorias mães com gasto e sem orçamento
		for id, v := range gastos {
			c := cats[id]
			if c.pai != nil || v <= 0 {
				continue
			}
			resp.Totais.Gasto += v
			coberta := orcadas[id]
			for oid := range orcadas {
				if p := cats[oid].pai; p != nil && *p == c.nome {
					coberta = true
				}
			}
			if !coberta {
				resp.SemOrcamento = append(resp.SemOrcamento, ItemOrcamento{CategoriaID: id, Nome: c.nome, Cor: c.cor, Gasto: v, Media3: media[id] / 3})
			}
		}
		// sem categoria também é gasto
		var semCat int64
		if err := tx.QueryRow(ctx, `select coalesce(-sum(valor_centavos), 0) from transacoes where entidade_id = $1
			and tipo in ('gasto', 'estorno') and categoria_id is null and data between $2 and $3`, ent, inicio, fim).Scan(&semCat); err != nil {
			return err
		}
		resp.Totais.Gasto += semCat

		if resp.Modo == "50_30_20" {
			var renda int64
			if err := tx.QueryRow(ctx, `select coalesce(sum(valor_centavos), 0) from transacoes where entidade_id = $1
				and tipo = 'receita' and data between $2 and $3`, ent, inicio, fim).Scan(&renda); err != nil {
				return err
			}
			reg := &struct {
				Renda        int64       `json:"renda_centavos"`
				Necessidades Grupo503020 `json:"necessidades"`
				Desejos      Grupo503020 `json:"desejos"`
				Poupanca     Grupo503020 `json:"poupanca"`
			}{Renda: renda}
			reg.Necessidades.Alvo, reg.Desejos.Alvo, reg.Poupanca.Alvo = renda*50/100, renda*30/100, renda*20/100
			for id, v := range gastos {
				c := cats[id]
				// só as folhas (filhas, ou mães sem filhas com gasto) para não somar duas vezes
				if c.pai == nil && temFilhaComGasto(id, c.nome, cats, gastos) {
					continue
				}
				g := resp.Grupos[id]
				if g == "" && c.pai != nil {
					for pid, pc := range cats {
						if pc.nome == *c.pai && pc.pai == nil {
							g = resp.Grupos[pid]
						}
					}
				}
				if g == "" {
					g = grupoPadrao(c.nome, c.pai)
				}
				if g == "desejos" {
					reg.Desejos.Valor += v
				} else {
					reg.Necessidades.Valor += v
				}
			}
			reg.Necessidades.Valor += semCat
			reg.Poupanca.Valor = renda - reg.Necessidades.Valor - reg.Desejos.Valor
			resp.Regra503020 = reg
		}
		return nil
	}()
	if err != nil {
		return resp, err
	}
	if resp.Itens == nil {
		resp.Itens = []ItemOrcamento{}
	}
	if resp.SemOrcamento == nil {
		resp.SemOrcamento = []ItemOrcamento{}
	}
	if resp.Grupos == nil {
		resp.Grupos = map[string]string{}
	}
	return resp, nil
}

func temFilhaComGasto(id, nome string, cats map[string]infoCategoria, gastos map[string]int64) bool {
	for cid, c := range cats {
		if c.pai != nil && *c.pai == nome && gastos[cid] != 0 && cid != id {
			return true
		}
	}
	return false
}
