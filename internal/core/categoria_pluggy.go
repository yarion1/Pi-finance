package core

import "strings"

// Categorias que a Pluggy dá às transações ("Eating out", "Groceries", "Transfer - PIX",
// com um id de 8 dígitos cuja família são os 2 primeiros). CategoriaPluggy traduz para
// as categorias padrão do painel: (mãe, filha). Mãe vazia = sem sugestão (transferências
// para terceiros, empréstimos, "outros"): a pessoa decide, e as regras e o histórico
// aprendem.

type traducao struct {
	chaves []string
	mae    string
	filha  string
}

// Da mais específica para a mais geral (a primeira que casar vale).
var traducoesPluggy = []traducao{
	{[]string{"credit card payment", "pagamento de fatura", "pagamento de cartao"}, "Pagamento de fatura", ""},
	{[]string{"same person transfer", "mesma titularidade"}, "Transferência entre contas", ""},
	{[]string{"proceeds", "dividend", "dividendo", "juros sobre capital"}, "Rendimentos", ""},
	{[]string{"investment", "investimento", "aplicacao", "resgate"}, "Aplicação e resgate", ""},
	{[]string{"salary", "salario"}, "Salário", ""},
	{[]string{"food delivery", "delivery"}, "Alimentação", "Delivery"},
	{[]string{"eating out", "restaurant", "restaurante", "bares"}, "Alimentação", "Restaurante"},
	{[]string{"grocer", "supermercado", "mercearia"}, "Alimentação", "Mercado"},
	{[]string{"taxi", "ride-hailing", "ride hailing", "aplicativo de transporte", "transporte por aplicativo"}, "Transporte", "App"},
	{[]string{"public transport", "transporte publico", "bus ticket"}, "Transporte", "Transporte público"},
	{[]string{"gas station", "posto de combustivel", "combustivel", "fuel"}, "Transporte", "Combustível"},
	{[]string{"parking", "toll", "estacionamento", "pedagio"}, "Transporte", "Estacionamento e pedágio"},
	{[]string{"vehicle maintenance", "manutencao veicular", "manutencao de veiculo"}, "Transporte", "Manutenção do veículo"},
	{[]string{"automotive", "automotivo", "transportation", "transporte", "car rental"}, "Transporte", ""},
	{[]string{"health insurance", "plano de saude"}, "Saúde", "Plano de saúde"},
	{[]string{"pharmac", "farmacia", "drugstore"}, "Saúde", "Farmácia"},
	{[]string{"dentist", "hospital", "clinic", "laborator", "optometr", "medic"}, "Saúde", "Consultas e exames"},
	{[]string{"gym", "fitness", "wellness", "academia", "bem-estar"}, "Saúde", "Academia"},
	{[]string{"healthcare", "saude"}, "Saúde", ""},
	{[]string{"bookstore", "livraria", "books"}, "Educação", "Livros"},
	{[]string{"education", "educacao", "school", "university", "course"}, "Educação", ""},
	{[]string{"video streaming", "music streaming", "streaming"}, "Lazer", "Streaming"},
	{[]string{"gaming", "gambling", "apostas", "jogos"}, "Lazer", "Jogos"},
	{[]string{"airport", "airline", "accommodation", "hospedagem", "mileage", "milhas", "travel", "viagem"}, "Lazer", "Viagem"},
	{[]string{"tickets", "ingresso", "leisure", "lazer"}, "Lazer", "Passeios e eventos"},
	{[]string{"clothing", "vestuario", "roupa"}, "Compras", "Roupas"},
	{[]string{"electronics", "eletronico"}, "Compras", "Eletrônicos"},
	{[]string{"houseware", "utilidades domesticas", "decoracao"}, "Compras", "Casa e decoração"},
	{[]string{" pet "}, "Pessoal", "Pets"},
	{[]string{"shopping", "compras", "sports goods", "kids", "toys", "office supplies"}, "Compras", ""},
	{[]string{"rent", "aluguel"}, "Moradia", "Aluguel"},
	{[]string{"electricity", "energia eletrica"}, "Moradia", "Energia"},
	{[]string{"water", "agua"}, "Moradia", "Água"},
	{[]string{"telecommunication", "internet", "mobile", "telefon"}, "Moradia", "Internet e telefone"},
	{[]string{"housing", "moradia", "utilities"}, "Moradia", ""},
	{[]string{"bank fee", "tarifa"}, "Serviços", "Tarifas bancárias"},
	{[]string{"late payment", "interests charged", "juros", "multa"}, "Serviços", "Juros e multas"},
	{[]string{"iof"}, "Serviços", "IOF"},
	{[]string{"income tax", "imposto de renda"}, "Impostos", "Imposto de renda"},
	{[]string{"property tax", "iptu", "ipva"}, "Impostos", "IPTU e IPVA"},
	{[]string{"tax", "imposto"}, "Impostos", "Outros impostos"},
	{[]string{"donation", "doac"}, "Pessoal", "Doações"},
	{[]string{"digital services", "servicos digitais"}, "Serviços", "Assinaturas"},
	{[]string{"services", "servicos"}, "Serviços", ""},
}

// Família (2 primeiros dígitos do id) quando o nome não casou com nada.
var familiasPluggy = map[string][2]string{
	"01": {"Outras receitas", ""},
	"03": {"Aplicação e resgate", ""},
	"04": {"Transferência entre contas", ""},
	"07": {"Serviços", ""},
	"08": {"Compras", ""},
	"09": {"Lazer", "Streaming"},
	"10": {"Alimentação", "Mercado"},
	"11": {"Alimentação", "Restaurante"},
	"12": {"Lazer", "Viagem"},
	"13": {"Pessoal", "Doações"},
	"15": {"Impostos", "Outros impostos"},
	"16": {"Serviços", "Tarifas bancárias"},
	"17": {"Moradia", ""},
	"18": {"Saúde", ""},
	"19": {"Transporte", ""},
	"21": {"Lazer", ""},
}

// CategoriaPluggy devolve (mãe, filha) do painel para a categoria da Pluggy.
func CategoriaPluggy(id, nome string) (mae, filha string) {
	n := " " + NormalizarDescricao(nome) + " "
	for _, t := range traducoesPluggy {
		for _, c := range t.chaves {
			if strings.Contains(n, c) {
				return t.mae, t.filha
			}
		}
	}
	if len(id) >= 2 {
		if f, ok := familiasPluggy[id[:2]]; ok {
			return f[0], f[1]
		}
	}
	return "", ""
}
