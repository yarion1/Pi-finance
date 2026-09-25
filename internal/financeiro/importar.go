package financeiro

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/importadores"
)

// ErrContaNaoEditavel: a conta não existe para o usuário ou ele só pode ler.
var ErrContaNaoEditavel = errors.New("conta inexistente ou sem permissão para importar")

// LinhaPrevia é uma linha do arquivo com o que vai acontecer com ela.
type LinhaPrevia struct {
	Data      string        `json:"data"`
	Descricao string        `json:"descricao"`
	Valor     core.Centavos `json:"valor_centavos"`
	Duplicada bool          `json:"duplicada"`
	Categoria *string       `json:"categoria_id"`
	Origem    string        `json:"origem_categoria,omitempty"`
}

// ResultadoImportacao é a prévia (simulada) ou o resumo do que foi gravado.
type ResultadoImportacao struct {
	ImportacaoID   string         `json:"importacao_id,omitempty"`
	Formato        string         `json:"formato"`
	Linhas         int            `json:"linhas"`
	Novas          int            `json:"novas"`
	Duplicadas     int            `json:"duplicadas"`
	Casadas        int            `json:"casadas,omitempty"` // já estavam lá vindas da outra fonte (arquivo x Open Finance)
	Transferencias int            `json:"transferencias"`
	Categorizadas  int            `json:"categorizadas"`
	Avisos         []string       `json:"avisos,omitempty"`
	Previa         []LinhaPrevia  `json:"previa,omitempty"`
	SaldoArquivo   *core.Centavos `json:"saldo_arquivo_centavos,omitempty"`
	SaldoSistema   *core.Centavos `json:"saldo_sistema_centavos,omitempty"`
}

type linhaChave struct {
	importadores.Linha
	chave string
}

// Importar passa as linhas pela fila: deduplica (conta + data + valor + descrição
// normalizada + id externo), categoriza e pareia transferências internas. Com
// simular=true nada é gravado e volta a prévia linha a linha.
func Importar(ctx context.Context, tx pgx.Tx, usuarioID, contaID, arquivo string, r importadores.Resultado, simular bool) (ResultadoImportacao, error) {
	res := ResultadoImportacao{Formato: r.Formato, Linhas: len(r.Linhas), Avisos: r.Avisos, SaldoArquivo: r.SaldoFinal}
	conta, err := InfoContaEditavel(ctx, tx, contaID)
	if err != nil {
		return res, err
	}
	entidadeID, moeda := conta.EntidadeID, conta.Moeda
	if r.Moeda != "" && r.Moeda != moeda {
		return res, fmt.Errorf("%w: o arquivo está em %s e a conta em %s", ErrMoeda, r.Moeda, moeda)
	}

	// ordem entre linhas idênticas do arquivo: duas compras iguais no dia são duas
	vistas := map[string]int{}
	linhas := make([]linhaChave, len(r.Linhas))
	chaves := make([]string, len(r.Linhas))
	ids := []string{}
	for i, l := range r.Linhas {
		base := core.ChaveDedup(l.Data, l.Valor, l.Descricao, 0)
		vistas[base]++
		linhas[i] = linhaChave{l, core.ChaveDedup(l.Data, l.Valor, l.Descricao, vistas[base])}
		chaves[i] = linhas[i].chave
		if l.IDExterno != "" {
			ids = append(ids, l.IDExterno)
		}
	}

	// na Pluggy o id externo é o dela, guardado à parte do id do arquivo (FITID)
	pluggy := r.Formato == importadores.FormatoPluggy
	colunaID := "id_externo"
	if pluggy {
		colunaID = "id_pluggy"
	}
	existentes := map[string]bool{}
	rows, err := tx.Query(ctx, `select coalesce(chave_dedup, ''), coalesce(`+colunaID+`, '') from transacoes
		where conta_id = $1 and (chave_dedup = any($2) or `+colunaID+` = any($3))`, contaID, chaves, ids)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var c, e string
		if err := rows.Scan(&c, &e); err != nil {
			rows.Close()
			return res, err
		}
		existentes["c:"+c] = true
		existentes["e:"+e] = true
	}
	rows.Close()

	cat, err := NovoCategorizador(ctx, tx, entidadeID)
	if err != nil {
		return res, err
	}

	var importacaoID string
	if !simular {
		if err := tx.QueryRow(ctx, `insert into importacoes (entidade_id, conta_id, fonte, arquivo, linhas, criada_por)
			values ($1, $2, $3, $4, $5, $6) returning id`, entidadeID, contaID, r.Formato, arquivo, len(r.Linhas), usuarioID).
			Scan(&importacaoID); err != nil {
			return res, err
		}
	}

	var novas []string
	casadas := map[string]bool{}
	for _, l := range linhas {
		dup := existentes["c:"+l.chave] || (l.IDExterno != "" && existentes["e:"+l.IDExterno])
		if !dup {
			// a mesma transação pode já ter vindo pela outra fonte (DECISOES D21)
			id, err := casar(ctx, tx, contaID, l, pluggy, casadas, simular)
			if err != nil {
				return res, err
			}
			if id != "" {
				dup = true
				res.Casadas++
			}
		}
		categoria, origem := cat.Sugerir(contaID, l.Descricao, l.Valor)
		if dup {
			res.Duplicadas++
		} else {
			res.Novas++
			if categoria != nil {
				res.Categorizadas++
			}
		}
		if simular {
			res.Previa = append(res.Previa, LinhaPrevia{
				Data: l.Data.Format("2006-01-02"), Descricao: l.Descricao, Valor: l.Valor,
				Duplicada: dup, Categoria: categoria, Origem: origem,
			})
			continue
		}
		if dup {
			continue
		}
		var id string
		var idExterno, idPluggy *string
		if l.IDExterno != "" {
			idExterno = &l.IDExterno
		}
		origemTx := "csv"
		switch {
		case pluggy:
			origemTx, idExterno, idPluggy = "pluggy", nil, idExterno
		case r.Formato == importadores.FormatoOFX:
			origemTx = "ofx"
		}
		// "Loja - Parcela 3/12" vira parcela 3 de 12 (as futuras entram no comprometido)
		var parcelaN, parcelaTotal *int
		if _, n, total, ok := core.ExtrairParcela(l.Descricao); ok {
			parcelaN, parcelaTotal = &n, &total
		}
		err := tx.QueryRow(ctx, `insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao,
				valor_centavos, moeda, categoria_id, tipo, origem, id_externo, importacao_id, chave_dedup,
				fatura_em, parcela_n, parcela_total, id_pluggy)
			values ($1, $2, $3, $4, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			on conflict do nothing returning id`,
			entidadeID, contaID, l.Data, l.Descricao, int64(l.Valor), moeda, categoria, TipoPorValor(l.Valor),
			origemTx, idExterno, importacaoID, l.chave, conta.FaturaEm(l.Data), parcelaN, parcelaTotal, idPluggy).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			// corrida com outra importação: conta como duplicada
			res.Novas--
			res.Duplicadas++
			continue
		}
		if err != nil {
			return res, err
		}
		novas = append(novas, id)
	}

	if simular {
		return res, nil
	}
	if res.Transferencias, err = ParearTransferencias(ctx, tx, novas); err != nil {
		return res, err
	}
	var saldo int64
	if err := tx.QueryRow(ctx, "select app_saldo_conta($1)", contaID).Scan(&saldo); err == nil {
		s := core.Centavos(saldo)
		res.SaldoSistema = &s
	}
	// contas fixas e assinaturas novas aparecem a cada extrato
	if _, err := DetectarRecorrencias(ctx, tx, entidadeID, time.Now().In(fusoSP)); err != nil {
		return res, err
	}
	res.ImportacaoID = importacaoID
	_, err = tx.Exec(ctx, "update importacoes set novas = $2, duplicadas = $3, transferencias = $4 where id = $1",
		importacaoID, res.Novas, res.Duplicadas, res.Transferencias)
	return res, err
}

// casar procura, na mesma conta, a transação que já veio pela outra fonte: mesmo
// valor, até 1 dia de diferença, ainda sem par. Linha da Pluggy casa com o que veio
// de arquivo ou foi lançado à mão; linha de arquivo casa com o que veio da Pluggy.
// Grava o id da nova fonte na existente (a próxima importação reconhece direto) e
// devolve o id dela, ou "" se não achou.
func casar(ctx context.Context, tx pgx.Tx, contaID string, l linhaChave, pluggy bool, usadas map[string]bool, simular bool) (string, error) {
	filtro := "id_pluggy is null and origem <> 'pluggy'"
	if !pluggy {
		filtro = "origem = 'pluggy' and not casada"
	}
	usadasLista := make([]string, 0, len(usadas))
	for id := range usadas {
		usadasLista = append(usadasLista, id)
	}
	var id string
	err := tx.QueryRow(ctx, `select id from transacoes
		where conta_id = $1 and `+filtro+` and valor_centavos = $2 and data between $3::date - 1 and $3::date + 1
		  and not (id = any($4::uuid[]))
		order by abs(data - $3::date), criada_em limit 1`, contaID, int64(l.Valor), l.Data, usadasLista).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	usadas[id] = true
	if simular {
		return id, nil
	}
	if pluggy {
		_, err = tx.Exec(ctx, "update transacoes set id_pluggy = $2, casada = true where id = $1", id, l.IDExterno)
	} else {
		var idExterno *string
		if l.IDExterno != "" {
			idExterno = &l.IDExterno
		}
		// a chave do arquivo passa a ser a dela: reimportar o mesmo arquivo reconhece
		_, err = tx.Exec(ctx, `update transacoes set casada = true, chave_dedup = $2, id_externo = coalesce(id_externo, $3)
			where id = $1`, id, l.chave, idExterno)
	}
	return id, err
}

// ErrMoeda é devolvido quando o arquivo e a conta têm moedas diferentes.
var ErrMoeda = errors.New("moeda diferente")

// ParearTransferencias: saída numa conta e entrada do mesmo valor em outra conta
// da mesma entidade, em até 2 dias, viram uma transferência (não são gasto nem
// receita). Devolve quantos pares formou.
func ParearTransferencias(ctx context.Context, tx pgx.Tx, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var categoria *string
	_ = tx.QueryRow(ctx, `select id from categorias where entidade_id is null and tipo = 'transferencia'
		and nome = 'Transferência entre contas'`).Scan(&categoria)

	pares := 0
	for _, id := range ids {
		var par string
		err := tx.QueryRow(ctx, `select o.id from transacoes t
			join transacoes o on o.entidade_id = t.entidade_id and o.conta_id <> t.conta_id
			  and o.valor_centavos = -t.valor_centavos and abs(o.data - t.data) <= 2
			  and o.transferencia_par_id is null and o.tipo in ('gasto', 'receita')
			where t.id = $1 and t.transferencia_par_id is null and t.tipo in ('gasto', 'receita')
			  and app_pode_editar_entidade(t.entidade_id)
			order by abs(o.data - t.data), o.criada_em
			limit 1`, id).Scan(&par)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return pares, err
		}
		if _, err := tx.Exec(ctx, `update transacoes set tipo = 'transferencia', categoria_id = $3,
			transferencia_par_id = case when id = $1 then $2::uuid else $1::uuid end
			where id in ($1, $2)`, id, par, categoria); err != nil {
			return pares, err
		}
		pares++
	}
	return pares, nil
}

// DesfazerPar volta as duas pontas de uma transferência a gasto/receita comuns.
func DesfazerPar(ctx context.Context, tx pgx.Tx, id string) error {
	_, err := tx.Exec(ctx, `update transacoes set transferencia_par_id = null, categoria_id = null,
		tipo = case when valor_centavos < 0 then 'gasto'::tipo_transacao else 'receita'::tipo_transacao end
		where id = $1 or transferencia_par_id = $1`, id)
	return err
}

// DesfazerImportacao apaga as transações da importação. As transferências que
// estavam pareadas com transações de fora dela voltam a ser gasto/receita.
func DesfazerImportacao(ctx context.Context, tx pgx.Tx, importacaoID string) (int, error) {
	var desfeita *time.Time
	var pode bool
	err := tx.QueryRow(ctx, "select desfeita_em, app_pode_editar_entidade(entidade_id) from importacoes where id = $1",
		importacaoID).Scan(&desfeita, &pode)
	if err != nil {
		return 0, err
	}
	if !pode {
		return 0, ErrContaNaoEditavel
	}
	if desfeita != nil {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `update transacoes set transferencia_par_id = null, categoria_id = null,
		tipo = case when valor_centavos < 0 then 'gasto'::tipo_transacao else 'receita'::tipo_transacao end
		where importacao_id is distinct from $1
		  and transferencia_par_id in (select id from transacoes where importacao_id = $1)`, importacaoID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, "delete from transacoes where importacao_id = $1", importacaoID)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, "update importacoes set desfeita_em = now() where id = $1", importacaoID)
	return int(tag.RowsAffected()), err
}
