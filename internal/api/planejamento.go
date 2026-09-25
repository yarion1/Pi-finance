package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

// dataOpcional lê "AAAA-MM-DD" ou vazio.
func dataOpcional(s *string) (*time.Time, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(*s))
	if err != nil {
		return nil, invalido("data inválida")
	}
	return &t, nil
}

func vazioParaNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

// ---------------------------------------------------------------------------
// Cartões
// ---------------------------------------------------------------------------

func (s *Servidor) faturas(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		Faturas  []financeiro.Fatura        `json:"faturas"`
		Parcelas []financeiro.ParcelaFutura `json:"parcelas_futuras"`
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		hoje := financeiro.Hoje()
		var ent string
		if err := tx.QueryRow(ctx, "select entidade_id from contas where id = $1 and tipo = 'cartao'", r.PathValue("id")).Scan(&ent); err != nil {
			return err
		}
		var err error
		if resp.Faturas, err = financeiro.Faturas(ctx, tx, r.PathValue("id"), hoje); err != nil {
			return err
		}
		resp.Parcelas, err = financeiro.ParcelasFuturas(ctx, tx, []string{ent}, r.PathValue("id"), hoje)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if resp.Parcelas == nil {
		resp.Parcelas = []financeiro.ParcelaFutura{}
	}
	escreverJSON(w, http.StatusOK, resp)
}

// indicadores: os números que o painel sempre mostra (SPEC §2).
func (s *Servidor) indicadores(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		Patrimonio     financeiro.Patrimonio        `json:"patrimonio"`
		CustoDeVida    financeiro.CustoDeVida       `json:"custo_de_vida"`
		Comprometido   []financeiro.MesComprometido `json:"comprometido"`
		SaldoProjetado int64                        `json:"saldo_projetado_30_dias_centavos"`
		MenorSaldo30   financeiro.SaldoDia          `json:"menor_saldo_30_dias"`
		Reserva        int64                        `json:"reserva_centavos"`
		MesesDeReserva *float64                     `json:"meses_de_reserva"`
	}
	serie := r.URL.Query().Get("serie") == "1"
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		hoje := financeiro.Hoje()
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		if resp.Patrimonio, err = financeiro.CalcularPatrimonio(ctx, tx, ents, hoje, serie); err != nil {
			return err
		}
		if resp.CustoDeVida, err = financeiro.CalcularCustoDeVida(ctx, tx, ents, hoje); err != nil {
			return err
		}
		if resp.Comprometido, err = financeiro.Comprometido(ctx, tx, ents, hoje, 12); err != nil {
			return err
		}
		ag, err := financeiro.CalcularAgenda(ctx, tx, ents, hoje, 30)
		if err != nil {
			return err
		}
		resp.SaldoProjetado, resp.MenorSaldo30 = ag.SaldoFinal, ag.MenorSaldo
		metas, err := listarMetas(ctx, tx, ents, hoje, resp.CustoDeVida.Meses6)
		if err != nil {
			return err
		}
		for _, m := range metas {
			if m.Tipo == "reserva" {
				resp.Reserva += m.Atual
			}
		}
		if resp.Reserva > 0 && resp.CustoDeVida.Meses6 > 0 {
			v := float64(core.MesesCobertos(core.Centavos(resp.Reserva), core.Centavos(resp.CustoDeVida.Meses6))) / 10
			resp.MesesDeReserva = &v
		}
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

func (s *Servidor) agenda(w http.ResponseWriter, r *http.Request) {
	dias, _ := strconv.Atoi(r.URL.Query().Get("dias"))
	if dias <= 0 || dias > 365 {
		dias = 60
	}
	var ag financeiro.Agenda
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		ag, err = financeiro.CalcularAgenda(ctx, tx, ents, financeiro.Hoje(), dias)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, ag)
}

// ---------------------------------------------------------------------------
// Recorrências
// ---------------------------------------------------------------------------

func (s *Servidor) listarRecorrencias(w http.ResponseWriter, r *http.Request) {
	var lista []financeiro.Recorrencia
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		lista, err = financeiro.Recorrencias(ctx, tx, ents, financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if lista == nil {
		lista = []financeiro.Recorrencia{}
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) detectarRecorrencias(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID string `json:"entidade_id"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	novas := 0
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var pode bool
		if err := tx.QueryRow(ctx, "select app_pode_editar_entidade($1)", c.EntidadeID).Scan(&pode); err != nil {
			return err
		}
		if !pode {
			return errNaoEncontrado
		}
		var err error
		novas, err = financeiro.DetectarRecorrencias(ctx, tx, c.EntidadeID, financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]int{"novas": novas})
}

type dadosRecorrencia struct {
	EntidadeID  string  `json:"entidade_id"`
	Descricao   string  `json:"descricao"`
	Valor       int64   `json:"valor_centavos"`
	Frequencia  string  `json:"frequencia"`
	Proxima     string  `json:"proxima"`
	Tipo        string  `json:"tipo"`
	ContaID     *string `json:"conta_id"`
	CategoriaID *string `json:"categoria_id"`
	Ativa       *bool   `json:"ativa"`
}

func (d *dadosRecorrencia) validar() (time.Time, error) {
	d.Descricao = strings.TrimSpace(d.Descricao)
	if d.Descricao == "" || len([]rune(d.Descricao)) > 100 {
		return time.Time{}, invalido("descrição de 1 a 100 caracteres")
	}
	if d.Valor == 0 {
		return time.Time{}, invalido("valor não pode ser zero")
	}
	if d.Frequencia != "mensal" && d.Frequencia != "anual" && d.Frequencia != "semanal" {
		return time.Time{}, invalido("frequência: mensal, anual ou semanal")
	}
	if d.Tipo != "assinatura" && d.Tipo != "conta_fixa" && d.Tipo != "receita" {
		return time.Time{}, invalido("tipo: assinatura, conta_fixa ou receita")
	}
	proxima, err := time.Parse("2006-01-02", d.Proxima)
	if err != nil {
		return time.Time{}, invalido("próxima data inválida")
	}
	d.ContaID, d.CategoriaID = vazioParaNil(d.ContaID), vazioParaNil(d.CategoriaID)
	return proxima, nil
}

// anterior: a ocorrência "de antes" da próxima, para o cálculo seguir a partir dela.
func anterior(proxima time.Time, frequencia string) time.Time {
	switch frequencia {
	case "anual":
		return core.SomarMeses(proxima, -12)
	case "semanal":
		return proxima.AddDate(0, 0, -7)
	default:
		return core.SomarMeses(proxima, -1)
	}
}

func (s *Servidor) criarRecorrencia(w http.ResponseWriter, r *http.Request) {
	var d dadosRecorrencia
	if !lerJSON(w, r, &d) {
		return
	}
	proxima, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into recorrencias (entidade_id, conta_id, descricao, chave, categoria_id, valor_centavos,
				frequencia, dia, ultima_data, tipo)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) returning id`,
			d.EntidadeID, d.ContaID, d.Descricao, "manual:"+core.ChaveHistorico(d.Descricao)+":"+proxima.Format("0102"),
			d.CategoriaID, d.Valor, d.Frequencia, proxima.Day(), anterior(proxima, d.Frequencia), d.Tipo).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarRecorrencia(w http.ResponseWriter, r *http.Request) {
	var d dadosRecorrencia
	if !lerJSON(w, r, &d) {
		return
	}
	proxima, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	ativa := true
	if d.Ativa != nil {
		ativa = *d.Ativa
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, `update recorrencias set conta_id = $2, descricao = $3, categoria_id = $4,
				valor_centavos = $5, frequencia = $6, dia = $7, ultima_data = $8, tipo = $9, ativa = $10
			where id = $1`, r.PathValue("id"), d.ContaID, d.Descricao, d.CategoriaID, d.Valor, d.Frequencia,
			proxima.Day(), anterior(proxima, d.Frequencia), d.Tipo, ativa))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarRecorrencia(w http.ResponseWriter, r *http.Request) {
	s.apagarDaTabela(w, r, "recorrencias")
}

// apagarDaTabela apaga por id; o RLS garante que só quem edita a entidade consegue.
func (s *Servidor) apagarDaTabela(w http.ResponseWriter, r *http.Request, tabela string) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "delete from "+pgx.Identifier{tabela}.Sanitize()+" where id = $1", r.PathValue("id")))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Metas
// ---------------------------------------------------------------------------

type meta struct {
	ID               string   `json:"id"`
	EntidadeID       string   `json:"entidade_id"`
	Nome             string   `json:"nome"`
	Tipo             string   `json:"tipo"`
	Alvo             int64    `json:"alvo_centavos"`
	DataAlvo         *string  `json:"data_alvo"`
	ContasVinculadas []string `json:"contas_vinculadas"`
	ValorManual      int64    `json:"valor_manual_centavos"`
	AportePlanejado  *int64   `json:"aporte_planejado_centavos"`
	Atual            int64    `json:"atual_centavos"`
	Percentual       float64  `json:"percentual"`
	AporteNecessario int64    `json:"aporte_necessario_centavos"`
	MesesRestantes   int      `json:"meses_restantes"`
	DataPrevista     *string  `json:"data_prevista"`
	MesesDeCusto     *float64 `json:"meses_de_custo_de_vida"`
	Concluida        bool     `json:"concluida"`
}

func listarMetas(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, custoVida int64) ([]meta, error) {
	linhas, err := tx.Query(ctx, `select m.id, m.entidade_id, m.nome, m.tipo::text, m.alvo_centavos, m.data_alvo,
			m.contas_vinculadas::text[], m.valor_manual_centavos, m.aporte_planejado_centavos,
			m.valor_manual_centavos + coalesce((select sum(coalesce(app_saldo_conta(c, $2), 0)) from unnest(m.contas_vinculadas) c), 0)
		from metas m where m.entidade_id = any($1) order by m.concluida_em nulls first, m.data_alvo nulls last, m.nome`, ents, hoje)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var out []meta
	for linhas.Next() {
		var m meta
		var dataAlvo *time.Time
		if err := linhas.Scan(&m.ID, &m.EntidadeID, &m.Nome, &m.Tipo, &m.Alvo, &dataAlvo, &m.ContasVinculadas,
			&m.ValorManual, &m.AportePlanejado, &m.Atual); err != nil {
			return nil, err
		}
		if dataAlvo != nil {
			d := dataAlvo.Format("2006-01-02")
			m.DataAlvo = &d
		}
		m.Percentual = float64(m.Atual) / float64(m.Alvo)
		m.Concluida = m.Atual >= m.Alvo
		aporte, meses := core.AporteNecessario(core.Centavos(m.Alvo), core.Centavos(m.Atual), hoje, dataAlvo)
		m.AporteNecessario, m.MesesRestantes = int64(aporte), meses
		if m.AportePlanejado != nil {
			if d := core.DataPrevista(core.Centavos(m.Alvo), core.Centavos(m.Atual), core.Centavos(*m.AportePlanejado), hoje); d != nil {
				s := d.Format("2006-01-02")
				m.DataPrevista = &s
			}
		}
		if m.Tipo == "reserva" {
			if c := core.MesesCobertos(core.Centavos(m.Atual), core.Centavos(custoVida)); c >= 0 {
				v := float64(c) / 10
				m.MesesDeCusto = &v
			}
		}
		out = append(out, m)
	}
	return out, linhas.Err()
}

func (s *Servidor) listarMetasHTTP(w http.ResponseWriter, r *http.Request) {
	var lista []meta
	var custo financeiro.CustoDeVida
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		hoje := financeiro.Hoje()
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		if custo, err = financeiro.CalcularCustoDeVida(ctx, tx, ents, hoje); err != nil {
			return err
		}
		lista, err = listarMetas(ctx, tx, ents, hoje, custo.Meses6)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if lista == nil {
		lista = []meta{}
	}
	escreverJSON(w, http.StatusOK, map[string]any{"metas": lista, "custo_de_vida": custo})
}

type dadosMeta struct {
	EntidadeID       string   `json:"entidade_id"`
	Nome             string   `json:"nome"`
	Tipo             string   `json:"tipo"`
	Alvo             int64    `json:"alvo_centavos"`
	DataAlvo         *string  `json:"data_alvo"`
	ContasVinculadas []string `json:"contas_vinculadas"`
	ValorManual      int64    `json:"valor_manual_centavos"`
	AportePlanejado  *int64   `json:"aporte_planejado_centavos"`
}

func (d *dadosMeta) validar() (*time.Time, error) {
	nome, err := validarNome(d.Nome)
	if err != nil {
		return nil, err
	}
	d.Nome = nome
	switch d.Tipo {
	case "reserva", "viagem", "compra", "aposentadoria", "outra":
	default:
		return nil, invalido("tipo de meta inválido")
	}
	if d.Alvo <= 0 {
		return nil, invalido("o alvo precisa ser positivo")
	}
	if d.ContasVinculadas == nil {
		d.ContasVinculadas = []string{}
	}
	if d.AportePlanejado != nil && *d.AportePlanejado <= 0 {
		d.AportePlanejado = nil
	}
	return dataOpcional(d.DataAlvo)
}

// contasVisiveis confere que o usuário vê todas as contas vinculadas.
func contasVisiveis(ctx context.Context, tx pgx.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	var n int
	if err := tx.QueryRow(ctx, "select count(*) from contas where id = any($1::uuid[])", ids).Scan(&n); err != nil {
		return err
	}
	if n != len(ids) {
		return invalido("conta vinculada inexistente")
	}
	return nil
}

func (s *Servidor) criarMeta(w http.ResponseWriter, r *http.Request) {
	var d dadosMeta
	if !lerJSON(w, r, &d) {
		return
	}
	data, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := contasVisiveis(ctx, tx, d.ContasVinculadas); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `insert into metas (entidade_id, nome, tipo, alvo_centavos, data_alvo, contas_vinculadas,
				valor_manual_centavos, aporte_planejado_centavos)
			values ($1, $2, $3, $4, $5, $6::uuid[], $7, $8) returning id`,
			d.EntidadeID, d.Nome, d.Tipo, d.Alvo, data, d.ContasVinculadas, d.ValorManual, d.AportePlanejado).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarMeta(w http.ResponseWriter, r *http.Request) {
	var d dadosMeta
	if !lerJSON(w, r, &d) {
		return
	}
	data, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := contasVisiveis(ctx, tx, d.ContasVinculadas); err != nil {
			return err
		}
		return exigirUma(tx.Exec(ctx, `update metas set nome = $2, tipo = $3, alvo_centavos = $4, data_alvo = $5,
				contas_vinculadas = $6::uuid[], valor_manual_centavos = $7, aporte_planejado_centavos = $8
			where id = $1`, r.PathValue("id"), d.Nome, d.Tipo, d.Alvo, data, d.ContasVinculadas, d.ValorManual, d.AportePlanejado))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarMeta(w http.ResponseWriter, r *http.Request) {
	s.apagarDaTabela(w, r, "metas")
}

// ---------------------------------------------------------------------------
// Patrimônio: bens e dívidas
// ---------------------------------------------------------------------------

type bem struct {
	ID           string  `json:"id"`
	EntidadeID   string  `json:"entidade_id"`
	Nome         string  `json:"nome"`
	Tipo         string  `json:"tipo"`
	Valor        int64   `json:"valor_centavos"`
	AtualizadoEm string  `json:"atualizado_em"`
	FipeCodigo   *string `json:"fipe_codigo"`
	Notas        *string `json:"notas"`
}

func (s *Servidor) patrimonio(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		financeiro.Patrimonio
		BensLista []bem               `json:"bens"`
		Dividas   []financeiro.Divida `json:"dividas"`
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		hoje := financeiro.Hoje()
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		if resp.Patrimonio, err = financeiro.CalcularPatrimonio(ctx, tx, ents, hoje, true); err != nil {
			return err
		}
		linhas, err := tx.Query(ctx, `select id, entidade_id, nome, tipo::text, valor_centavos, to_char(atualizado_em, 'YYYY-MM-DD'),
			fipe_codigo, notas from bens where entidade_id = any($1) order by valor_centavos desc`, ents)
		if err != nil {
			return err
		}
		if resp.BensLista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[bem]); err != nil {
			return err
		}
		resp.Dividas, err = financeiro.Dividas(ctx, tx, ents, hoje)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if resp.BensLista == nil {
		resp.BensLista = []bem{}
	}
	if resp.Dividas == nil {
		resp.Dividas = []financeiro.Divida{}
	}
	escreverJSON(w, http.StatusOK, resp)
}

type dadosBem struct {
	EntidadeID string  `json:"entidade_id"`
	Nome       string  `json:"nome"`
	Tipo       string  `json:"tipo"`
	Valor      int64   `json:"valor_centavos"`
	FipeCodigo *string `json:"fipe_codigo"`
	Notas      *string `json:"notas"`
}

func (d *dadosBem) validar() error {
	nome, err := validarNome(d.Nome)
	if err != nil {
		return err
	}
	d.Nome = nome
	if d.Tipo != "imovel" && d.Tipo != "veiculo" && d.Tipo != "outro" {
		return invalido("tipo: imovel, veiculo ou outro")
	}
	if d.Valor < 0 {
		return invalido("valor não pode ser negativo")
	}
	d.FipeCodigo, d.Notas = vazioParaNil(d.FipeCodigo), vazioParaNil(d.Notas)
	return nil
}

func (s *Servidor) criarBem(w http.ResponseWriter, r *http.Request) {
	var d dadosBem
	if !lerJSON(w, r, &d) {
		return
	}
	if err := d.validar(); err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into bens (entidade_id, nome, tipo, valor_centavos, fipe_codigo, notas)
			values ($1, $2, $3, $4, $5, $6) returning id`, d.EntidadeID, d.Nome, d.Tipo, d.Valor, d.FipeCodigo, d.Notas).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarBem(w http.ResponseWriter, r *http.Request) {
	var d dadosBem
	if !lerJSON(w, r, &d) {
		return
	}
	if err := d.validar(); err != nil {
		falhar(w, r, err)
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, `update bens set nome = $2, tipo = $3, valor_centavos = $4, fipe_codigo = $5, notas = $6,
			atualizado_em = current_date where id = $1`, r.PathValue("id"), d.Nome, d.Tipo, d.Valor, d.FipeCodigo, d.Notas))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarBem(w http.ResponseWriter, r *http.Request) { s.apagarDaTabela(w, r, "bens") }

type dadosDivida struct {
	EntidadeID         string  `json:"entidade_id"`
	Nome               string  `json:"nome"`
	Tipo               string  `json:"tipo"`
	Sistema            string  `json:"sistema"`
	Principal          int64   `json:"principal_centavos"`
	TaxaMensal         string  `json:"taxa_mensal"`
	Prazo              int     `json:"prazo_meses"`
	PrimeiroVencimento string  `json:"primeiro_vencimento"`
	ContaID            *string `json:"conta_id"`
	BemID              *string `json:"bem_id"`
}

func (d *dadosDivida) validar() (time.Time, error) {
	nome, err := validarNome(d.Nome)
	if err != nil {
		return time.Time{}, err
	}
	d.Nome = nome
	if d.Tipo != "financiamento" && d.Tipo != "emprestimo" && d.Tipo != "outro" {
		return time.Time{}, invalido("tipo: financiamento, emprestimo ou outro")
	}
	taxa, err := core.LerTaxa(d.TaxaMensal)
	if err != nil {
		return time.Time{}, invalido("taxa ao mês como fração (0,99 % = 0.0099)")
	}
	primeiro, err := time.Parse("2006-01-02", d.PrimeiroVencimento)
	if err != nil {
		return time.Time{}, invalido("data da primeira parcela inválida")
	}
	if _, err := core.TabelaAmortizacao(d.Sistema, core.Centavos(d.Principal), taxa, d.Prazo, primeiro); err != nil {
		return time.Time{}, invalido(err.Error())
	}
	d.TaxaMensal = taxa.Text('f', 8)
	d.ContaID, d.BemID = vazioParaNil(d.ContaID), vazioParaNil(d.BemID)
	return primeiro, nil
}

func (s *Servidor) criarDivida(w http.ResponseWriter, r *http.Request) {
	var d dadosDivida
	if !lerJSON(w, r, &d) {
		return
	}
	primeiro, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into dividas (entidade_id, nome, tipo, sistema, principal_centavos, taxa_mensal, prazo_meses,
				primeiro_vencimento, conta_id, bem_id)
			values ($1, $2, $3, $4, $5, $6::numeric, $7, $8, $9, $10) returning id`,
			d.EntidadeID, d.Nome, d.Tipo, d.Sistema, d.Principal, d.TaxaMensal, d.Prazo, primeiro, d.ContaID, d.BemID).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarDivida(w http.ResponseWriter, r *http.Request) {
	var d dadosDivida
	if !lerJSON(w, r, &d) {
		return
	}
	primeiro, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, `update dividas set nome = $2, tipo = $3, sistema = $4, principal_centavos = $5,
				taxa_mensal = $6::numeric, prazo_meses = $7, primeiro_vencimento = $8, conta_id = $9, bem_id = $10
			where id = $1`, r.PathValue("id"), d.Nome, d.Tipo, d.Sistema, d.Principal, d.TaxaMensal, d.Prazo, primeiro, d.ContaID, d.BemID))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarDivida(w http.ResponseWriter, r *http.Request) {
	s.apagarDaTabela(w, r, "dividas")
}

func (s *Servidor) tabelaDivida(w http.ResponseWriter, r *http.Request) {
	var tabela []core.ParcelaDivida
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var ent string
		if err := tx.QueryRow(ctx, "select entidade_id from dividas where id = $1", r.PathValue("id")).Scan(&ent); err != nil {
			return err
		}
		dividas, err := financeiro.Dividas(ctx, tx, []string{ent}, financeiro.Hoje())
		if err != nil {
			return err
		}
		for _, d := range dividas {
			if d.ID == r.PathValue("id") {
				tabela = d.Tabela()
			}
		}
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, tabela)
}

// ---------------------------------------------------------------------------
// Compromissos (contas a pagar e a receber avulsas)
// ---------------------------------------------------------------------------

type dadosCompromisso struct {
	EntidadeID  string  `json:"entidade_id"`
	Descricao   string  `json:"descricao"`
	Valor       int64   `json:"valor_centavos"`
	Vencimento  string  `json:"vencimento"`
	ContaID     *string `json:"conta_id"`
	CategoriaID *string `json:"categoria_id"`
	PagoEm      *string `json:"pago_em"`
}

func (d *dadosCompromisso) validar() (time.Time, *time.Time, error) {
	d.Descricao = strings.TrimSpace(d.Descricao)
	if d.Descricao == "" || len([]rune(d.Descricao)) > 200 {
		return time.Time{}, nil, invalido("descrição de 1 a 200 caracteres")
	}
	if d.Valor == 0 {
		return time.Time{}, nil, invalido("valor não pode ser zero")
	}
	venc, err := time.Parse("2006-01-02", d.Vencimento)
	if err != nil {
		return time.Time{}, nil, invalido("vencimento inválido")
	}
	pago, err := dataOpcional(d.PagoEm)
	d.ContaID, d.CategoriaID = vazioParaNil(d.ContaID), vazioParaNil(d.CategoriaID)
	return venc, pago, err
}

func (s *Servidor) criarCompromisso(w http.ResponseWriter, r *http.Request) {
	var d dadosCompromisso
	if !lerJSON(w, r, &d) {
		return
	}
	venc, pago, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into compromissos (entidade_id, conta_id, descricao, valor_centavos, vencimento, categoria_id, pago_em)
			values ($1, $2, $3, $4, $5, $6, $7) returning id`,
			d.EntidadeID, d.ContaID, d.Descricao, d.Valor, venc, d.CategoriaID, pago).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarCompromisso(w http.ResponseWriter, r *http.Request) {
	var d dadosCompromisso
	if !lerJSON(w, r, &d) {
		return
	}
	venc, pago, err := d.validar()
	if err != nil {
		falhar(w, r, err)
		return
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, `update compromissos set conta_id = $2, descricao = $3, valor_centavos = $4, vencimento = $5,
			categoria_id = $6, pago_em = $7 where id = $1`, r.PathValue("id"), d.ContaID, d.Descricao, d.Valor, venc, d.CategoriaID, pago))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarCompromisso(w http.ResponseWriter, r *http.Request) {
	s.apagarDaTabela(w, r, "compromissos")
}
