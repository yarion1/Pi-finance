package financeiro

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/importadores"
)

// HoleriteSalvo na tabela holerites.
type HoleriteSalvo struct {
	ID         string `json:"id"`
	EntidadeID string `json:"entidade_id"`
	importadores.Holerite
}

// SalvarHolerite (substitui o do mesmo mês e empregador).
func SalvarHolerite(ctx context.Context, tx pgx.Tx, entidadeID string, h importadores.Holerite) (string, error) {
	if err := PodeEditarEntidade(ctx, tx, entidadeID); err != nil {
		return "", err
	}
	comp, err := time.Parse("2006-01", h.Competencia)
	if err != nil {
		return "", ErrCampo{"competência no formato AAAA-MM"}
	}
	var id string
	err = tx.QueryRow(ctx, `insert into holerites (entidade_id, competencia, empregador, bruto_centavos, inss_centavos,
			irrf_centavos, outros_descontos_centavos, liquido_centavos)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (entidade_id, competencia, empregador) do update set bruto_centavos = excluded.bruto_centavos,
			inss_centavos = excluded.inss_centavos, irrf_centavos = excluded.irrf_centavos,
			outros_descontos_centavos = excluded.outros_descontos_centavos, liquido_centavos = excluded.liquido_centavos
		returning id`, entidadeID, comp, h.Empregador, int64(h.Bruto), int64(h.INSS), int64(h.IRRF), int64(h.Outros),
		int64(h.Liquido)).Scan(&id)
	return id, err
}

// HoleritesDoAno com os totais (rendimentos tributáveis e retenções para o IR).
type HoleritesDoAno struct {
	Ano     int             `json:"ano"`
	Itens   []HoleriteSalvo `json:"itens"`
	Bruto   int64           `json:"bruto_centavos"`
	INSS    int64           `json:"inss_centavos"`
	IRRF    int64           `json:"irrf_centavos"`
	Liquido int64           `json:"liquido_centavos"`
}

// ListarHolerites do ano nas entidades.
func ListarHolerites(ctx context.Context, tx pgx.Tx, ents []string, ano int) (HoleritesDoAno, error) {
	out := HoleritesDoAno{Ano: ano, Itens: []HoleriteSalvo{}}
	linhas, err := tx.Query(ctx, `select id, entidade_id, to_char(competencia, 'YYYY-MM'), empregador, bruto_centavos,
			inss_centavos, irrf_centavos, outros_descontos_centavos, liquido_centavos
		from holerites where entidade_id = any($1) and extract(year from competencia) = $2 order by competencia, empregador`, ents, ano)
	if err != nil {
		return out, err
	}
	defer linhas.Close()
	for linhas.Next() {
		var h HoleriteSalvo
		if err := linhas.Scan(&h.ID, &h.EntidadeID, &h.Competencia, &h.Empregador, &h.Bruto, &h.INSS, &h.IRRF, &h.Outros,
			&h.Liquido); err != nil {
			return out, err
		}
		out.Itens = append(out.Itens, h)
		out.Bruto += int64(h.Bruto)
		out.INSS += int64(h.INSS)
		out.IRRF += int64(h.IRRF)
		out.Liquido += int64(h.Liquido)
	}
	return out, linhas.Err()
}
