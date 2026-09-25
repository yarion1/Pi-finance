// Package financeiro tem as regras de contas e transações que tocam o banco:
// importação (deduplicar, categorizar, parear transferências), desfazer e
// categorização. Tudo roda dentro de uma transação aberta por db.ComUsuario,
// então o RLS vale em cada consulta.
package financeiro

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

type regra struct {
	texto      string // normalizado
	contaID    *string
	categoria  string
	tipo       string
	prioridade int
}

type sugestao struct {
	categoria string
	tipo      string
}

// Categorizador escolhe a categoria de uma transação: regras do usuário primeiro,
// depois o histórico de descrições parecidas da mesma entidade (docs/SPEC.md §6;
// o modelo de IA entra na fase 6).
type Categorizador struct {
	regras    []regra
	historico map[string]sugestao
}

// NovoCategorizador carrega regras e histórico da entidade.
func NovoCategorizador(ctx context.Context, tx pgx.Tx, entidadeID string) (*Categorizador, error) {
	c := &Categorizador{historico: map[string]sugestao{}}
	linhas, err := tx.Query(ctx, `select r.texto, r.conta_id, r.categoria_id, c.tipo::text, r.prioridade
		from regras_categoria r join categorias c on c.id = r.categoria_id
		where r.entidade_id = $1 order by r.prioridade desc, length(r.texto) desc, r.criada_em`, entidadeID)
	if err != nil {
		return nil, err
	}
	for linhas.Next() {
		var r regra
		if err := linhas.Scan(&r.texto, &r.contaID, &r.categoria, &r.tipo, &r.prioridade); err != nil {
			linhas.Close()
			return nil, err
		}
		r.texto = core.NormalizarDescricao(r.texto)
		c.regras = append(c.regras, r)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, err
	}

	// o mais recente de cada descrição vence (a última correção do usuário)
	linhas, err = tx.Query(ctx, `select t.descricao, t.categoria_id, c.tipo::text from transacoes t
		join categorias c on c.id = t.categoria_id
		where t.entidade_id = $1 order by t.data desc, t.criada_em desc limit 5000`, entidadeID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	for linhas.Next() {
		var desc, cat, tipo string
		if err := linhas.Scan(&desc, &cat, &tipo); err != nil {
			return nil, err
		}
		chave := core.ChaveHistorico(desc)
		if _, existe := c.historico[chave]; !existe && chave != "" {
			c.historico[chave] = sugestao{cat, tipo}
		}
	}
	return c, linhas.Err()
}

// compativel: categoria de gasto não vai em entrada de dinheiro e vice-versa.
func compativel(tipoCategoria string, valor core.Centavos) bool {
	switch tipoCategoria {
	case "gasto":
		return valor < 0
	case "receita":
		return valor > 0
	default:
		return true
	}
}

// Sugerir devolve a categoria e de onde veio ("regra", "historico") ou nil.
func (c *Categorizador) Sugerir(contaID, descricao string, valor core.Centavos) (*string, string) {
	norm := core.NormalizarDescricao(descricao)
	for _, r := range c.regras {
		if r.contaID != nil && *r.contaID != contaID {
			continue
		}
		if strings.Contains(norm, r.texto) && compativel(r.tipo, valor) {
			cat := r.categoria
			return &cat, "regra"
		}
	}
	if s, ok := c.historico[core.ChaveHistorico(descricao)]; ok && compativel(s.tipo, valor) {
		cat := s.categoria
		return &cat, "historico"
	}
	return nil, ""
}

// Aprender registra uma correção feita agora, para as próximas linhas do mesmo lote.
func (c *Categorizador) Aprender(descricao, categoria, tipo string) {
	if chave := core.ChaveHistorico(descricao); chave != "" {
		c.historico[chave] = sugestao{categoria, tipo}
	}
}

// TipoPorValor: saída é gasto, entrada é receita.
func TipoPorValor(v core.Centavos) string {
	if v < 0 {
		return "gasto"
	}
	return "receita"
}
