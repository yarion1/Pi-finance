package financeiro

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/ia"
)

// Totais de um período.
type Totais struct {
	Receitas int64 `json:"receitas_centavos"`
	Gastos   int64 `json:"gastos_centavos"`
	Saldo    int64 `json:"saldo_centavos"`
}

func totais(ctx context.Context, tx pgx.Tx, ents []string, de, ate time.Time) (Totais, error) {
	r, g, err := TotaisPeriodo(ctx, tx, ents, de, ate)
	return Totais{Receitas: r, Gastos: g, Saldo: r - g}, err
}

// CategoriaMes: gasto da categoria no mês e a média dos 3 anteriores.
type CategoriaMes struct {
	Nome   string  `json:"nome"`
	Cor    *string `json:"cor"`
	Total  int64   `json:"total_centavos"`
	Media3 int64   `json:"media_3_meses_centavos"`
}

// DiaAcumulado: gasto acumulado até o dia, no mês e no mês anterior.
type DiaAcumulado struct {
	Dia      int   `json:"dia"`
	Mes      int64 `json:"mes_centavos"`
	Anterior int64 `json:"anterior_centavos"`
}

// RelatorioMes: o que mudou, 3 gráficos (categorias, gasto acumulado, patrimônio) e 3 ações.
type RelatorioMes struct {
	Mes          string            `json:"mes"`
	Totais       Totais            `json:"totais"`
	Anterior     Totais            `json:"anterior"`
	Categorias   []CategoriaMes    `json:"categorias"`
	Acumulado    []DiaAcumulado    `json:"acumulado"`
	Patrimonio   []PontoPatrimonio `json:"patrimonio"`
	Acoes        []string          `json:"acoes"`
	SemCategoria int               `json:"sem_categoria"`
}

// MontarRelatorioMes das entidades do usuário da sessão para o mês (primeiro dia).
func MontarRelatorioMes(ctx context.Context, tx pgx.Tx, mes, hoje time.Time) (RelatorioMes, error) {
	r := RelatorioMes{Mes: mes.Format("2006-01")}
	ents, err := Entidades(ctx, tx, "")
	if err != nil {
		return r, err
	}
	fim := mes.AddDate(0, 1, -1)
	anterior := mes.AddDate(0, -1, 0)
	if r.Totais, err = totais(ctx, tx, ents, mes, fim); err != nil {
		return r, err
	}
	if r.Anterior, err = totais(ctx, tx, ents, anterior, mes.AddDate(0, 0, -1)); err != nil {
		return r, err
	}

	// categorias do mês contra a média dos 3 anteriores
	atual, err := GastosPorCategoria(ctx, tx, ents, mes, fim)
	if err != nil {
		return r, err
	}
	antes, err := GastosPorCategoria(ctx, tx, ents, mes.AddDate(0, -3, 0), mes.AddDate(0, 0, -1))
	if err != nil {
		return r, err
	}
	media := map[string]int64{}
	for _, c := range antes {
		media[c.Nome] += c.Total / 3
	}
	entrada := core.EntradaAcoes{Receitas: core.Centavos(r.Totais.Receitas), Gastos: core.Centavos(r.Totais.Gastos)}
	for _, c := range atual {
		r.Categorias = append(r.Categorias, CategoriaMes{Nome: c.Nome, Cor: c.Cor, Total: c.Total, Media3: media[c.Nome]})
		if alta := c.Total - media[c.Nome]; c.Nome != "Sem categoria" && media[c.Nome] > 0 && core.Centavos(alta) > entrada.AltaCategoria {
			entrada.CategoriaQueSubiu, entrada.AltaCategoria = c.Nome, core.Centavos(alta)
			entrada.AltaPercentual = float64(alta) / float64(media[c.Nome])
		}
	}
	if len(r.Categorias) > 8 {
		r.Categorias = r.Categorias[:8]
	}
	if r.Categorias == nil {
		r.Categorias = []CategoriaMes{}
	}

	// gasto acumulado dia a dia, contra o mês anterior
	porDia := func(de time.Time) (map[int]int64, error) {
		linhas, err := tx.Query(ctx, `select extract(day from data)::int, -sum(valor_centavos) from transacoes
			where entidade_id = any($1) and tipo in ('gasto', 'estorno') and moeda = 'BRL' and data between $2 and $3
			group by 1`, ents, de, de.AddDate(0, 1, -1))
		if err != nil {
			return nil, err
		}
		defer linhas.Close()
		out := map[int]int64{}
		for linhas.Next() {
			var d int
			var v int64
			if err := linhas.Scan(&d, &v); err != nil {
				return nil, err
			}
			out[d] = v
		}
		return out, linhas.Err()
	}
	dm, err := porDia(mes)
	if err != nil {
		return r, err
	}
	da, err := porDia(anterior)
	if err != nil {
		return r, err
	}
	var am, aa int64
	diasAnterior := core.DiasNoMes(anterior.Year(), anterior.Month())
	for d := 1; d <= core.DiasNoMes(mes.Year(), mes.Month()); d++ {
		am += dm[d]
		if d <= diasAnterior {
			aa += da[d]
		}
		r.Acumulado = append(r.Acumulado, DiaAcumulado{Dia: d, Mes: am, Anterior: aa})
	}

	// patrimônio dos últimos meses
	p, err := CalcularPatrimonio(ctx, tx, ents, hoje, true)
	if err != nil {
		return r, err
	}
	r.Patrimonio = p.Serie
	if len(r.Patrimonio) > 6 {
		r.Patrimonio = r.Patrimonio[len(r.Patrimonio)-6:]
	}

	// orçamento estourado (da PF), reserva, assinaturas e sem categoria
	if orc, err := CalcularOrcamento(ctx, tx, "", r.Mes, hoje); err == nil {
		for _, it := range orc.Itens {
			if it.Situacao == "estourado" {
				entrada.Estouradas = append(entrada.Estouradas, it.Nome)
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return r, err
	}
	custo, err := CalcularCustoDeVida(ctx, tx, ents, hoje)
	if err != nil {
		return r, err
	}
	entrada.CustoMedio, entrada.Reserva = core.Centavos(custo.Meses6), core.Centavos(p.Contas)
	recs, err := Recorrencias(ctx, tx, ents, hoje)
	if err != nil {
		return r, err
	}
	for _, rc := range recs {
		if rc.Ativa && rc.Tipo == "assinatura" && rc.Valor < 0 {
			entrada.Assinaturas += core.Centavos(-rc.Valor)
		}
	}
	if err := tx.QueryRow(ctx, `select count(*) from transacoes where entidade_id = any($1) and categoria_id is null
		and tipo in ('gasto', 'receita') and data between $2 and $3`, ents, mes, fim).Scan(&r.SemCategoria); err != nil {
		return r, err
	}
	entrada.SemCategoria = r.SemCategoria
	r.Acoes = core.SugerirAcoes(entrada)
	if r.Acoes == nil {
		r.Acoes = []string{}
	}
	return r, nil
}

// ResumoSemana: gasto da semana, orçamento que resta no mês e as contas dos próximos 7 dias.
type ResumoSemana struct {
	Inicio     string           `json:"inicio"`
	Fim        string           `json:"fim"`
	Gasto      int64            `json:"gasto_centavos"`
	Anterior   int64            `json:"semana_anterior_centavos"`
	Categorias []GastoCategoria `json:"categorias"`
	Orcamento  *struct {
		Mes        string `json:"mes"`
		Disponivel int64  `json:"disponivel_centavos"`
		Gasto      int64  `json:"gasto_centavos"`
	} `json:"orcamento"`
	Proximas []ItemAgenda `json:"proximas"`
}

// MontarResumoSemana da semana que começa na segunda-feira `inicio`.
func MontarResumoSemana(ctx context.Context, tx pgx.Tx, inicio, hoje time.Time) (ResumoSemana, error) {
	fim := inicio.AddDate(0, 0, 6)
	r := ResumoSemana{Inicio: inicio.Format(formatoData), Fim: fim.Format(formatoData)}
	ents, err := Entidades(ctx, tx, "")
	if err != nil {
		return r, err
	}
	if _, r.Gasto, err = TotaisPeriodo(ctx, tx, ents, inicio, fim); err != nil {
		return r, err
	}
	if _, r.Anterior, err = TotaisPeriodo(ctx, tx, ents, inicio.AddDate(0, 0, -7), inicio.AddDate(0, 0, -1)); err != nil {
		return r, err
	}
	if r.Categorias, err = GastosPorCategoria(ctx, tx, ents, inicio, fim); err != nil {
		return r, err
	}
	if len(r.Categorias) > 3 {
		r.Categorias = r.Categorias[:3]
	}
	orc, err := CalcularOrcamento(ctx, tx, "", fim.Format("2006-01"), hoje)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return r, err
	}
	if err == nil && orc.Totais.Disponivel > 0 {
		r.Orcamento = &struct {
			Mes        string `json:"mes"`
			Disponivel int64  `json:"disponivel_centavos"`
			Gasto      int64  `json:"gasto_centavos"`
		}{orc.Mes, orc.Totais.Disponivel, orc.Totais.Gasto}
	}
	ag, err := CalcularAgenda(ctx, tx, ents, hoje, 7)
	if err != nil {
		return r, err
	}
	r.Proximas = []ItemAgenda{}
	for _, it := range ag.Itens {
		if !it.Pago {
			r.Proximas = append(r.Proximas, it)
		}
	}
	return r, nil
}

// RelatorioSalvo: a linha da tabela relatorios.
type RelatorioSalvo struct {
	ID       string          `json:"id"`
	Tipo     string          `json:"tipo"`
	Periodo  string          `json:"periodo"`
	Dados    json.RawMessage `json:"dados,omitempty"`
	Texto    *string         `json:"texto"`
	CriadoEm time.Time       `json:"criado_em"`
}

// salvarRelatorio grava (se ainda não existe) e avisa no início.
func salvarRelatorio(ctx context.Context, tx pgx.Tx, tipo, periodo string, dados any, texto *string) (string, bool, error) {
	j, err := json.Marshal(dados)
	if err != nil {
		return "", false, err
	}
	var id string
	err = tx.QueryRow(ctx, `insert into relatorios (usuario_id, tipo, periodo, dados, texto)
		values (app_usuario_id(), $1, $2, $3, $4) on conflict (usuario_id, tipo, periodo) do nothing returning id`,
		tipo, periodo, j, texto).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, alertar(ctx, tx, "relatorio_pronto", map[string]any{"relatorio_id": id, "tipo": tipo, "periodo": periodo},
		"relatorio_id")
}

// existeRelatorio do usuário da sessão.
func existeRelatorio(ctx context.Context, tx pgx.Tx, tipo, periodo string) (bool, error) {
	var existe bool
	err := tx.QueryRow(ctx, `select exists (select 1 from relatorios where usuario_id = app_usuario_id()
		and tipo = $1 and periodo = $2)`, tipo, periodo).Scan(&existe)
	return existe, err
}

// GerarRelatorios cria o relatório do mês fechado e o resumo da última semana que ainda
// faltam para o usuário (comUsuario abre a transação com ele). Período sem movimento não
// vira relatório. Com a IA ligada e dentro do teto, o relatório do mês ganha um texto.
func GerarRelatorios(ctx context.Context, cliente *ia.Cliente, agora time.Time,
	comUsuario func(func(ctx context.Context, tx pgx.Tx) error) error) (novos []string, err error) {
	hoje := dia(agora)
	mes := core.MesDoRelatorio(hoje)
	var rel *RelatorioMes
	if err := comUsuario(func(ctx context.Context, tx pgx.Tx) error {
		existe, err := existeRelatorio(ctx, tx, "mes", mes.Format("2006-01"))
		if err != nil || existe {
			return err
		}
		r, err := MontarRelatorioMes(ctx, tx, mes, hoje)
		if err != nil {
			return err
		}
		if r.Totais.Receitas != 0 || r.Totais.Gastos != 0 {
			rel = &r
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if rel != nil {
		var texto *string
		if cliente != nil {
			var pode bool
			_ = comUsuario(func(ctx context.Context, tx pgx.Tx) error {
				pode = PodeUsarIA(ctx, tx, hoje) == nil
				return nil
			})
			if pode {
				// só números e nomes de categoria: nada de descrição de transação
				t, uso, errIA := cliente.EscreverRelatorio(ctx, map[string]any{"mes": rel.Mes, "totais": rel.Totais,
					"mes_anterior": rel.Anterior, "categorias": rel.Categorias, "acoes_ja_sugeridas": rel.Acoes})
				if errIA == nil && t != "" {
					texto = &t
				}
				_ = comUsuario(func(ctx context.Context, tx pgx.Tx) error { return RegistrarUsoIA(ctx, tx, hoje, uso) })
			}
		}
		if err := comUsuario(func(ctx context.Context, tx pgx.Tx) error {
			id, novo, err := salvarRelatorio(ctx, tx, "mes", rel.Mes, rel, texto)
			if novo {
				novos = append(novos, id)
			}
			return err
		}); err != nil {
			return novos, err
		}
	}

	semana := core.SemanaDoResumo(agora)
	err = comUsuario(func(ctx context.Context, tx pgx.Tx) error {
		periodo := semana.Format(formatoData)
		existe, err := existeRelatorio(ctx, tx, "semana", periodo)
		if err != nil || existe {
			return err
		}
		r, err := MontarResumoSemana(ctx, tx, semana, hoje)
		if err != nil || (r.Gasto == 0 && r.Anterior == 0 && len(r.Proximas) == 0) {
			return err
		}
		id, novo, err := salvarRelatorio(ctx, tx, "semana", periodo, r, nil)
		if novo {
			novos = append(novos, id)
		}
		return err
	})
	return novos, err
}

// ListarRelatorios do usuário da sessão (sem os dados).
func ListarRelatorios(ctx context.Context, tx pgx.Tx) ([]RelatorioSalvo, error) {
	linhas, err := tx.Query(ctx, `select id, tipo, periodo, null::jsonb, texto, criado_em from relatorios
		order by criado_em desc limit 60`)
	if err != nil {
		return nil, err
	}
	lista, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[RelatorioSalvo])
	if lista == nil {
		lista = []RelatorioSalvo{}
	}
	return lista, err
}

// LerRelatorio completo.
func LerRelatorio(ctx context.Context, tx pgx.Tx, id string) (RelatorioSalvo, error) {
	linhas, err := tx.Query(ctx, "select id, tipo, periodo, dados, texto, criado_em from relatorios where id = $1", id)
	if err != nil {
		return RelatorioSalvo{}, err
	}
	return pgx.CollectExactlyOneRow(linhas, pgx.RowToStructByPos[RelatorioSalvo])
}
