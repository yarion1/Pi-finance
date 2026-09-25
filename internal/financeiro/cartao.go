package financeiro

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// InfoConta é o que a gravação de transações precisa saber da conta.
type InfoConta struct {
	EntidadeID string
	Moeda      string
	Tipo       string
	Fechamento *int16
	Vencimento *int16
}

// EhCartao com dias de fechamento e vencimento: as transações ganham fatura.
func (c InfoConta) EhCartao() bool {
	return c.Tipo == "cartao" && c.Fechamento != nil && c.Vencimento != nil
}

// FaturaEm é o vencimento da fatura em que uma compra nessa data entra (nil fora de cartão).
func (c InfoConta) FaturaEm(data time.Time) *time.Time {
	if !c.EhCartao() {
		return nil
	}
	_, venc := core.FaturaDaCompra(data, int(*c.Fechamento), int(*c.Vencimento))
	return &venc
}

// InfoContaEditavel lê a conta se o usuário pode gravar nela.
func InfoContaEditavel(ctx context.Context, tx pgx.Tx, contaID string) (InfoConta, error) {
	var c InfoConta
	var pode bool
	err := tx.QueryRow(ctx, `select entidade_id, moeda, tipo::text, fechamento, vencimento, app_pode_editar_entidade(entidade_id)
		from contas where id = $1`, contaID).Scan(&c.EntidadeID, &c.Moeda, &c.Tipo, &c.Fechamento, &c.Vencimento, &pode)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !pode) {
		return c, ErrContaNaoEditavel
	}
	return c, err
}

// ContaEditavel devolve a entidade e a moeda da conta se o usuário pode gravar nela.
func ContaEditavel(ctx context.Context, tx pgx.Tx, contaID string) (entidadeID, moeda string, err error) {
	c, err := InfoContaEditavel(ctx, tx, contaID)
	return c.EntidadeID, c.Moeda, err
}

// RecalcularFaturas refaz a fatura de todas as transações da conta (quando mudam os
// dias de fechamento/vencimento ou a conta vira ou deixa de ser cartão).
func RecalcularFaturas(ctx context.Context, tx pgx.Tx, contaID string) error {
	c, err := InfoContaEditavel(ctx, tx, contaID)
	if err != nil {
		return err
	}
	if !c.EhCartao() {
		_, err := tx.Exec(ctx, "update transacoes set fatura_em = null where conta_id = $1 and fatura_em is not null", contaID)
		return err
	}
	linhas, err := tx.Query(ctx, "select id, data from transacoes where conta_id = $1", contaID)
	if err != nil {
		return err
	}
	type par struct {
		id   string
		data time.Time
	}
	var todas []par
	for linhas.Next() {
		var p par
		if err := linhas.Scan(&p.id, &p.data); err != nil {
			linhas.Close()
			return err
		}
		todas = append(todas, p)
	}
	linhas.Close()
	for _, p := range todas {
		if _, err := tx.Exec(ctx, "update transacoes set fatura_em = $2 where id = $1", p.id, c.FaturaEm(p.data)); err != nil {
			return err
		}
	}
	return nil
}
