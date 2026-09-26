package pluggy

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
)

// Produtos extras do Open Finance que o Meu Pluggy entrega junto: faturas do cartão,
// empréstimos, identidade e movimentações dos investimentos. Formatos conferidos com os
// tipos do SDK oficial (pluggy-sdk 0.90.0: CreditCardBills, Loan, IdentityResponse,
// InvestmentTransaction). Nem todo banco compartilha todos: quem chama trata
// ErrProdutoIndisponivel como "sem dados", não como falha da sincronização.

// ErrProdutoIndisponivel: a instituição (ou o plano) não oferece esse produto.
var ErrProdutoIndisponivel = errors.New("produto indisponível para este item")

// Opcional converte os erros de "não tem" em ErrProdutoIndisponivel.
func Opcional(err error) error {
	if errors.Is(err, ErrItemNaoEncontrado) || errors.Is(err, ErrRecusada) || errors.Is(err, ErrCredenciais) {
		return ErrProdutoIndisponivel
	}
	return err
}

// paginar busca todas as páginas de um endpoint paginado (page/totalPages).
func paginar[T any](ctx context.Context, c *Cliente, chave, caminho string, q url.Values) ([]T, error) {
	var todos []T
	for pagina := 1; pagina <= 50; pagina++ {
		q.Set("page", strconv.Itoa(pagina))
		var out struct {
			Results    []T `json:"results"`
			TotalPages int `json:"totalPages"`
		}
		if err := c.get(ctx, chave, caminho+"?"+q.Encode(), &out); err != nil {
			return nil, Opcional(err)
		}
		todos = append(todos, out.Results...)
		if pagina >= out.TotalPages || len(out.Results) == 0 {
			break
		}
	}
	return todos, nil
}

// Fatura do cartão como o banco fechou.
type Fatura struct {
	ID              string          `json:"id"`
	DueDate         string          `json:"dueDate"`
	BillClosingDate *string         `json:"billClosingDate"`
	TotalAmount     json.Number     `json:"totalAmount"`
	CurrencyCode    string          `json:"totalAmountCurrencyCode"`
	MinimumPayment  json.Number     `json:"minimumPaymentAmount"`
	FinanceCharges  []EncargoFatura `json:"financeCharges"`
}

// EncargoFatura: juros, multa, IOF... cobrados na fatura.
type EncargoFatura struct {
	Type   string      `json:"type"`
	Amount json.Number `json:"amount"`
}

// Faturas de uma conta de cartão.
func (c *Cliente) Faturas(ctx context.Context, chave, contaID string) ([]Fatura, error) {
	return paginar[Fatura](ctx, c, chave, "/bills", url.Values{"accountId": {contaID}, "pageSize": {"100"}})
}

// Emprestimo (empréstimo, financiamento, cheque especial contratado...).
type Emprestimo struct {
	ID                      string                `json:"id"`
	ProductName             string                `json:"productName"`
	Type                    *string               `json:"type"`
	Kind                    string                `json:"kind"` // LOAN, FINANCING, INVOICE_FINANCING, UNARRANGED_ACCOUNT_OVERDRAFT
	ContractAmount          json.Number           `json:"contractAmount"`
	CurrencyCode            string                `json:"currencyCode"`
	ContractDate            *string               `json:"contractDate"`
	DueDate                 *string               `json:"dueDate"`
	FirstInstallmentDueDate *string               `json:"firstInstallmentDueDate"`
	CET                     json.Number           `json:"CET"`
	AmortizationScheduled   *string               `json:"amortizationScheduled"`
	InterestRates           []TaxaJuros           `json:"interestRates"`
	Installments            *ParcelasEmprestimo   `json:"installments"`
	Payments                *PagamentosEmprestimo `json:"payments"`
}

// TaxaJuros do contrato.
type TaxaJuros struct {
	TaxPeriodicity *string     `json:"taxPeriodicity"` // MONTHLY, YEARLY
	PreFixedRate   json.Number `json:"preFixedRate"`
}

// ParcelasEmprestimo: contagem das parcelas.
type ParcelasEmprestimo struct {
	TotalNumberOfInstallments *int `json:"totalNumberOfInstallments"`
	ContractRemainingNumber   *int `json:"contractRemainingNumber"`
	PaidInstallments          *int `json:"paidInstallments"`
	DueInstallments           *int `json:"dueInstallments"`
	PastDueInstallments       *int `json:"pastDueInstallments"`
}

// PagamentosEmprestimo: saldo devedor.
type PagamentosEmprestimo struct {
	ContractOutstandingBalance json.Number `json:"contractOutstandingBalance"`
}

// Emprestimos de um item.
func (c *Cliente) Emprestimos(ctx context.Context, chave, itemID string) ([]Emprestimo, error) {
	return paginar[Emprestimo](ctx, c, chave, "/loans", url.Values{"itemId": {itemID}, "pageSize": {"100"}})
}

// Identidade do titular do item.
type Identidade struct {
	FullName     *string `json:"fullName"`
	Document     *string `json:"document"`
	DocumentType *string `json:"documentType"` // CPF, CNPJ
	TaxNumber    *string `json:"taxNumber"`
	BirthDate    *string `json:"birthDate"`
}

// Identidade do item.
func (c *Cliente) Identidade(ctx context.Context, chave, itemID string) (Identidade, error) {
	var id Identidade
	err := c.get(ctx, chave, "/identity?"+url.Values{"itemId": {itemID}}.Encode(), &id)
	return id, Opcional(err)
}

// MovimentoInvestimento: aporte, resgate, imposto ou transferência.
type MovimentoInvestimento struct {
	ID           string      `json:"id"`
	Type         *string     `json:"type"` // BUY, SELL, TAX, TRANSFER
	Description  *string     `json:"description"`
	Quantity     json.Number `json:"quantity"`
	Value        json.Number `json:"value"`
	Amount       json.Number `json:"amount"`
	NetAmount    json.Number `json:"netAmount"`
	Date         string      `json:"date"`
	MovementType string      `json:"movementType"` // CREDIT (entra no investimento), DEBIT (sai)
}

// MovimentosInvestimento de um investimento.
func (c *Cliente) MovimentosInvestimento(ctx context.Context, chave, investimentoID string) ([]MovimentoInvestimento, error) {
	return paginar[MovimentoInvestimento](ctx, c, chave, "/investments/"+url.PathEscape(investimentoID)+"/transactions",
		url.Values{"pageSize": {"500"}})
}
