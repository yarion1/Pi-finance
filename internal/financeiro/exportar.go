package financeiro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"github.com/yarion1/pi-finance/internal/cripto"
)

// Exportação dos dados da pessoa (LGPD, SPEC §10): tudo o que é dela, em JSON, e as
// tabelas principais em planilha. Segredos nunca saem: colunas cifradas, hashes e tokens
// ficam de fora; o CPF/CNPJ sai decifrado (é da própria pessoa).

// Tabelas que nunca entram: autenticação, operação e o que é só do servidor.
var foraDaExportacao = map[string]bool{
	"sessoes": true, "dois_fatores": true, "codigos_recuperacao": true, "passkeys": true,
	"desafios_webauthn": true, "push_inscricoes": true, "notificacoes_enviadas": true,
	"sinais_vida": true, "estado_servidor": true, "goose_db_version": true, "convites_casa": true,
	"convites_conta": true, "usuarios": true, "entidades": true, "cotacoes": true, "indices": true,
	"instituicoes": true, "regras_fiscais": true,
}

// colunas com segredo (cifradas, hashes, tokens): nunca vão para a exportação
var colunaSecreta = regexp.MustCompile(`(_cifrad[oa]|_hash|senha|segredo|token|secret)`)

// Exportacao: o arquivo JSON.
type Exportacao struct {
	GeradoEm  time.Time                    `json:"gerado_em"`
	Aviso     string                       `json:"aviso"`
	Usuario   json.RawMessage              `json:"usuario"`
	Entidades []map[string]any             `json:"entidades"`
	Tabelas   map[string][]json.RawMessage `json:"tabelas"`
}

type tabelaExportavel struct {
	nome    string
	colunas map[string]bool
}

func tabelasExportaveis(ctx context.Context, tx pgx.Tx) ([]tabelaExportavel, error) {
	linhas, err := tx.Query(ctx, `select c.table_name, c.column_name from information_schema.columns c
		join information_schema.tables t using (table_schema, table_name)
		where c.table_schema = 'public' and t.table_type = 'BASE TABLE' and c.table_name not like 'river%'
		order by c.table_name, c.ordinal_position`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	por := map[string]*tabelaExportavel{}
	var ordem []string
	for linhas.Next() {
		var t, c string
		if err := linhas.Scan(&t, &c); err != nil {
			return nil, err
		}
		if foraDaExportacao[t] {
			continue
		}
		if por[t] == nil {
			por[t] = &tabelaExportavel{nome: t, colunas: map[string]bool{}}
			ordem = append(ordem, t)
		}
		por[t].colunas[c] = true
	}
	out := make([]tabelaExportavel, 0, len(ordem))
	for _, t := range ordem {
		out = append(out, *por[t])
	}
	return out, linhas.Err()
}

// ExportarDados de quem está na sessão.
func ExportarDados(ctx context.Context, tx pgx.Tx, cif *cripto.Cifrador, agora time.Time) (Exportacao, error) {
	e := Exportacao{GeradoEm: agora, Tabelas: map[string][]json.RawMessage{},
		Aviso: "Seus dados no painel Finanças. Senhas, segredos do 2FA e credenciais ficam de fora."}
	if err := tx.QueryRow(ctx, `select to_jsonb(u) - array['senha_hash', 'tentativas_falhas', 'bloqueado_ate']
		from usuarios u where id = app_usuario_id()`).Scan(&e.Usuario); err != nil {
		return e, err
	}
	// entidades de que a pessoa é dona, com o documento decifrado
	linhas, err := tx.Query(ctx, `select id, to_jsonb(e) - 'documento_cifrado', documento_cifrado from entidades e
		where dono_id = app_usuario_id() order by criada_em`)
	if err != nil {
		return e, err
	}
	var ents []string
	for linhas.Next() {
		var id string
		var j []byte
		var doc *string
		if err := linhas.Scan(&id, &j, &doc); err != nil {
			linhas.Close()
			return e, err
		}
		m := map[string]any{}
		_ = json.Unmarshal(j, &m)
		if doc != nil && cif != nil {
			if d, err := cif.Decifrar(*doc, "entidades.documento"); err == nil {
				m["documento"] = d
			}
		}
		ents = append(ents, id)
		e.Entidades = append(e.Entidades, m)
	}
	linhas.Close()
	if e.Entidades == nil {
		e.Entidades = []map[string]any{}
	}

	tabelas, err := tabelasExportaveis(ctx, tx)
	if err != nil {
		return e, err
	}
	for _, t := range tabelas {
		var onde string
		switch {
		case t.colunas["entidade_id"]:
			onde = "entidade_id = any($2::uuid[])"
		case t.colunas["usuario_id"]:
			onde = "usuario_id = app_usuario_id()"
		case t.colunas["pago_por"]:
			onde = "pago_por = app_usuario_id()"
		case t.nome == "acertos":
			onde = "(de_usuario = app_usuario_id() or para_usuario = app_usuario_id())"
		case t.nome == "casas":
			onde = "app_eh_membro_casa(id)"
		default:
			continue
		}
		secretas := []string{}
		for c := range t.colunas {
			if colunaSecreta.MatchString(c) {
				secretas = append(secretas, c)
			}
		}
		sort.Strings(secretas)
		var linhasJSON []json.RawMessage
		sql := fmt.Sprintf("select to_jsonb(t) - $1::text[] from %s t where %s", pgx.Identifier{t.nome}.Sanitize(), onde)
		args := []any{secretas}
		if t.colunas["entidade_id"] {
			args = append(args, ents)
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return e, fmt.Errorf("exportar %s: %w", t.nome, err)
		}
		for rows.Next() {
			var j json.RawMessage
			if err := rows.Scan(&j); err != nil {
				rows.Close()
				return e, err
			}
			linhasJSON = append(linhasJSON, j)
		}
		rows.Close()
		if len(linhasJSON) > 0 {
			e.Tabelas[t.nome] = linhasJSON
		}
	}
	return e, nil
}

// PlanilhaDados: contas, transações, operações e metas da pessoa num .xlsx (valores em
// reais só para leitura; o JSON tem os centavos).
func PlanilhaDados(ctx context.Context, tx pgx.Tx) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	reais := 0
	if st, err := f.NewStyle(&excelize.Style{NumFmt: 4}); err == nil { // #,##0.00
		reais = st
	}
	folhas := []struct {
		nome, sql string
		cab       []string
		dinheiro  map[int]bool
	}{
		{"Contas", `select c.nome, c.tipo::text, c.moeda, e.nome, c.visibilidade::text, (coalesce(app_saldo_conta(c.id), 0) / 100.0)::float8
			from contas c join entidades e on e.id = c.entidade_id where e.dono_id = app_usuario_id() order by e.nome, c.nome`,
			[]string{"Conta", "Tipo", "Moeda", "Entidade", "Visibilidade", "Saldo"}, map[int]bool{5: true}},
		{"Transações", `select t.data::text, c.nome, t.descricao, coalesce(cat.nome, ''), t.tipo::text, (t.valor_centavos / 100.0)::float8,
				t.moeda, coalesce(t.notas, '')
			from transacoes t join contas c on c.id = t.conta_id join entidades e on e.id = t.entidade_id
			left join categorias cat on cat.id = t.categoria_id
			where e.dono_id = app_usuario_id() order by t.data, t.id`,
			[]string{"Data", "Conta", "Descrição", "Categoria", "Tipo", "Valor", "Moeda", "Notas"}, map[int]bool{5: true}},
		{"Investimentos", `select o.data::text, coalesce(a.nome, a.codigo), o.tipo::text, o.quantidade::text, o.preco::text, (o.valor_centavos / 100.0)::float8
			from operacoes o join ativos a on a.id = o.ativo_id join entidades e on e.id = o.entidade_id
			where e.dono_id = app_usuario_id() order by o.data`,
			[]string{"Data", "Ativo", "Operação", "Quantidade", "Preço", "Valor (proventos, juros)"}, map[int]bool{5: true}},
		{"Metas", `select m.nome, m.tipo::text, (m.alvo_centavos / 100.0)::float8, coalesce(m.data_alvo::text, '')
			from metas m join entidades e on e.id = m.entidade_id where e.dono_id = app_usuario_id() order by m.nome`,
			[]string{"Meta", "Tipo", "Alvo", "Data"}, map[int]bool{2: true}},
	}
	for i, fl := range folhas {
		if i == 0 {
			if err := f.SetSheetName("Sheet1", fl.nome); err != nil {
				return nil, err
			}
		} else if _, err := f.NewSheet(fl.nome); err != nil {
			return nil, err
		}
		for c, t := range fl.cab {
			cel, _ := excelize.CoordinatesToCellName(c+1, 1)
			_ = f.SetCellStr(fl.nome, cel, t)
		}
		linhas, err := tx.Query(ctx, fl.sql)
		if err != nil {
			return nil, fmt.Errorf("planilha %s: %w", fl.nome, err)
		}
		n := 2
		for linhas.Next() {
			vals, err := linhas.Values()
			if err != nil {
				linhas.Close()
				return nil, err
			}
			for c, v := range vals {
				cel, _ := excelize.CoordinatesToCellName(c+1, n)
				if fl.dinheiro[c] {
					_ = f.SetCellValue(fl.nome, cel, v)
					_ = f.SetCellStyle(fl.nome, cel, cel, reais)
				} else {
					// texto sempre como texto: nada vira fórmula na planilha
					_ = f.SetCellStr(fl.nome, cel, fmt.Sprint(v))
				}
			}
			n++
		}
		linhas.Close()
		if err := linhas.Err(); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
