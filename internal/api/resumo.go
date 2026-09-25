package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type gastoCategoria struct {
	CategoriaID *string `json:"categoria_id"`
	Nome        string  `json:"nome"`
	Cor         *string `json:"cor"`
	Total       int64   `json:"total_centavos"`
}

type resumoMes struct {
	Mes               string           `json:"mes"`
	Ate               string           `json:"ate"`
	SaldoTotal        int64            `json:"saldo_total_centavos"`
	Contas            []conta          `json:"contas"`
	Receitas          int64            `json:"receitas_centavos"`
	Gastos            int64            `json:"gastos_centavos"`
	GastosMesAnterior int64            `json:"gastos_mes_anterior_mesmo_periodo_centavos"`
	PorCategoria      []gastoCategoria `json:"gastos_por_categoria"`
	SemCategoria      int              `json:"sem_categoria"`
	Ultimas           []transacao      `json:"ultimas"`
}

// PeriodoMes devolve o primeiro dia do mês, o dia de corte (hoje, se for o mês
// corrente) e o mesmo intervalo no mês anterior, para comparar "até hoje com até
// o mesmo dia do mês passado".
func PeriodoMes(mes string, hoje time.Time) (inicio, ate, inicioAnt, ateAnt time.Time, err error) {
	inicio, err = time.Parse("2006-01", mes)
	if err != nil {
		return
	}
	fim := inicio.AddDate(0, 1, -1)
	hojeDia := time.Date(hoje.Year(), hoje.Month(), hoje.Day(), 0, 0, 0, 0, time.UTC)
	ate = fim
	if !hojeDia.Before(inicio) && !hojeDia.After(fim) {
		ate = hojeDia
	}
	inicioAnt = inicio.AddDate(0, -1, 0)
	fimAnt := inicio.AddDate(0, 0, -1)
	ateAnt = inicioAnt.AddDate(0, 0, ate.Day()-1)
	if ateAnt.After(fimAnt) {
		ateAnt = fimAnt
	}
	return
}

var fusoSP, _ = time.LoadLocation("America/Sao_Paulo")

// resumo: "como estou este mês?" — só das entidades de que o usuário é dono (ou de
// uma entidade escolhida). Transferências entre contas não entram como gasto nem
// receita; estornos abatem o gasto.
func (s *Servidor) resumo(w http.ResponseWriter, r *http.Request) {
	agora := time.Now().In(fusoSP)
	mes := r.URL.Query().Get("mes")
	if mes == "" {
		mes = agora.Format("2006-01")
	}
	inicio, ate, inicioAnt, ateAnt, err := PeriodoMes(mes, agora)
	if err != nil {
		falhar(w, r, invalido("mês no formato AAAA-MM"))
		return
	}
	res := resumoMes{Mes: mes, Ate: ate.Format("2006-01-02")}
	entidade := r.URL.Query().Get("entidade_id")

	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		// entidades do resumo
		filtro := "e.dono_id = app_usuario_id()"
		args := []any{}
		if entidade != "" {
			var ok bool
			if err := tx.QueryRow(ctx, "select true from entidades where id = $1", entidade).Scan(&ok); err != nil {
				return err // RLS: entidade alheia vira 404
			}
			filtro, args = "e.id = $1", []any{entidade}
		}
		ents := "(select e.id from entidades e where " + filtro + ")"

		linhas, err := tx.Query(ctx, consultaContas+" where c.entidade_id in "+ents+" and not c.arquivada order by e.nome, c.nome", args...)
		if err != nil {
			return err
		}
		if res.Contas, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[conta]); err != nil {
			return err
		}
		for _, c := range res.Contas {
			if c.Moeda == "BRL" && c.Saldo != nil {
				res.SaldoTotal += *c.Saldo
			}
		}

		n := len(args)
		periodo := func(de, ate time.Time) []any { return append(append([]any{}, args...), de, ate) }
		p1, p2 := "$"+strconv.Itoa(n+1), "$"+strconv.Itoa(n+2)
		somas := `select
			coalesce(sum(valor_centavos) filter (where tipo = 'receita'), 0),
			coalesce(-sum(valor_centavos) filter (where tipo in ('gasto', 'estorno')), 0)
			from transacoes t where t.moeda = 'BRL' and t.entidade_id in ` + ents + ` and t.data between ` + p1 + ` and ` + p2
		if err := tx.QueryRow(ctx, somas, periodo(inicio, ate)...).Scan(&res.Receitas, &res.Gastos); err != nil {
			return err
		}
		var receitasAnt int64
		if err := tx.QueryRow(ctx, somas, periodo(inicioAnt, ateAnt)...).Scan(&receitasAnt, &res.GastosMesAnterior); err != nil {
			return err
		}

		linhas, err = tx.Query(ctx, `select coalesce(pai.id, cat.id), coalesce(pai.nome, cat.nome, 'Sem categoria'),
				coalesce(pai.cor, cat.cor), -sum(t.valor_centavos) as total
			from transacoes t
			left join categorias cat on cat.id = t.categoria_id
			left join categorias pai on pai.id = cat.pai_id
			where t.tipo in ('gasto', 'estorno') and t.moeda = 'BRL' and t.entidade_id in `+ents+`
			  and t.data between `+p1+` and `+p2+`
			group by 1, 2, 3 having -sum(t.valor_centavos) > 0 order by total desc`, periodo(inicio, ate)...)
		if err != nil {
			return err
		}
		if res.PorCategoria, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[gastoCategoria]); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `select count(*) from transacoes t where t.categoria_id is null
			and t.tipo in ('gasto', 'receita') and t.entidade_id in `+ents, args...).Scan(&res.SemCategoria); err != nil {
			return err
		}
		linhas, err = tx.Query(ctx, consultaTransacoes+" where t.entidade_id in "+ents+
			" order by t.data desc, t.criada_em desc limit 8", args...)
		if err != nil {
			return err
		}
		res.Ultimas, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[transacao])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if res.Contas == nil {
		res.Contas = []conta{}
	}
	if res.PorCategoria == nil {
		res.PorCategoria = []gastoCategoria{}
	}
	if res.Ultimas == nil {
		res.Ultimas = []transacao{}
	}
	escreverJSON(w, http.StatusOK, res)
}
