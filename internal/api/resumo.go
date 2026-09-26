package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

type resumoMes struct {
	Mes               string                      `json:"mes"`
	Ate               string                      `json:"ate"`
	SaldoTotal        int64                       `json:"saldo_total_centavos"`
	Contas            []conta                     `json:"contas"`
	Receitas          int64                       `json:"receitas_centavos"`
	Gastos            int64                       `json:"gastos_centavos"`
	GastosMesAnterior int64                       `json:"gastos_mes_anterior_mesmo_periodo_centavos"`
	PorCategoria      []financeiro.GastoCategoria `json:"gastos_por_categoria"`
	SemCategoria      int                         `json:"sem_categoria"`
	Ultimas           []transacao                 `json:"ultimas"`
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
// hojeSQL: a data de hoje no fuso de São Paulo, dentro do banco.
const hojeSQL = "(now() at time zone 'America/Sao_Paulo')::date"

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

		lista, err := tx.Query(ctx, "select e.id from entidades e where "+filtro, args...)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(lista, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if res.Receitas, res.Gastos, err = financeiro.TotaisPeriodo(ctx, tx, ids, inicio, ate); err != nil {
			return err
		}
		if _, res.GastosMesAnterior, err = financeiro.TotaisPeriodo(ctx, tx, ids, inicioAnt, ateAnt); err != nil {
			return err
		}
		// a mesma consulta que a IA usa (gastos_por_categoria)
		if res.PorCategoria, err = financeiro.GastosPorCategoria(ctx, tx, ids, inicio, ate); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `select count(*) from transacoes t where t.categoria_id is null
			and t.tipo in ('gasto', 'receita') and t.data <= `+hojeSQL+` and t.entidade_id in `+ents, args...).Scan(&res.SemCategoria); err != nil {
			return err
		}
		// parcelas futuras já lançadas não são "últimas"
		linhas, err = tx.Query(ctx, consultaTransacoes+" where t.entidade_id in "+ents+" and t.data <= "+hojeSQL+
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
		res.PorCategoria = []financeiro.GastoCategoria{}
	}
	if res.Ultimas == nil {
		res.Ultimas = []transacao{}
	}
	escreverJSON(w, http.StatusOK, res)
}
