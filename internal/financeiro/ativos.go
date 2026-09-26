package financeiro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/importadores"
)

// ErrSemPermissao: a entidade (ou o ativo) não existe para quem pediu ou é só leitura.
var ErrSemPermissao = errors.New("sem permissão para editar esta entidade")

// ResultadoB3 da gravação de um extrato da B3.
type ResultadoB3 struct {
	Formato     string   `json:"formato"`
	Novas       int      `json:"novas"`
	Duplicadas  int      `json:"duplicadas"`
	AtivosNovos []string `json:"ativos_novos"`
	Avisos      []string `json:"avisos"`
}

// cotacaoPadrao: ações, FIIs, ETFs e BDRs têm cotação pelo próprio ticker (brapi).
func cotacaoPadrao(classe, codigo string) *string {
	switch classe {
	case core.ClasseAcao, core.ClasseFII, core.ClasseETF, core.ClasseBDR:
		return &codigo
	}
	return nil
}

// PodeEditarEntidade confere pelo RLS se o usuário da sessão escreve na entidade.
func PodeEditarEntidade(ctx context.Context, tx pgx.Tx, entidadeID string) error {
	var pode *bool
	if err := tx.QueryRow(ctx, "select app_pode_editar_entidade($1::uuid)", entidadeID).Scan(&pode); err != nil {
		return err
	}
	if pode == nil || !*pode {
		return ErrSemPermissao
	}
	return nil
}

// ImportarB3 grava as operações e eventos do extrato na entidade, criando os ativos que
// faltam. A chave de cada linha evita duplicar ao importar o mesmo arquivo (ou um
// período sobreposto) de novo. Com simular=true só conta.
func ImportarB3(ctx context.Context, tx pgx.Tx, entidadeID string, lido importadores.ResultadoB3, simular bool) (ResultadoB3, error) {
	res := ResultadoB3{Formato: lido.Formato, AtivosNovos: []string{}, Avisos: lido.Avisos}
	if res.Avisos == nil {
		res.Avisos = []string{}
	}
	if err := PodeEditarEntidade(ctx, tx, entidadeID); err != nil {
		return res, err
	}
	ativos := map[string]string{} // código → id
	origem := "b3"
	if lido.Formato == importadores.FormatoNotaCorretagem {
		origem = "documento"
	}
	for _, l := range lido.Linhas {
		var existe bool
		tabela := "operacoes"
		if l.Tipo == importadores.TipoEvento {
			tabela = "eventos_corporativos"
		}
		if err := tx.QueryRow(ctx, fmt.Sprintf("select exists (select 1 from %s where entidade_id = $1 and chave_dedup = $2)", tabela),
			entidadeID, l.Chave).Scan(&existe); err != nil {
			return res, err
		}
		if existe {
			res.Duplicadas++
			continue
		}
		res.Novas++
		id, ok := ativos[l.Codigo]
		if !ok {
			err := tx.QueryRow(ctx, "select id from ativos where entidade_id = $1 and codigo = $2", entidadeID, l.Codigo).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				res.AtivosNovos = append(res.AtivosNovos, l.Codigo)
				if !simular {
					nome := l.Nome
					if nome == "" {
						nome = l.Codigo
					}
					err = tx.QueryRow(ctx, `insert into ativos (entidade_id, codigo, nome, classe, cotacao_codigo, origem, instituicao)
						values ($1, $2, $3, $4::classe_ativo, $5, 'b3', nullif($6, '')) returning id`,
						entidadeID, l.Codigo, nome, l.Classe, cotacaoPadrao(l.Classe, l.Codigo), l.Instituicao).Scan(&id)
				} else {
					err = nil
				}
			}
			if err != nil {
				return res, err
			}
			ativos[l.Codigo] = id
		}
		if simular {
			continue
		}
		var err error
		if l.Tipo == importadores.TipoEvento {
			_, err = tx.Exec(ctx, `insert into eventos_corporativos (entidade_id, ativo_id, data, tipo, fator, custo_unitario, chave_dedup)
				values ($1, $2, $3, 'ajuste', $4::numeric, $5::numeric, $6)`,
				entidadeID, id, l.Data, l.Quantidade.String(), l.Preco.String(), l.Chave)
		} else {
			_, err = tx.Exec(ctx, `insert into operacoes (entidade_id, ativo_id, data, tipo, quantidade, preco, valor_centavos,
					taxas_centavos, descricao, origem, chave_dedup)
				values ($1, $2, $3, $4::tipo_operacao, $5::numeric, $6::numeric, $7, $8, $9, $10, $11)`,
				entidadeID, id, l.Data, l.Tipo, l.Quantidade.String(), l.Preco.String(), int64(l.Valor), int64(l.Taxas), l.Descricao,
				origem, l.Chave)
		}
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

// DadosAtivo para criar ou editar um ativo manual.
type DadosAtivo struct {
	EntidadeID    string  `json:"entidade_id"`
	Codigo        string  `json:"codigo"`
	Nome          string  `json:"nome"`
	Classe        string  `json:"classe"`
	Emissor       *string `json:"emissor"`
	Instituicao   *string `json:"instituicao"`
	Indexador     *string `json:"indexador"`
	Taxa          *string `json:"taxa"`
	Vencimento    *string `json:"vencimento"`
	IsentoIR      bool    `json:"isento_ir"`
	CotacaoCodigo *string `json:"cotacao_codigo"`
}

var classesValidas = map[string]bool{"acao": true, "fii": true, "etf": true, "bdr": true, "renda_fixa": true,
	"tesouro": true, "fundo": true, "cripto": true, "exterior": true, "previdencia": true, "outro": true}

// ErrCampo é um erro de validação para mostrar à pessoa.
type ErrCampo struct{ Mensagem string }

func (e ErrCampo) Error() string { return e.Mensagem }

func vazioNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	v := strings.TrimSpace(*s)
	return &v
}

func (d *DadosAtivo) validar() error {
	d.Codigo = strings.TrimSpace(d.Codigo)
	d.Nome = strings.TrimSpace(d.Nome)
	if d.Codigo == "" || len(d.Codigo) > 120 {
		return ErrCampo{"informe o código ou nome do ativo"}
	}
	if !classesValidas[d.Classe] {
		return ErrCampo{"classe inválida"}
	}
	if d.Nome == "" {
		d.Nome = d.Codigo
	}
	d.Emissor, d.Instituicao, d.Indexador = vazioNil(d.Emissor), vazioNil(d.Instituicao), vazioNil(d.Indexador)
	d.Taxa, d.Vencimento, d.CotacaoCodigo = vazioNil(d.Taxa), vazioNil(d.Vencimento), vazioNil(d.CotacaoCodigo)
	if d.Indexador != nil && *d.Indexador != "prefixado" && *d.Indexador != "cdi" && *d.Indexador != "ipca" {
		return ErrCampo{"indexador deve ser prefixado, cdi ou ipca"}
	}
	if d.Taxa != nil {
		if _, err := core.ParseDec8(*d.Taxa); err != nil {
			return ErrCampo{"taxa inválida (use ponto: 0.12 para 12 % a.a. ou 1.10 para 110 % do CDI)"}
		}
	}
	if d.Vencimento != nil {
		if _, err := time.Parse("2006-01-02", *d.Vencimento); err != nil {
			return ErrCampo{"vencimento inválido"}
		}
	}
	if d.CotacaoCodigo == nil {
		d.CotacaoCodigo = cotacaoPadrao(d.Classe, strings.ToUpper(d.Codigo))
	}
	return nil
}

// CriarAtivo manual.
func CriarAtivo(ctx context.Context, tx pgx.Tx, d DadosAtivo) (string, error) {
	if err := d.validar(); err != nil {
		return "", err
	}
	if err := PodeEditarEntidade(ctx, tx, d.EntidadeID); err != nil {
		return "", err
	}
	var id string
	err := tx.QueryRow(ctx, `insert into ativos (entidade_id, codigo, nome, classe, emissor, instituicao, indexador, taxa,
			vencimento, isento_ir, cotacao_codigo, origem)
		values ($1, $2, $3, $4::classe_ativo, $5, $6, $7, $8::numeric, $9::date, $10, $11, 'manual')
		on conflict (entidade_id, codigo) do nothing returning id`,
		d.EntidadeID, d.Codigo, d.Nome, d.Classe, d.Emissor, d.Instituicao, d.Indexador, d.Taxa, d.Vencimento,
		d.IsentoIR, d.CotacaoCodigo).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCampo{"já existe um ativo com este código nesta entidade"}
	}
	return id, err
}

// EditarAtivo: aplica só os campos enviados (o código e a entidade não mudam).
func EditarAtivo(ctx context.Context, tx pgx.Tx, id string, corpo []byte) error {
	var d DadosAtivo
	var taxa *string
	err := tx.QueryRow(ctx, `select entidade_id, codigo, coalesce(nome, codigo), classe::text, emissor, instituicao, indexador,
			taxa::text, to_char(vencimento, 'YYYY-MM-DD'), isento_ir, cotacao_codigo
		from ativos where id = $1`, id).Scan(&d.EntidadeID, &d.Codigo, &d.Nome, &d.Classe, &d.Emissor, &d.Instituicao,
		&d.Indexador, &taxa, &d.Vencimento, &d.IsentoIR, &d.CotacaoCodigo)
	if err != nil {
		return err
	}
	if taxa != nil {
		t := limparDec(*taxa)
		d.Taxa = &t
	}
	if err := PodeEditarEntidade(ctx, tx, d.EntidadeID); err != nil {
		return err
	}
	ent, codigo := d.EntidadeID, d.Codigo
	var mudou struct {
		Classe *string `json:"classe"`
	}
	estrito := json.NewDecoder(bytes.NewReader(corpo))
	estrito.DisallowUnknownFields()
	if estrito.Decode(&d) != nil || json.Unmarshal(corpo, &mudou) != nil {
		return ErrCampo{"dados inválidos"}
	}
	d.EntidadeID, d.Codigo = ent, codigo
	if mudou.Classe != nil && d.CotacaoCodigo != nil && *d.CotacaoCodigo == codigo {
		d.CotacaoCodigo = nil // volta ao padrão da classe nova
	}
	if err := d.validar(); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `update ativos set nome = $2, classe = $3::classe_ativo, emissor = $4, instituicao = $5,
			indexador = $6, taxa = $7::numeric, vencimento = $8::date, isento_ir = $9, cotacao_codigo = $10
		where id = $1`, id, d.Nome, d.Classe, d.Emissor, d.Instituicao, d.Indexador, d.Taxa, d.Vencimento, d.IsentoIR,
		d.CotacaoCodigo)
	return err
}

// ApagarAtivo com as operações (os do Open Finance voltam na próxima sincronização).
func ApagarAtivo(ctx context.Context, tx pgx.Tx, id string) error {
	var ent string
	if err := tx.QueryRow(ctx, "select entidade_id from ativos where id = $1", id).Scan(&ent); err != nil {
		return err
	}
	if err := PodeEditarEntidade(ctx, tx, ent); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "delete from ativos where id = $1", id)
	return err
}

// OperacaoAtivo é uma operação ou evento para a lista do ativo.
type OperacaoAtivo struct {
	ID         string        `json:"id"`
	Data       string        `json:"data"`
	Tipo       string        `json:"tipo"`
	Quantidade string        `json:"quantidade"`
	Preco      string        `json:"preco"`
	Valor      core.Centavos `json:"valor_centavos"`
	Taxas      core.Centavos `json:"taxas_centavos"`
	IRRetido   core.Centavos `json:"ir_retido_centavos"`
	Descricao  *string       `json:"descricao"`
	Origem     string        `json:"origem"`
	Evento     bool          `json:"evento"`
}

// ListarOperacoes do ativo, mais recentes primeiro, com os eventos.
func ListarOperacoes(ctx context.Context, tx pgx.Tx, ativoID string) ([]OperacaoAtivo, error) {
	linhas, err := tx.Query(ctx, `
		select id, to_char(data, 'YYYY-MM-DD'), tipo::text, quantidade::text, preco::text, valor_centavos, taxas_centavos,
			ir_retido_centavos, descricao, origem, false, criada_em
		from operacoes where ativo_id = $1
		union all
		select id, to_char(data, 'YYYY-MM-DD'), tipo::text, fator::text, custo_unitario::text, 0, 0, 0, null, 'evento', true, criado_em
		from eventos_corporativos where ativo_id = $1
		order by 2 desc, 12 desc`, ativoID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	lista := []OperacaoAtivo{}
	for linhas.Next() {
		var o OperacaoAtivo
		var criada time.Time
		if err := linhas.Scan(&o.ID, &o.Data, &o.Tipo, &o.Quantidade, &o.Preco, &o.Valor, &o.Taxas, &o.IRRetido,
			&o.Descricao, &o.Origem, &o.Evento, &criada); err != nil {
			return nil, err
		}
		o.Quantidade, o.Preco = limparDec(o.Quantidade), limparDec(o.Preco)
		lista = append(lista, o)
	}
	return lista, linhas.Err()
}

// limparDec: "10.50000000" → "10.5".
func limparDec(s string) string {
	if d, err := core.ParseDec8(s); err == nil {
		return d.String()
	}
	return s
}

// DadosOperacao para lançar uma operação manual.
type DadosOperacao struct {
	Data       string `json:"data"`
	Tipo       string `json:"tipo"`
	Quantidade string `json:"quantidade"`
	Preco      string `json:"preco"`
	Valor      int64  `json:"valor_centavos"`
	Taxas      int64  `json:"taxas_centavos"`
	IRRetido   int64  `json:"ir_retido_centavos"`
	Descricao  string `json:"descricao"`
}

// CriarOperacao manual no ativo. Venda não pode passar da quantidade na data.
func CriarOperacao(ctx context.Context, tx pgx.Tx, ativoID string, d DadosOperacao) (string, error) {
	var ent, origem string
	if err := tx.QueryRow(ctx, "select entidade_id, origem from ativos where id = $1", ativoID).Scan(&ent, &origem); err != nil {
		return "", err
	}
	if err := PodeEditarEntidade(ctx, tx, ent); err != nil {
		return "", err
	}
	if origem == "pluggy" {
		return "", ErrCampo{"este investimento vem do banco: o valor é o que a instituição informa"}
	}
	data, err := time.Parse("2006-01-02", d.Data)
	if err != nil {
		return "", ErrCampo{"data inválida"}
	}
	qtd, preco := core.Dec8(0), core.Dec8(0)
	switch d.Tipo {
	case "compra", "venda":
		if qtd, err = core.ParseDec8(d.Quantidade); err != nil || qtd <= 0 {
			return "", ErrCampo{"quantidade inválida"}
		}
		if preco, err = core.ParseDec8(d.Preco); err != nil || preco < 0 {
			return "", ErrCampo{"preço inválido"}
		}
		d.Valor = 0
	case "provento", "juros", "amortizacao":
		if d.Valor <= 0 {
			return "", ErrCampo{"informe o valor recebido"}
		}
	default:
		return "", ErrCampo{"tipo inválido"}
	}
	if d.Taxas < 0 || d.IRRetido < 0 {
		return "", ErrCampo{"taxas e IR retido não podem ser negativos"}
	}
	var id string
	err = tx.QueryRow(ctx, `insert into operacoes (entidade_id, ativo_id, data, tipo, quantidade, preco, valor_centavos,
			taxas_centavos, ir_retido_centavos, descricao, origem)
		values ($1, $2, $3, $4::tipo_operacao, $5::numeric, $6::numeric, $7, $8, $9, nullif($10, ''), 'manual') returning id`,
		ent, ativoID, data, d.Tipo, qtd.String(), preco.String(), d.Valor, d.Taxas, d.IRRetido,
		strings.TrimSpace(d.Descricao)).Scan(&id)
	if err != nil {
		return "", err
	}
	ops, evs, err := operacoesDoAtivo(ctx, tx, ativoID)
	if err != nil {
		return "", err
	}
	if _, err := core.CalcularPosicao(ops, evs, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		return "", ErrCampo{"a venda passa da quantidade que existia na data"}
	}
	return id, nil
}

// ApagarOperacao (ou evento) do ativo.
func ApagarOperacao(ctx context.Context, tx pgx.Tx, ativoID, id string) error {
	var ent string
	if err := tx.QueryRow(ctx, "select entidade_id from ativos where id = $1", ativoID).Scan(&ent); err != nil {
		return err
	}
	if err := PodeEditarEntidade(ctx, tx, ent); err != nil {
		return err
	}
	t1, err := tx.Exec(ctx, "delete from operacoes where id = $1 and ativo_id = $2", id, ativoID)
	if err != nil {
		return err
	}
	t2, err := tx.Exec(ctx, "delete from eventos_corporativos where id = $1 and ativo_id = $2", id, ativoID)
	if err != nil {
		return err
	}
	if t1.RowsAffected()+t2.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	ops, evs, err := operacoesDoAtivo(ctx, tx, ativoID)
	if err != nil {
		return err
	}
	if _, err := core.CalcularPosicao(ops, evs, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		return ErrCampo{"sem esta operação, uma venda posterior passaria da quantidade"}
	}
	return nil
}

// ApuracaoIR de renda variável da entidade, com as vendas de cada mês.
type ApuracaoIR struct {
	Meses  []core.ApuracaoMes `json:"meses"`
	Vendas []VendaIR          `json:"vendas"`
	Fonte  string             `json:"fonte"`
}

// VendaIR para a tabela de vendas do mês.
type VendaIR struct {
	Mes      string        `json:"mes"`
	Data     string        `json:"data"`
	Ativo    string        `json:"ativo"`
	Classe   string        `json:"classe"`
	Valor    core.Centavos `json:"valor_centavos"`
	Custo    core.Centavos `json:"custo_centavos"`
	Ganho    core.Centavos `json:"ganho_centavos"`
	DayTrade bool          `json:"day_trade"`
}

// ApurarIR: vendas de ações, ETFs, BDRs e FIIs da entidade desde o início, apuradas mês
// a mês com as regras vigentes em cada mês.
func ApurarIR(ctx context.Context, tx pgx.Tx, entidadeID string) (ApuracaoIR, error) {
	out := ApuracaoIR{Meses: []core.ApuracaoMes{}, Vendas: []VendaIR{}}
	regras, err := CarregarRegras(ctx, tx, "ir_renda_variavel")
	if err != nil {
		return out, err
	}
	linhas, err := tx.Query(ctx, `select id, codigo, classe::text from ativos
		where entidade_id = $1 and classe in ('acao', 'fii', 'etf', 'bdr') and origem <> 'pluggy'`, entidadeID)
	if err != nil {
		return out, err
	}
	type at struct{ id, codigo, classe string }
	var ativos []at
	for linhas.Next() {
		var a at
		if err := linhas.Scan(&a.id, &a.codigo, &a.classe); err != nil {
			linhas.Close()
			return out, err
		}
		ativos = append(ativos, a)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return out, err
	}
	var vendas []core.VendaClassificada
	for _, a := range ativos {
		ops, evs, err := operacoesDoAtivo(ctx, tx, a.id)
		if err != nil {
			return out, err
		}
		p, err := core.CalcularPosicao(ops, evs, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			return out, err
		}
		for _, v := range p.Vendas {
			vendas = append(vendas, core.VendaClassificada{Venda: v, Classe: a.classe, Ativo: a.codigo})
			out.Vendas = append(out.Vendas, VendaIR{Mes: v.Data.Format("2006-01"), Data: v.Data.Format("2006-01-02"),
				Ativo: a.codigo, Classe: a.classe, Valor: v.Valor, Custo: v.Custo, Ganho: v.Ganho, DayTrade: v.DayTrade})
		}
	}
	var erroRegra error
	out.Meses = core.ApurarRendaVariavel(vendas, func(mes time.Time) core.RegrasRendaVariavel {
		var r core.RegrasRendaVariavel
		rg, err := core.RegraVigente(regras, "ir_renda_variavel", mes)
		if err == nil {
			err = json.Unmarshal(rg.Valor, &r)
			out.Fonte = rg.Fonte
		}
		if err != nil && erroRegra == nil {
			erroRegra = err
		}
		return r
	})
	if erroRegra != nil {
		return out, erroRegra
	}
	if out.Meses == nil {
		out.Meses = []core.ApuracaoMes{}
	}
	// mais recentes primeiro na tela
	for i, j := 0, len(out.Meses)-1; i < j; i, j = i+1, j-1 {
		out.Meses[i], out.Meses[j] = out.Meses[j], out.Meses[i]
	}
	return out, nil
}

// CarregarRegras de regras_fiscais de uma chave, com as vigências.
func CarregarRegras(ctx context.Context, tx pgx.Tx, chave string) ([]core.Regra, error) {
	linhas, err := tx.Query(ctx, `select chave, valor_json::text, vigente_de, vigente_ate, coalesce(fonte, '')
		from regras_fiscais where chave = $1`, chave)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var out []core.Regra
	for linhas.Next() {
		var r core.Regra
		var v string
		if err := linhas.Scan(&r.Chave, &v, &r.VigenteDe, &r.VigenteAte, &r.Fonte); err != nil {
			return nil, err
		}
		r.Valor = []byte(v)
		out = append(out, r)
	}
	return out, linhas.Err()
}
