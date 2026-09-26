package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

type transacao struct {
	ID            string  `json:"id"`
	EntidadeID    string  `json:"entidade_id"`
	ContaID       string  `json:"conta_id"`
	Conta         string  `json:"conta"`
	Data          string  `json:"data"`
	Descricao     string  `json:"descricao"`
	Original      string  `json:"descricao_original"`
	ValorCentavos int64   `json:"valor_centavos"`
	Moeda         string  `json:"moeda"`
	Tipo          string  `json:"tipo"`
	CategoriaID   *string `json:"categoria_id"`
	Categoria     *string `json:"categoria"`
	CategoriaPai  *string `json:"categoria_pai"`
	Cor           *string `json:"cor"`
	ParID         *string `json:"transferencia_par_id"`
	Origem        string  `json:"origem"`
	Notas         *string `json:"notas"`
	ImportacaoID  *string `json:"importacao_id"`
}

const consultaTransacoes = `select t.id, t.entidade_id, t.conta_id, c.nome, to_char(t.data, 'YYYY-MM-DD'), t.descricao,
	t.descricao_original, t.valor_centavos, t.moeda, t.tipo::text, t.categoria_id, cat.nome, pai.nome,
	coalesce(cat.cor, pai.cor), t.transferencia_par_id, t.origem::text, t.notas, t.importacao_id
	from transacoes t
	join contas c on c.id = t.conta_id
	left join categorias cat on cat.id = t.categoria_id
	left join categorias pai on pai.id = cat.pai_id`

// filtroTransacoes monta o where a partir da query string.
func filtroTransacoes(q map[string][]string) (string, []any, error) {
	valor := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	var conds []string
	var args []any
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if v := valor("conta_id"); v != "" {
		add("t.conta_id = $%d", v)
	}
	if v := valor("entidade_id"); v != "" {
		add("t.entidade_id = $%d", v)
	}
	if v := valor("categoria_id"); v != "" {
		add("(t.categoria_id = $%[1]d or cat.pai_id = $%[1]d)", v)
	}
	if v := valor("importacao_id"); v != "" {
		add("t.importacao_id = $%d", v)
	}
	if valor("sem_categoria") == "1" {
		conds = append(conds, "t.categoria_id is null and t.tipo <> 'transferencia'")
	}
	if v := valor("tipo"); v != "" {
		add("t.tipo::text = $%d", v)
	}
	// fluxo de caixa: só o que entrou ou só o que saiu (transferências incluídas)
	switch valor("sentido") {
	case "entradas":
		conds = append(conds, "t.valor_centavos > 0")
	case "saidas":
		conds = append(conds, "t.valor_centavos < 0")
	case "":
	default:
		return "", nil, invalido("sentido: entradas ou saidas")
	}
	for _, k := range []string{"de", "ate"} {
		if v := valor(k); v != "" {
			if _, err := time.Parse("2006-01-02", v); err != nil {
				return "", nil, invalido("data inválida em " + k)
			}
			op := ">="
			if k == "ate" {
				op = "<="
			}
			add("t.data "+op+" $%d::date", v)
		}
	}
	if v := valor("busca"); v != "" {
		add("(t.descricao ilike '%%' || $%[1]d || '%%' or t.descricao_original ilike '%%' || $%[1]d || '%%' or t.notas ilike '%%' || $%[1]d || '%%')", v)
	}
	if len(conds) == 0 {
		return "", args, nil
	}
	return " where " + strings.Join(conds, " and "), args, nil
}

func (s *Servidor) listarTransacoes(w http.ResponseWriter, r *http.Request) {
	where, args, err := filtroTransacoes(r.URL.Query())
	if err != nil {
		falhar(w, r, err)
		return
	}
	limite, _ := strconv.Atoi(r.URL.Query().Get("limite"))
	if limite <= 0 || limite > 500 {
		limite = 100
	}
	pular, _ := strconv.Atoi(r.URL.Query().Get("pular"))
	if pular < 0 {
		pular = 0
	}
	var resp struct {
		Itens []transacao `json:"itens"`
		Total int         `json:"total"`
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `select count(*) from transacoes t
			left join categorias cat on cat.id = t.categoria_id`+where, args...).Scan(&resp.Total); err != nil {
			return err
		}
		linhas, err := tx.Query(ctx, consultaTransacoes+where+
			fmt.Sprintf(" order by t.data desc, t.criada_em desc, t.id limit %d offset %d", limite, pular), args...)
		if err != nil {
			return err
		}
		resp.Itens, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[transacao])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if resp.Itens == nil {
		resp.Itens = []transacao{}
	}
	escreverJSON(w, http.StatusOK, resp)
}

func (s *Servidor) criarTransacao(w http.ResponseWriter, r *http.Request) {
	var c struct {
		ContaID     string  `json:"conta_id"`
		Data        string  `json:"data"`
		Descricao   string  `json:"descricao"`
		Valor       int64   `json:"valor_centavos"`
		CategoriaID *string `json:"categoria_id"`
		Notas       *string `json:"notas"`
		Parcelas    int     `json:"parcelas"` // 2 a 72: o valor é o total da compra
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if c.Parcelas == 1 {
		c.Parcelas = 0
	}
	if c.Parcelas != 0 && (c.Parcelas < 2 || c.Parcelas > 72) {
		falhar(w, r, invalido("parcelas de 2 a 72"))
		return
	}
	data, err := time.Parse("2006-01-02", c.Data)
	desc := strings.TrimSpace(c.Descricao)
	switch {
	case err != nil:
		falhar(w, r, invalido("data inválida"))
		return
	case desc == "" || len([]rune(desc)) > 200:
		falhar(w, r, invalido("descrição de 1 a 200 caracteres"))
		return
	case c.Valor == 0:
		falhar(w, r, invalido("valor não pode ser zero"))
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		conta, err := financeiro.InfoContaEditavel(ctx, tx, c.ContaID)
		if err != nil {
			return err
		}
		categoria := c.CategoriaID
		if categoria != nil && *categoria == "" {
			categoria = nil
		}
		if categoria == nil {
			cat, err := financeiro.NovoCategorizador(ctx, tx, conta.EntidadeID)
			if err != nil {
				return err
			}
			categoria, _ = cat.Sugerir(c.ContaID, desc, core.Centavos(c.Valor))
		}
		tipo, err := tipoParaCategoria(ctx, tx, categoria, c.Valor)
		if err != nil {
			return err
		}
		if c.Parcelas == 0 {
			if err := tx.QueryRow(ctx, `insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao,
					valor_centavos, moeda, categoria_id, tipo, origem, notas, fatura_em)
				values ($1, $2, $3, $4, $4, $5, $6, $7, $8, 'manual', $9, $10) returning id`,
				conta.EntidadeID, c.ContaID, data, desc, c.Valor, conta.Moeda, categoria, tipo, c.Notas, conta.FaturaEm(data)).Scan(&id); err != nil {
				return err
			}
			if tipo != "transferencia" {
				_, err = financeiro.ParearTransferencias(ctx, tx, []string{id})
			}
			return err
		}
		// compra parcelada: uma transação por parcela, um mês depois da outra; no cartão,
		// cada parcela cai na fatura do seu mês
		var compra string
		if err := tx.QueryRow(ctx, "select gen_random_uuid()").Scan(&compra); err != nil {
			return err
		}
		for i, valor := range core.DividirParcelas(core.Centavos(c.Valor), c.Parcelas) {
			dataParcela := core.SomarMeses(data, i)
			var pid string
			if err := tx.QueryRow(ctx, `insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao,
					valor_centavos, moeda, categoria_id, tipo, origem, notas, fatura_em, parcela_n, parcela_total, compra_id)
				values ($1, $2, $3, $4, $4, $5, $6, $7, $8, 'manual', $9, $10, $11, $12, $13) returning id`,
				conta.EntidadeID, c.ContaID, dataParcela, fmt.Sprintf("%s (%d/%d)", desc, i+1, c.Parcelas), int64(valor),
				conta.Moeda, categoria, tipo, c.Notas, conta.FaturaEm(dataParcela), i+1, c.Parcelas, compra).Scan(&pid); err != nil {
				return err
			}
			if i == 0 {
				id = pid
			}
		}
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// tipoParaCategoria: categoria de transferência marca transferência; de gasto num
// valor positivo é estorno (abate o gasto); de receita num valor negativo não vale.
func tipoParaCategoria(ctx context.Context, tx pgx.Tx, categoria *string, valor int64) (string, error) {
	porValor := financeiro.TipoPorValor(core.Centavos(valor))
	if categoria == nil {
		return porValor, nil
	}
	var tipo string
	err := tx.QueryRow(ctx, "select tipo::text from categorias where id = $1", *categoria).Scan(&tipo)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", invalido("categoria inexistente")
	}
	if err != nil {
		return "", err
	}
	switch {
	case tipo == "transferencia":
		return "transferencia", nil
	case tipo == "gasto" && valor > 0:
		return "estorno", nil
	case tipo == "receita" && valor < 0:
		return "", invalido("categoria de receita numa saída de dinheiro")
	default:
		return porValor, nil
	}
}

// recategorizar aplica a categoria (ou nenhuma) a uma transação editável.
func recategorizar(ctx context.Context, tx pgx.Tx, id string, categoria *string) error {
	var valor int64
	var tipoAtual string
	var par *string
	var pode bool
	err := tx.QueryRow(ctx, `select valor_centavos, tipo::text, transferencia_par_id, app_pode_editar_entidade(entidade_id)
		from transacoes where id = $1`, id).Scan(&valor, &tipoAtual, &par, &pode)
	if err != nil {
		return err
	}
	if !pode {
		return errNaoEncontrado
	}
	tipo, err := tipoParaCategoria(ctx, tx, categoria, valor)
	if err != nil {
		return err
	}
	// sair de transferência desfaz o par (a outra ponta volta a ser gasto/receita)
	if tipoAtual == "transferencia" && tipo != "transferencia" && par != nil {
		if err := financeiro.DesfazerPar(ctx, tx, id); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, "update transacoes set categoria_id = $2, tipo = $3, categorizada_por_ia = false where id = $1", id, categoria, tipo)
	return err
}

func (s *Servidor) editarTransacao(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Descricao        *string `json:"descricao"`
		Notas            *string `json:"notas"`
		CategoriaID      *string `json:"categoria_id"`
		AlterarCategoria bool    `json:"alterar_categoria"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	id := r.PathValue("id")
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if c.Descricao != nil {
			d := strings.TrimSpace(*c.Descricao)
			if d == "" || len([]rune(d)) > 200 {
				return invalido("descrição de 1 a 200 caracteres")
			}
			if err := exigirUma(tx.Exec(ctx, "update transacoes set descricao = $2 where id = $1", id, d)); err != nil {
				return err
			}
		}
		if c.Notas != nil {
			if err := exigirUma(tx.Exec(ctx, "update transacoes set notas = nullif($2, '') where id = $1", id, *c.Notas)); err != nil {
				return err
			}
		}
		if c.AlterarCategoria {
			cat := c.CategoriaID
			if cat != nil && *cat == "" {
				cat = nil
			}
			return recategorizar(ctx, tx, id, cat)
		}
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// categorizarLote é a edição em massa; opcionalmente cria a regra "contém X".
func (s *Servidor) categorizarLote(w http.ResponseWriter, r *http.Request) {
	var c struct {
		IDs         []string `json:"ids"`
		CategoriaID *string  `json:"categoria_id"`
		CriarRegra  string   `json:"criar_regra"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if len(c.IDs) == 0 || len(c.IDs) > 1000 {
		falhar(w, r, invalido("selecione de 1 a 1000 transações"))
		return
	}
	cat := c.CategoriaID
	if cat != nil && *cat == "" {
		cat = nil
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		for _, id := range c.IDs {
			if err := recategorizar(ctx, tx, id, cat); err != nil {
				return err
			}
		}
		if texto := strings.TrimSpace(c.CriarRegra); texto != "" && cat != nil {
			_, err := tx.Exec(ctx, `insert into regras_categoria (entidade_id, texto, categoria_id)
				select entidade_id, $2, $3 from transacoes where id = $1`, c.IDs[0], texto, *cat)
			return err
		}
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarTransacao(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var pode bool
		if err := tx.QueryRow(ctx, "select app_pode_editar_entidade(entidade_id) from transacoes where id = $1", id).Scan(&pode); err != nil {
			return err
		}
		if !pode {
			return errNaoEncontrado
		}
		if err := financeiro.DesfazerPar(ctx, tx, id); err != nil {
			return err
		}
		return exigirUma(tx.Exec(ctx, "delete from transacoes where id = $1", id))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
