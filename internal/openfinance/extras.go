package openfinance

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/db"
	"github.com/yarion1/pi-finance/internal/pluggy"
)

// contextoDocumento: o mesmo contexto de cifra da coluna entidades.documento_cifrado.
const contextoDocumento = "entidades.documento"

// sincronizarExtras: faturas dos cartões vinculados, empréstimos, identidade e
// movimentações dos investimentos. Produto que o banco não oferece é pulado; outro
// erro vai para o relatório sem parar a sincronização das transações.
func (s *Servico) sincronizarExtras(ctx context.Context, usuarioID, chave string, it itemLocal, contas []contaLocal, invs []pluggy.Investimento, rel *Relatorio) {
	registrar := func(produto string, err error) {
		if err == nil || errors.Is(err, pluggy.ErrProdutoIndisponivel) {
			return
		}
		slog.Warn("open finance: extra falhou", "produto", produto, "erro", err)
		rel.Erros = append(rel.Erros, produto+": "+err.Error())
	}
	for _, c := range contas {
		if c.contaID != nil && !c.ignorada && c.pluggy.Cartao() {
			registrar("faturas", s.sincronizarFaturas(ctx, usuarioID, chave, *c.contaID, c.pluggy.ID))
		}
	}
	registrar("empréstimos", s.sincronizarEmprestimos(ctx, usuarioID, chave, it))
	registrar("identidade", s.sincronizarIdentidade(ctx, usuarioID, chave, it))
	registrar("movimentações de investimentos", s.sincronizarMovimentos(ctx, usuarioID, chave, it, invs))
}

func (s *Servico) sincronizarFaturas(ctx context.Context, usuarioID, chave, contaID, contaPluggy string) error {
	faturas, err := s.Pluggy.Faturas(ctx, chave, contaPluggy)
	if err != nil {
		return err
	}
	limite := s.hoje().AddDate(-1, -1, 0) // só o último ano
	return db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		for _, f := range faturas {
			venc, err := pluggy.DataSP(f.DueDate)
			if err != nil || venc.Before(limite) {
				continue
			}
			total, err := pluggy.Centavos(f.TotalAmount)
			if err != nil {
				continue
			}
			var fech *time.Time
			if f.BillClosingDate != nil {
				if d, err := pluggy.DataSP(*f.BillClosingDate); err == nil {
					fech = &d
				}
			}
			var minimo *core.Centavos
			if v, err := pluggy.Centavos(f.MinimumPayment); err == nil && f.MinimumPayment != "" {
				minimo = &v
			}
			var encargos core.Centavos
			for _, e := range f.FinanceCharges {
				if v, err := pluggy.Centavos(e.Amount); err == nil {
					encargos += v
				}
			}
			moeda := strings.ToUpper(f.CurrencyCode)
			if len(moeda) != 3 {
				moeda = "BRL"
			}
			if _, err := tx.Exec(ctx, `insert into faturas_banco (entidade_id, conta_id, pluggy_id, vencimento, fechamento,
					total_centavos, minimo_centavos, encargos_centavos, moeda)
				select c.entidade_id, c.id, $2, $3, $4, $5, $6, $7, $8 from contas c where c.id = $1
				on conflict (conta_id, pluggy_id) do update set vencimento = excluded.vencimento, fechamento = excluded.fechamento,
					total_centavos = excluded.total_centavos, minimo_centavos = excluded.minimo_centavos,
					encargos_centavos = excluded.encargos_centavos, moeda = excluded.moeda, atualizada_em = now()`,
				contaID, f.ID, venc, fech, int64(total), minimo, int64(encargos), moeda); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Servico) sincronizarEmprestimos(ctx context.Context, usuarioID, chave string, it itemLocal) error {
	lista, err := s.Pluggy.Emprestimos(ctx, chave, it.itemID)
	if err != nil {
		return err
	}
	return db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		vistos := []string{}
		for _, e := range lista {
			vistos = append(vistos, e.ID)
			valor := centavosOpcional(e.ContractAmount)
			var saldo *int64
			if e.Payments != nil {
				saldo = centavosOpcional(e.Payments.ContractOutstandingBalance)
			}
			var total, pagas, restantes, atrasadas *int
			if p := e.Installments; p != nil {
				total, pagas, restantes, atrasadas = p.TotalNumberOfInstallments, p.PaidInstallments, p.DueInstallments, p.PastDueInstallments
				if restantes == nil {
					restantes = p.ContractRemainingNumber
				}
			}
			var taxa *string
			var periodicidade *string
			for _, r := range e.InterestRates {
				if r.PreFixedRate != "" {
					v := r.PreFixedRate.String()
					taxa, periodicidade = &v, r.TaxPeriodicity
					break
				}
			}
			var cet *string
			if e.CET != "" {
				v := e.CET.String()
				cet = &v
			}
			nome := strings.TrimSpace(e.ProductName)
			if nome == "" {
				nome = "Empréstimo"
			}
			moeda := strings.ToUpper(e.CurrencyCode)
			if len(moeda) != 3 {
				moeda = "BRL"
			}
			if _, err := tx.Exec(ctx, `insert into emprestimos_banco (entidade_id, item_id, pluggy_id, nome, modalidade,
					valor_contratado_centavos, saldo_devedor_centavos, moeda, parcelas_total, parcelas_pagas, parcelas_restantes,
					parcelas_atrasadas, taxa, taxa_periodicidade, cet, sistema, contratado_em, vencimento_final)
				values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::numeric, $14, $15::numeric, $16, $17, $18)
				on conflict (entidade_id, pluggy_id) do update set nome = excluded.nome, modalidade = excluded.modalidade,
					valor_contratado_centavos = excluded.valor_contratado_centavos, saldo_devedor_centavos = excluded.saldo_devedor_centavos,
					moeda = excluded.moeda, parcelas_total = excluded.parcelas_total, parcelas_pagas = excluded.parcelas_pagas,
					parcelas_restantes = excluded.parcelas_restantes, parcelas_atrasadas = excluded.parcelas_atrasadas,
					taxa = excluded.taxa, taxa_periodicidade = excluded.taxa_periodicidade, cet = excluded.cet,
					sistema = excluded.sistema, contratado_em = excluded.contratado_em, vencimento_final = excluded.vencimento_final,
					atualizado_em = now()`,
				it.entidade, it.id, e.ID, nome, strings.ToUpper(e.Kind), valor, saldo, moeda, total, pagas, restantes, atrasadas,
				taxa, periodicidade, cet, e.AmortizationScheduled, dataOpcional(e.ContractDate), dataOpcional(e.DueDate)); err != nil {
				return err
			}
		}
		// quitados somem da Pluggy: somem daqui também
		_, err := tx.Exec(ctx, `delete from emprestimos_banco where item_id = $1 and not (pluggy_id = any($2))`, it.id, vistos)
		return err
	})
}

// sincronizarIdentidade preenche o CPF da pessoa física do item quando está vazio (cifrado).
func (s *Servico) sincronizarIdentidade(ctx context.Context, usuarioID, chave string, it itemLocal) error {
	var tipo string
	var doc *string
	if err := db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select tipo::text, documento_cifrado from entidades where id = $1", it.entidade).Scan(&tipo, &doc)
	}); err != nil {
		return err
	}
	if tipo != "PF" || doc != nil || s.Cifrador == nil {
		return nil // só completa, nunca troca
	}
	id, err := s.Pluggy.Identidade(ctx, chave, it.itemID)
	if err != nil {
		return err
	}
	numero := ""
	for _, v := range []*string{id.Document, id.TaxNumber} {
		if v != nil && core.CPFValido(core.SoDigitos(*v)) {
			numero = core.SoDigitos(*v)
			break
		}
	}
	if numero == "" {
		return nil
	}
	cifrado, err := s.Cifrador.Cifrar(numero, contextoDocumento)
	if err != nil {
		return err
	}
	return db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update entidades set documento_cifrado = $2 where id = $1 and documento_cifrado is null
			and app_pode_editar_entidade(id)`, it.entidade, cifrado)
		return err
	})
}

// sincronizarMovimentos grava aportes e resgates dos investimentos do banco como
// operações do ativo (para a rentabilidade); impostos ficam de fora.
func (s *Servico) sincronizarMovimentos(ctx context.Context, usuarioID, chave string, it itemLocal, invs []pluggy.Investimento) error {
	for _, inv := range invs {
		movs, err := s.Pluggy.MovimentosInvestimento(ctx, chave, inv.ID)
		if errors.Is(err, pluggy.ErrProdutoIndisponivel) {
			continue
		}
		if err != nil {
			return err
		}
		if len(movs) == 0 {
			continue
		}
		err = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
			var ativo string
			if err := tx.QueryRow(ctx, "select id from ativos where entidade_id = $1 and pluggy_id = $2", it.entidade, inv.ID).
				Scan(&ativo); err != nil {
				return err
			}
			for _, m := range movs {
				op, ok := OperacaoDoMovimento(m)
				if !ok {
					continue
				}
				if _, err := tx.Exec(ctx, `insert into operacoes (entidade_id, ativo_id, data, tipo, quantidade, preco, descricao,
						origem, chave_dedup)
					values ($1, $2, $3, $4::tipo_operacao, $5::numeric, $6::numeric, $7, 'pluggy', $8)
					on conflict (entidade_id, chave_dedup) do nothing`,
					it.entidade, ativo, op.Data, op.Tipo, op.Quantidade.String(), op.Preco.String(), op.Descricao, "pluggy:"+m.ID); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// OperacaoPluggy: movimento do banco convertido em operação.
type OperacaoPluggy struct {
	Data       time.Time
	Tipo       string // compra, venda
	Quantidade core.Dec8
	Preco      core.Dec8
	Descricao  *string
}

// OperacaoDoMovimento: CREDIT (entra dinheiro no investimento) é compra; DEBIT é
// resgate (venda), pelo valor líquido quando o banco informa; TAX fica de fora. Sem
// quantidade (renda fixa, fundos), vira 1 × valor.
func OperacaoDoMovimento(m pluggy.MovimentoInvestimento) (OperacaoPluggy, bool) {
	var op OperacaoPluggy
	if m.Type != nil && strings.EqualFold(*m.Type, "TAX") {
		return op, false
	}
	data, err := pluggy.DataSP(m.Date)
	if err != nil {
		return op, false
	}
	op.Data, op.Descricao = data, m.Description
	valor, err := pluggy.Centavos(m.Amount)
	switch strings.ToUpper(m.MovementType) {
	case "CREDIT":
		op.Tipo = "compra"
	case "DEBIT":
		op.Tipo = "venda"
		if v, e := pluggy.Centavos(m.NetAmount); e == nil && v != 0 {
			valor, err = v, nil
		}
	default:
		return op, false
	}
	if valor < 0 { // o sentido vem de movementType; o sinal do valor varia por banco
		valor = -valor
	}
	if err != nil || valor <= 0 {
		return op, false
	}
	op.Quantidade = core.Um
	if q, err := core.ParseDec8(m.Quantity.String()); err == nil && q > 0 {
		op.Quantidade = q
	}
	op.Preco = core.PrecoMedio(valor, op.Quantidade)
	return op, true
}

func centavosOpcional(n interface{ String() string }) *int64 {
	s := n.String()
	if s == "" {
		return nil
	}
	v, err := core.ParseDecimalPonto(strings.TrimPrefix(s, "+"))
	if err != nil {
		return nil
	}
	x := int64(v)
	return &x
}

func dataOpcional(s *string) *time.Time {
	if s == nil {
		return nil
	}
	d, err := pluggy.DataSP(*s)
	if err != nil {
		return nil
	}
	return &d
}
