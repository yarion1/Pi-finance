package financeiro

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// Casa (fase 7): o consolidado do que cada um compartilha, as despesas divididas com o
// "quem deve quem" e as metas conjuntas. O RLS decide o que cada pessoa vê.

// MembroPainel: quanto vem de cada pessoa.
type MembroPainel struct {
	UsuarioID    string  `json:"usuario_id"`
	Nome         string  `json:"nome"`
	Saldo        int64   `json:"saldo_centavos"`        // contas que ela compartilha (inteiras ou só saldo)
	Gastos       int64   `json:"gastos_centavos"`       // gastos do mês nas contas compartilhadas dela
	Contribuicao int64   `json:"contribuicao_centavos"` // gastos + despesas divididas que ela pagou fora delas
	Percentual   float64 `json:"percentual"`            // da contribuição total
}

// ContaCasa compartilhada com a casa.
type ContaCasa struct {
	ID           string `json:"id"`
	Nome         string `json:"nome"`
	Tipo         string `json:"tipo"`
	Visibilidade string `json:"visibilidade"`
	DonoID       string `json:"dono_id"`
	Dono         string `json:"dono"`
	Saldo        int64  `json:"saldo_centavos"`
}

// ParteDespesa de uma pessoa numa despesa dividida.
type ParteDespesa struct {
	UsuarioID string `json:"usuario_id"`
	Nome      string `json:"nome"`
	Valor     int64  `json:"valor_centavos"`
	Acertada  bool   `json:"acertada"`
}

// DespesaDividida em que a pessoa da sessão está (pagou ou deve).
type DespesaDividida struct {
	ID        string         `json:"id"`
	Descricao string         `json:"descricao"`
	Data      string         `json:"data"`
	Total     int64          `json:"total_centavos"`
	Modo      string         `json:"modo"`
	PagoPor   string         `json:"pago_por"`
	PagoNome  string         `json:"pago_por_nome"`
	Partes    []ParteDespesa `json:"partes"`
}

// SaldoPessoa: positivo, a pessoa me deve; negativo, eu devo a ela.
type SaldoPessoa struct {
	UsuarioID string `json:"usuario_id"`
	Nome      string `json:"nome"`
	Valor     int64  `json:"valor_centavos"`
}

// ContribuicaoMeta de um membro.
type ContribuicaoMeta struct {
	UsuarioID string `json:"usuario_id"`
	Nome      string `json:"nome"`
	Total     int64  `json:"total_centavos"`
}

// MetaCasa conjunta.
type MetaCasa struct {
	ID               string             `json:"id"`
	Nome             string             `json:"nome"`
	Tipo             string             `json:"tipo"`
	Alvo             int64              `json:"alvo_centavos"`
	DataAlvo         *string            `json:"data_alvo"`
	Atual            int64              `json:"atual_centavos"`
	Percentual       float64            `json:"percentual"`
	AporteNecessario int64              `json:"aporte_necessario_centavos"`
	MesesRestantes   int                `json:"meses_restantes"`
	Contribuicoes    []ContribuicaoMeta `json:"contribuicoes"`
	PodeApagar       bool               `json:"pode_apagar"`
}

// PainelCasa: o consolidado do mês.
type PainelCasa struct {
	Mes        string            `json:"mes"`
	SaldoTotal int64             `json:"saldo_total_centavos"`
	Gastos     int64             `json:"gastos_centavos"`
	Membros    []MembroPainel    `json:"membros"`
	Contas     []ContaCasa       `json:"contas"`
	Categorias []GastoCategoria  `json:"categorias"`
	Saldos     []SaldoPessoa     `json:"quem_deve_quem"`
	Despesas   []DespesaDividida `json:"despesas"`
	Metas      []MetaCasa        `json:"metas"`
	PodeEditar bool              `json:"pode_editar"`
}

// MontarPainelCasa do mês (primeiro dia) para a pessoa da sessão.
func MontarPainelCasa(ctx context.Context, tx pgx.Tx, casaID string, mes, hoje time.Time) (PainelCasa, error) {
	p := PainelCasa{Mes: mes.Format("2006-01")}
	var papel string
	if err := tx.QueryRow(ctx, "select app_papel_casa($1)::text", casaID).Scan(&papel); err != nil || papel == "" {
		return p, pgx.ErrNoRows
	}
	p.PodeEditar = papel == "dono" || papel == "membro"
	de, ate := mes, fimDoMes(mes)

	// membros
	linhas, err := tx.Query(ctx, `select u.id, u.nome from membros_casa m join usuarios u on u.id = m.usuario_id
		where m.casa_id = $1 order by m.entrou_em`, casaID)
	if err != nil {
		return p, err
	}
	nomes := map[string]string{}
	idx := map[string]int{}
	for linhas.Next() {
		var m MembroPainel
		if err := linhas.Scan(&m.UsuarioID, &m.Nome); err != nil {
			linhas.Close()
			return p, err
		}
		idx[m.UsuarioID] = len(p.Membros)
		nomes[m.UsuarioID] = m.Nome
		p.Membros = append(p.Membros, m)
	}
	linhas.Close()

	// contas que a casa vê (o RLS já deixa só as não privadas dos outros)
	linhas, err = tx.Query(ctx, `select c.id, c.nome, c.tipo::text, c.visibilidade::text, coalesce(app_dono_conta_id(c.id)::text, ''),
			coalesce(app_dono_conta(c.id), ''), coalesce(app_saldo_conta(c.id, $2), 0)
		from contas c where c.casa_id = $1 and c.visibilidade <> 'privada' and not c.arquivada and c.moeda = 'BRL'
		order by c.nome`, casaID, hoje)
	if err != nil {
		return p, err
	}
	if p.Contas, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[ContaCasa]); err != nil {
		return p, err
	}
	for _, c := range p.Contas {
		p.SaldoTotal += c.Saldo
		if i, ok := idx[c.DonoID]; ok {
			p.Membros[i].Saldo += c.Saldo
		}
	}

	// gastos do mês nas contas compartilhadas por inteiro
	filtro := `from transacoes t join contas c on c.id = t.conta_id
		left join categorias cat on cat.id = t.categoria_id
		left join categorias pai on pai.id = cat.pai_id
		where c.casa_id = $1 and c.visibilidade = 'compartilhada' and t.tipo in ('gasto', 'estorno')
		  and t.moeda = 'BRL' and t.data between $2 and $3`
	linhas, err = tx.Query(ctx, `select app_dono_conta_id(c.id), -sum(t.valor_centavos) `+filtro+` group by 1`, casaID, de, ate)
	if err != nil {
		return p, err
	}
	for linhas.Next() {
		var u *string
		var v int64
		if err := linhas.Scan(&u, &v); err != nil {
			linhas.Close()
			return p, err
		}
		p.Gastos += v
		if u != nil {
			if i, ok := idx[*u]; ok {
				p.Membros[i].Gastos += v
				p.Membros[i].Contribuicao += v
			}
		}
	}
	linhas.Close()
	linhas, err = tx.Query(ctx, `select coalesce(pai.id, cat.id), coalesce(pai.nome, cat.nome, 'Sem categoria'),
			coalesce(pai.cor, cat.cor), -sum(t.valor_centavos) as total `+filtro+`
		group by 1, 2, 3 having -sum(t.valor_centavos) > 0 order by total desc`, casaID, de, ate)
	if err != nil {
		return p, err
	}
	if p.Categorias, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[GastoCategoria]); err != nil {
		return p, err
	}

	// despesas divididas pagas fora das contas compartilhadas também contam
	linhas, err = tx.Query(ctx, "select usuario_id, total_centavos from app_divididas_da_casa($1, $2, $3)", casaID, de, ate)
	if err != nil {
		return p, err
	}
	for linhas.Next() {
		var u string
		var v int64
		if err := linhas.Scan(&u, &v); err != nil {
			linhas.Close()
			return p, err
		}
		if i, ok := idx[u]; ok {
			p.Membros[i].Contribuicao += v
		}
	}
	linhas.Close()
	var total int64
	for _, m := range p.Membros {
		total += m.Contribuicao
	}
	for i := range p.Membros {
		if total > 0 {
			p.Membros[i].Percentual = float64(p.Membros[i].Contribuicao) / float64(total)
		}
	}

	if p.Despesas, p.Saldos, err = despesasDaCasa(ctx, tx, casaID, nomes); err != nil {
		return p, err
	}
	p.Metas, err = metasDaCasa(ctx, tx, casaID, hoje, nomes)
	if p.Categorias == nil {
		p.Categorias = []GastoCategoria{}
	}
	return p, err
}

// despesasDaCasa em que a pessoa da sessão está (as 50 mais recentes) e o saldo com cada um.
func despesasDaCasa(ctx context.Context, tx pgx.Tx, casaID string, nomes map[string]string) ([]DespesaDividida, []SaldoPessoa, error) {
	var eu string
	if err := tx.QueryRow(ctx, "select app_usuario_id()").Scan(&eu); err != nil {
		return nil, nil, err
	}
	// as dívidas em aberto contam todas; a lista mostra as recentes
	linhas, err := tx.Query(ctx, `select d.pago_por, v.usuario_id, v.valor_centavos
		from divisoes v join despesas_divididas d on d.id = v.despesa_id
		where d.casa_id = $1 and v.acertado_em is null`, casaID)
	if err != nil {
		return nil, nil, err
	}
	var dividas []core.Divida
	for linhas.Next() {
		var d core.Divida
		var v int64
		if err := linhas.Scan(&d.Credor, &d.Devedor, &v); err != nil {
			linhas.Close()
			return nil, nil, err
		}
		d.Valor = core.Centavos(v)
		dividas = append(dividas, d)
	}
	linhas.Close()
	saldos := []SaldoPessoa{}
	for _, s := range core.QuemDeveQuem(eu, dividas) {
		saldos = append(saldos, SaldoPessoa{UsuarioID: s.UsuarioID, Nome: nomeOu(nomes, s.UsuarioID), Valor: int64(s.Valor)})
	}

	linhas, err = tx.Query(ctx, `select id, descricao, data, total_centavos, modo, pago_por from despesas_divididas
		where casa_id = $1 order by data desc, criada_em desc limit 50`, casaID)
	if err != nil {
		return nil, nil, err
	}
	despesas := []DespesaDividida{}
	pos := map[string]int{}
	for linhas.Next() {
		var d DespesaDividida
		var data time.Time
		if err := linhas.Scan(&d.ID, &d.Descricao, &data, &d.Total, &d.Modo, &d.PagoPor); err != nil {
			linhas.Close()
			return nil, nil, err
		}
		d.Data = data.Format("2006-01-02")
		d.PagoNome = nomeOu(nomes, d.PagoPor)
		d.Partes = []ParteDespesa{}
		pos[d.ID] = len(despesas)
		despesas = append(despesas, d)
	}
	linhas.Close()
	if len(despesas) == 0 {
		return despesas, saldos, nil
	}
	ids := make([]string, len(despesas))
	for i, d := range despesas {
		ids[i] = d.ID
	}
	linhas, err = tx.Query(ctx, `select despesa_id, usuario_id, valor_centavos, acertado_em is not null
		from divisoes where despesa_id = any($1)`, ids)
	if err != nil {
		return nil, nil, err
	}
	for linhas.Next() {
		var id string
		var pt ParteDespesa
		if err := linhas.Scan(&id, &pt.UsuarioID, &pt.Valor, &pt.Acertada); err != nil {
			linhas.Close()
			return nil, nil, err
		}
		pt.Nome = nomeOu(nomes, pt.UsuarioID)
		d := &despesas[pos[id]]
		d.Partes = append(d.Partes, pt)
	}
	linhas.Close()
	for i := range despesas {
		sort.SliceStable(despesas[i].Partes, func(a, b int) bool { return despesas[i].Partes[a].Nome < despesas[i].Partes[b].Nome })
	}
	return despesas, saldos, linhas.Err()
}

func nomeOu(nomes map[string]string, id string) string {
	if n, ok := nomes[id]; ok {
		return n
	}
	return "ex-membro"
}

func metasDaCasa(ctx context.Context, tx pgx.Tx, casaID string, hoje time.Time, nomes map[string]string) ([]MetaCasa, error) {
	linhas, err := tx.Query(ctx, `select m.id, m.nome, m.tipo::text, m.alvo_centavos, m.data_alvo,
			m.valor_manual_centavos + coalesce((select sum(valor_centavos) from contribuicoes_meta c where c.meta_id = m.id), 0),
			app_pode_editar_entidade(m.entidade_id)
		from metas m where m.casa_id = $1 order by m.data_alvo nulls last, m.nome`, casaID)
	if err != nil {
		return nil, err
	}
	metas := []MetaCasa{}
	pos := map[string]int{}
	for linhas.Next() {
		var m MetaCasa
		var dataAlvo *time.Time
		if err := linhas.Scan(&m.ID, &m.Nome, &m.Tipo, &m.Alvo, &dataAlvo, &m.Atual, &m.PodeApagar); err != nil {
			linhas.Close()
			return nil, err
		}
		if dataAlvo != nil {
			s := dataAlvo.Format("2006-01-02")
			m.DataAlvo = &s
		}
		m.Percentual = float64(m.Atual) / float64(m.Alvo)
		aporte, meses := core.AporteNecessario(core.Centavos(m.Alvo), core.Centavos(m.Atual), hoje, dataAlvo)
		m.AporteNecessario, m.MesesRestantes = int64(aporte), meses
		m.Contribuicoes = []ContribuicaoMeta{}
		pos[m.ID] = len(metas)
		metas = append(metas, m)
	}
	linhas.Close()
	linhas, err = tx.Query(ctx, `select c.meta_id, c.usuario_id, sum(c.valor_centavos)::bigint from contribuicoes_meta c
		join metas m on m.id = c.meta_id where m.casa_id = $1 group by 1, 2 order by 3 desc`, casaID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	for linhas.Next() {
		var meta string
		var c ContribuicaoMeta
		if err := linhas.Scan(&meta, &c.UsuarioID, &c.Total); err != nil {
			return nil, err
		}
		c.Nome = nomeOu(nomes, c.UsuarioID)
		if i, ok := pos[meta]; ok {
			metas[i].Contribuicoes = append(metas[i].Contribuicoes, c)
		}
	}
	return metas, linhas.Err()
}

// PedidoDivisao: como dividir uma transação (gasto) da pessoa da sessão.
type PedidoDivisao struct {
	TransacaoID string `json:"transacao_id"`
	Modo        string `json:"modo"` // igual, valor, percentual
	Partes      []struct {
		UsuarioID  string `json:"usuario_id"`
		Valor      int64  `json:"valor_centavos"`
		Percentual int64  `json:"percentual_centesimos"` // 5000 = 50 %
	} `json:"partes"`
}

// ErrSemPermissaoCasa: leitor (ou quem não é da casa) tentando escrever.
var ErrSemPermissaoCasa = errors.New("sem permissão nesta casa: só donos e membros")

// ErrJaDividida: a transação já foi dividida.
var ErrJaDividida = ErrCampo{"essa compra já foi dividida; apague a divisão para refazer"}

// DividirDespesa cria a despesa dividida; devolve o id.
func DividirDespesa(ctx context.Context, tx pgx.Tx, casaID string, p PedidoDivisao) (string, error) {
	var eu, descricao, moeda, tipo string
	var data time.Time
	var valor int64
	err := tx.QueryRow(ctx, `select app_usuario_id(), t.descricao, t.data, t.valor_centavos, t.moeda, t.tipo::text
		from transacoes t where t.id = $1 and app_pode_editar_entidade(t.entidade_id)`, p.TransacaoID).
		Scan(&eu, &descricao, &data, &valor, &moeda, &tipo)
	if err != nil {
		return "", err
	}
	if tipo != "gasto" || valor >= 0 {
		return "", ErrCampo{"só dá para dividir um gasto"}
	}
	total := core.Centavos(-valor)
	var partes []core.Parte
	switch p.Modo {
	case "igual":
		ids := make([]string, len(p.Partes))
		for i, pt := range p.Partes {
			ids[i] = pt.UsuarioID
		}
		partes, err = core.DividirIgual(total, ids)
	case "percentual":
		ids := make([]string, len(p.Partes))
		basis := make([]int64, len(p.Partes))
		for i, pt := range p.Partes {
			ids[i], basis[i] = pt.UsuarioID, pt.Percentual
		}
		partes, err = core.DividirPorPercentual(total, ids, basis)
	case "valor":
		ps := make([]core.Parte, len(p.Partes))
		for i, pt := range p.Partes {
			ps[i] = core.Parte{UsuarioID: pt.UsuarioID, Valor: core.Centavos(pt.Valor)}
		}
		partes, err = core.DividirPorValor(total, ps)
	default:
		return "", ErrCampo{"modo: igual, valor ou percentual"}
	}
	if err != nil {
		return "", ErrCampo{err.Error()}
	}
	ids := make([]string, len(partes))
	for i, pt := range partes {
		ids[i] = pt.UsuarioID
	}
	var podem int
	if err := tx.QueryRow(ctx, `select count(*) from membros_casa where casa_id = $1 and usuario_id = any($2::uuid[])
		and papel in ('dono', 'membro')`, casaID, ids).Scan(&podem); err != nil {
		return "", err
	}
	var escrevo bool
	if err := tx.QueryRow(ctx, "select app_escreve_na_casa($1)", casaID).Scan(&escrevo); err != nil {
		return "", err
	}
	if !escrevo {
		return "", ErrSemPermissaoCasa
	}
	if podem != len(ids) {
		return "", ErrCampo{"só dá para dividir com donos e membros desta casa"}
	}
	var ja bool
	if err := tx.QueryRow(ctx, "select exists (select 1 from despesas_divididas where transacao_id = $1)", p.TransacaoID).Scan(&ja); err != nil {
		return "", err
	}
	if ja {
		return "", ErrJaDividida
	}
	var id string
	err = tx.QueryRow(ctx, `insert into despesas_divididas (casa_id, transacao_id, pago_por, descricao, data, total_centavos, moeda, modo)
		values ($1, $2, $3, $4, $5, $6, $7, $8) returning id`, casaID, p.TransacaoID, eu, descricao, data, int64(total), moeda, p.Modo).
		Scan(&id)
	if err != nil {
		return "", err
	}
	for _, pt := range partes {
		// a parte de quem pagou já nasce acertada; o RLS barra quem não é da casa
		if _, err := tx.Exec(ctx, `insert into divisoes (despesa_id, usuario_id, valor_centavos, acertado_em)
			values ($1, $2, $3, case when $2::uuid = $4::uuid then now() end)`, id, pt.UsuarioID, int64(pt.Valor), eu); err != nil {
			return "", err
		}
	}
	return id, nil
}

// ApagarDespesaDividida (só quem pagou).
func ApagarDespesaDividida(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx, "delete from despesas_divididas where id = $1", id)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

// Acerto entre duas pessoas.
type Acerto struct {
	De    string `json:"de_usuario"`
	Para  string `json:"para_usuario"`
	Valor int64  `json:"valor_centavos"`
}

// Acertar tudo o que está em aberto com a outra pessoa na casa. Sem nada em aberto,
// devolve o acerto zerado.
func Acertar(ctx context.Context, tx pgx.Tx, casaID, outro string) (Acerto, error) {
	var a Acerto
	err := tx.QueryRow(ctx, "select de_usuario, para_usuario, valor_centavos from app_acertar($1, $2)", casaID, outro).
		Scan(&a.De, &a.Para, &a.Valor)
	if errors.Is(err, pgx.ErrNoRows) {
		return Acerto{}, nil
	}
	return a, err
}

// DadosMetaCasa para criar uma meta conjunta.
type DadosMetaCasa struct {
	Nome     string  `json:"nome"`
	Tipo     string  `json:"tipo"`
	Alvo     int64   `json:"alvo_centavos"`
	DataAlvo *string `json:"data_alvo"`
}

// CriarMetaCasa: a meta fica na PF de quem cria, visível para a casa.
func CriarMetaCasa(ctx context.Context, tx pgx.Tx, casaID string, d DadosMetaCasa) (string, error) {
	if d.Nome == "" || len(d.Nome) > 100 {
		return "", ErrCampo{"dê um nome à meta"}
	}
	if d.Alvo <= 0 {
		return "", ErrCampo{"o valor da meta precisa ser maior que zero"}
	}
	switch d.Tipo {
	case "":
		d.Tipo = "outra"
	case "reserva", "viagem", "compra", "aposentadoria", "outra":
	default:
		return "", ErrCampo{"tipo: reserva, viagem, compra, aposentadoria ou outra"}
	}
	var dataAlvo *time.Time
	if d.DataAlvo != nil && *d.DataAlvo != "" {
		t, err := time.Parse("2006-01-02", *d.DataAlvo)
		if err != nil {
			return "", ErrCampo{"data inválida"}
		}
		dataAlvo = &t
	}
	var escrevo bool
	if err := tx.QueryRow(ctx, "select app_escreve_na_casa($1)", casaID).Scan(&escrevo); err != nil {
		return "", err
	}
	if !escrevo {
		return "", ErrSemPermissaoCasa
	}
	var ent string
	if err := tx.QueryRow(ctx, `select id from entidades where dono_id = app_usuario_id()
		order by tipo = 'PF' desc, criada_em limit 1`).Scan(&ent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrCampo{"crie sua pessoa física antes"}
		}
		return "", err
	}
	var id string
	err := tx.QueryRow(ctx, `insert into metas (entidade_id, casa_id, nome, tipo, alvo_centavos, data_alvo)
		values ($1, $2, $3, $4::tipo_meta, $5, $6) returning id`, ent, casaID, d.Nome, d.Tipo, d.Alvo, dataAlvo).Scan(&id)
	return id, err
}

// Contribuir com uma meta conjunta.
func Contribuir(ctx context.Context, tx pgx.Tx, metaID string, valor int64, data time.Time) (string, error) {
	if valor <= 0 {
		return "", ErrCampo{"o valor precisa ser maior que zero"}
	}
	var id string
	err := tx.QueryRow(ctx, `insert into contribuicoes_meta (meta_id, usuario_id, valor_centavos, data)
		values ($1, app_usuario_id(), $2, $3) returning id`, metaID, valor, data).Scan(&id)
	return id, err
}
