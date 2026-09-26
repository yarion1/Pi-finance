package financeiro

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/ia"
)

// ErrIADesligada: a pessoa não ligou a IA nas configurações.
var ErrIADesligada = ErrCampo{"a IA está desligada: ligue em Mais › Inteligência artificial"}

// ErrTetoIA: o gasto com a API no mês chegou ao teto que a pessoa definiu.
var ErrTetoIA = ErrCampo{"o teto de gasto com a IA neste mês foi atingido"}

// ConfigIA de quem está logado, com o consumo do mês.
type ConfigIA struct {
	Ativa    bool   `json:"ativa"`
	TetoMes  int64  `json:"teto_mensal_microdolares"`
	UsoMes   ia.Uso `json:"uso_mes"`
	Servidor bool   `json:"servidor"` // o servidor tem a chave da API
}

// LerConfigIA do usuário da sessão (sem linha: desligada, teto padrão de US$ 5).
func LerConfigIA(ctx context.Context, tx pgx.Tx, hoje time.Time) (ConfigIA, error) {
	c := ConfigIA{TetoMes: 5_000_000}
	err := tx.QueryRow(ctx, `select ativa, teto_mensal_microdolares from ia_config where usuario_id = app_usuario_id()`).
		Scan(&c.Ativa, &c.TetoMes)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	err = tx.QueryRow(ctx, `select chamadas, tokens_entrada, tokens_saida, custo_microdolares from ia_uso
		where usuario_id = app_usuario_id() and mes = $1`, inicioMes(hoje)).
		Scan(&c.UsoMes.Chamadas, &c.UsoMes.Entrada, &c.UsoMes.Saida, &c.UsoMes.CustoMicro)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	return c, nil
}

// SalvarConfigIA do usuário da sessão.
func SalvarConfigIA(ctx context.Context, tx pgx.Tx, ativa bool, teto int64) error {
	if teto < 0 || teto > 1_000_000_000 {
		return ErrCampo{"teto entre US$ 0 e US$ 1.000 por mês"}
	}
	_, err := tx.Exec(ctx, `insert into ia_config (usuario_id, ativa, teto_mensal_microdolares) values (app_usuario_id(), $1, $2)
		on conflict (usuario_id) do update set ativa = excluded.ativa, teto_mensal_microdolares = excluded.teto_mensal_microdolares,
			atualizada_em = now()`, ativa, teto)
	return err
}

// PodeUsarIA: ligada e abaixo do teto do mês.
func PodeUsarIA(ctx context.Context, tx pgx.Tx, hoje time.Time) error {
	c, err := LerConfigIA(ctx, tx, hoje)
	if err != nil {
		return err
	}
	if !c.Ativa {
		return ErrIADesligada
	}
	if c.UsoMes.CustoMicro >= c.TetoMes {
		return ErrTetoIA
	}
	return nil
}

// RegistrarUsoIA soma o consumo ao mês do usuário da sessão.
func RegistrarUsoIA(ctx context.Context, tx pgx.Tx, hoje time.Time, u ia.Uso) error {
	if u.Chamadas == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `insert into ia_uso (usuario_id, mes, chamadas, tokens_entrada, tokens_saida, custo_microdolares)
		values (app_usuario_id(), $1, $2, $3, $4, $5)
		on conflict (usuario_id, mes) do update set chamadas = ia_uso.chamadas + excluded.chamadas,
			tokens_entrada = ia_uso.tokens_entrada + excluded.tokens_entrada,
			tokens_saida = ia_uso.tokens_saida + excluded.tokens_saida,
			custo_microdolares = ia_uso.custo_microdolares + excluded.custo_microdolares`,
		inicioMes(hoje), u.Chamadas, u.Entrada, u.Saida, u.CustoMicro)
	return err
}

// LotePendente: transações sem categoria de uma entidade e as categorias dela.
type LotePendente struct {
	EntidadeID string
	Itens      []ia.ItemCategorizar
	Categorias []ia.CategoriaOpcao
}

// TamanhoLoteIA: transações por chamada ao modelo.
const TamanhoLoteIA = 80

// PendentesIA: gastos e receitas sem categoria dos últimos 180 dias nas entidades em
// que o usuário pode lançar, em lotes por entidade (cada uma tem as suas categorias).
func PendentesIA(ctx context.Context, tx pgx.Tx, hoje time.Time, maximo int) ([]LotePendente, error) {
	linhas, err := tx.Query(ctx, `select t.entidade_id, t.id, t.descricao, t.valor_centavos from transacoes t
		where t.categoria_id is null and t.tipo in ('gasto', 'receita') and t.data > $1 and t.data <= $2
		  and app_pode_editar_entidade(t.entidade_id)
		order by t.entidade_id, t.data desc limit $3`, hoje.AddDate(0, 0, -180), hoje, maximo)
	if err != nil {
		return nil, err
	}
	var lotes []LotePendente
	idx := map[string]int{}
	for linhas.Next() {
		var ent string
		var it ia.ItemCategorizar
		if err := linhas.Scan(&ent, &it.ID, &it.Descricao, &it.Valor); err != nil {
			linhas.Close()
			return nil, err
		}
		i, ok := idx[ent]
		if !ok || len(lotes[i].Itens) >= TamanhoLoteIA {
			lotes = append(lotes, LotePendente{EntidadeID: ent})
			i = len(lotes) - 1
			idx[ent] = i
		}
		lotes[i].Itens = append(lotes[i].Itens, it)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, err
	}
	cache := map[string][]ia.CategoriaOpcao{}
	for i := range lotes {
		ent := lotes[i].EntidadeID
		if _, ok := cache[ent]; !ok {
			linhas, err := tx.Query(ctx, `select c.id, coalesce(p.nome || ' › ' || c.nome, c.nome), c.tipo::text
				from categorias c left join categorias p on p.id = c.pai_id
				where c.entidade_id = $1 or c.entidade_id is null order by 2`, ent)
			if err != nil {
				return nil, err
			}
			cats, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[ia.CategoriaOpcao])
			if err != nil {
				return nil, err
			}
			cache[ent] = cats
		}
		lotes[i].Categorias = cache[ent]
	}
	return lotes, nil
}

// AplicarCategoriasIA grava as sugestões só onde ainda não há categoria (nunca passa
// por cima da pessoa); o tipo acompanha a categoria (transferência sai do gasto).
func AplicarCategoriasIA(ctx context.Context, tx pgx.Tx, escolhas map[string]string) (int, error) {
	n := 0
	for transacao, categoria := range escolhas {
		tag, err := tx.Exec(ctx, `update transacoes t set categoria_id = c.id, categorizada_por_ia = true,
				tipo = case when c.tipo = 'transferencia' then 'transferencia'::tipo_transacao else t.tipo end
			from categorias c
			where t.id = $1 and c.id = $2 and t.categoria_id is null
			  and (c.entidade_id = t.entidade_id or c.entidade_id is null)
			  and (c.tipo = 'transferencia' or c.tipo::text = t.tipo::text)
			  and app_pode_editar_entidade(t.entidade_id)`, transacao, categoria)
		if err != nil {
			return n, err
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

// MaxCategorizarPorVez: transações analisadas numa rodada (as que sobram vão na próxima).
const MaxCategorizarPorVez = 400

// RelatorioCategorizacao de uma rodada.
type RelatorioCategorizacao struct {
	Analisadas    int    `json:"analisadas"`
	Categorizadas int    `json:"categorizadas"`
	Uso           ia.Uso `json:"uso"`
	Parou         string `json:"parou,omitempty"` // teto atingido
}

// CategorizarComIA roda por lotes, gravando o consumo a cada lote e parando no teto.
// comUsuario abre a transação com o usuário da sessão (API) ou do worker.
func CategorizarComIA(ctx context.Context, cliente *ia.Cliente, comUsuario func(func(ctx context.Context, tx pgx.Tx) error) error) (RelatorioCategorizacao, error) {
	var rel RelatorioCategorizacao
	var lotes []LotePendente
	if err := comUsuario(func(ctx context.Context, tx pgx.Tx) error {
		var err error
		lotes, err = PendentesIA(ctx, tx, Hoje(), MaxCategorizarPorVez)
		return err
	}); err != nil {
		return rel, err
	}
	for _, l := range lotes {
		if err := comUsuario(func(ctx context.Context, tx pgx.Tx) error {
			return PodeUsarIA(ctx, tx, Hoje())
		}); err != nil {
			rel.Parou = err.Error()
			break
		}
		escolhas, uso, err := cliente.Categorizar(ctx, l.Itens, l.Categorias)
		rel.Uso.Somar(uso)
		rel.Analisadas += len(l.Itens)
		if errGrava := comUsuario(func(ctx context.Context, tx pgx.Tx) error {
			if err := RegistrarUsoIA(ctx, tx, Hoje(), uso); err != nil {
				return err
			}
			n, err := AplicarCategoriasIA(ctx, tx, escolhas)
			rel.Categorizadas += n
			return err
		}); errGrava != nil {
			return rel, errGrava
		}
		if err != nil {
			return rel, err
		}
	}
	return rel, nil
}
