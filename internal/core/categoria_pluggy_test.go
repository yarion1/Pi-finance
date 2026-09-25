package core

import "testing"

func TestCategoriaPluggy(t *testing.T) {
	casos := []struct{ id, nome, mae, filha string }{
		{"11010000", "Eating out", "Alimentação", "Restaurante"},
		{"11020000", "Food delivery", "Alimentação", "Delivery"},
		{"10000000", "Groceries", "Alimentação", "Mercado"},
		{"19010000", "Taxi and ride-hailing", "Transporte", "App"},
		{"19050000", "Automotive", "Transporte", ""},
		{"19050300", "Vehicle maintenance", "Transporte", "Manutenção do veículo"},
		{"19050100", "Gas stations", "Transporte", "Combustível"},
		{"08010000", "Online shopping", "Compras", ""},
		{"08040000", "Clothing", "Compras", "Roupas"},
		{"08020000", "Electronics", "Compras", "Eletrônicos"},
		{"08030000", "Pet supplies and vet", "Pessoal", "Pets"},
		{"17030000", "Houseware", "Compras", "Casa e decoração"},
		{"12010000", "Airport and airlines", "Lazer", "Viagem"},
		{"18020000", "Pharmacy", "Saúde", "Farmácia"},
		{"16000000", "Bank fees", "Serviços", "Tarifas bancárias"},
		{"05100000", "Credit card payment", "Pagamento de fatura", ""},
		{"04000000", "Same person transfer", "Transferência entre contas", ""},
		{"01010000", "Salary", "Salário", ""},
		{"03060000", "Proceeds interests and dividends", "Rendimentos", ""},
		{"09020000", "Video streaming", "Lazer", "Streaming"},
		{"19020000", "Public transportation", "Transporte", "Transporte público"},
		{"17010000", "Rent", "Moradia", "Aluguel"},
		{"15000000", "Taxes", "Impostos", "Outros impostos"},
		// o nome manda; sem nome conhecido, a família; transferência a terceiros não tem sugestão
		{"18990000", "", "Saúde", ""},
		{"05070000", "Transfer - PIX", "", ""},
		{"99000000", "Other", "", ""},
		{"", "", "", ""},
		{"x", "Transferência PIX", "", ""},
		// "petrobras" não é pet; "car rental" não é aluguel
		{"", "Petrobras", "", ""},
		{"19040000", "Car rental", "Transporte", ""},
	}
	for _, c := range casos {
		mae, filha := CategoriaPluggy(c.id, c.nome)
		if mae != c.mae || filha != c.filha {
			t.Errorf("%s %q → (%q, %q), esperado (%q, %q)", c.id, c.nome, mae, filha, c.mae, c.filha)
		}
	}
}
