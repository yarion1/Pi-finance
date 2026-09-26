package financeiro

import (
	"context"
	"errors"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// FluxoProjetado: saldo das contas do dia a dia nos próximos dias, com faixa de 80 %.
type FluxoProjetado struct {
	SaldoInicial   core.Centavos   `json:"saldo_inicial_centavos"`
	Pontos         []core.FluxoDia `json:"pontos"`
	Final          core.FluxoDia   `json:"final"`
	Menor          core.FluxoDia   `json:"menor"`       // pelo cenário base
	MenorBaixo     core.FluxoDia   `json:"menor_baixo"` // pelo cenário ruim da faixa
	VariavelMensal core.Centavos   `json:"variavel_mensal_centavos"`
	Conhecidos     int             `json:"itens_conhecidos"`
}

// MaxDiasFluxo: horizonte máximo da projeção.
const MaxDiasFluxo = 366

// gastosVariaveis: gasto por mês (últimos 13 meses fechados) sem parcelas e sem o que já
// é recorrência conhecida — o que a agenda não prevê.
func gastosVariaveis(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time) (map[string]core.Centavos, error) {
	fixas := map[string]bool{}
	linhas, err := tx.Query(ctx, "select chave from recorrencias where entidade_id = any($1) and ativa", ents)
	if err != nil {
		return nil, err
	}
	chaves, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, c := range chaves {
		fixas[c] = true
	}
	inicio := time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC)
	linhas, err = tx.Query(ctx, `select t.data, t.descricao, t.valor_centavos from transacoes t
		where t.entidade_id = any($1) and t.tipo = 'gasto' and t.parcela_n is null and t.data >= $2 and t.data < $3`,
		ents, inicio.AddDate(0, -13, 0), inicio)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	out := map[string]core.Centavos{}
	for linhas.Next() {
		var d time.Time
		var desc string
		var v int64
		if err := linhas.Scan(&d, &desc, &v); err != nil {
			return nil, err
		}
		if fixas[core.ChaveHistorico(desc)] {
			continue
		}
		out[d.Format("2006-01")] += core.Centavos(-v)
	}
	return out, linhas.Err()
}

// ProjetarFluxo: agenda (recorrências, faturas, parcelas, dívidas, compromissos) + gasto
// variável sazonal; extras entram como itens a mais (simulação de compra).
func ProjetarFluxo(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, dias int, extras map[string]core.Centavos) (FluxoProjetado, error) {
	var f FluxoProjetado
	if dias < 1 || dias > MaxDiasFluxo {
		return f, ErrCampo{"projeção de 1 a 366 dias"}
	}
	ag, err := CalcularAgenda(ctx, tx, ents, hoje, dias)
	if err != nil {
		return f, err
	}
	conhecidos := map[string]core.Centavos{}
	for _, it := range ag.Itens {
		if !it.Pago && !it.NoCartao {
			conhecidos[it.Data] += core.Centavos(it.Valor)
			f.Conhecidos++
		}
	}
	for d, v := range extras {
		conhecidos[d] += v
	}
	variaveis, err := gastosVariaveis(ctx, tx, ents, hoje)
	if err != nil {
		return f, err
	}
	f.SaldoInicial = core.Centavos(ag.SaldoInicial)
	f.VariavelMensal = core.VariavelDoMes(variaveis, hoje, hoje)
	f.Pontos = core.ProjetarFluxo(core.EntradaFluxo{Hoje: dia(hoje), Dias: dias, SaldoInicial: f.SaldoInicial,
		Conhecidos: conhecidos, Variaveis: variaveis})
	f.Final, f.Menor, f.MenorBaixo = f.Pontos[len(f.Pontos)-1], f.Pontos[0], f.Pontos[0]
	for _, p := range f.Pontos {
		if p.Base < f.Menor.Base {
			f.Menor = p
		}
		if p.Baixo < f.MenorBaixo.Baixo {
			f.MenorBaixo = p
		}
	}
	return f, nil
}

// SimulacaoCompra: o fluxo com e sem a compra parcelada.
type SimulacaoCompra struct {
	Parcela      core.Centavos  `json:"parcela_centavos"`
	Sem          FluxoProjetado `json:"sem"`
	Com          FluxoProjetado `json:"com"`
	Situacao     string         `json:"situacao"` // folgada, apertada, nao_cabe
	PrimeiraData string         `json:"primeira_data"`
	UltimaData   string         `json:"ultima_data"`
}

// SimularCompra: valor em n parcelas mensais a partir de `primeira`. "Não cabe" quando o
// saldo base fica negativo; "apertada" quando só o cenário ruim da faixa fica.
func SimularCompra(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, valor core.Centavos, parcelas int, primeira time.Time) (SimulacaoCompra, error) {
	var s SimulacaoCompra
	if valor <= 0 || valor > 100_000_000_00 || parcelas < 1 || parcelas > 24 {
		return s, ErrCampo{"valor positivo e de 1 a 24 parcelas"}
	}
	if primeira.Before(dia(hoje)) || primeira.After(dia(hoje).AddDate(0, 3, 0)) {
		return s, ErrCampo{"a primeira parcela vence de hoje a 3 meses"}
	}
	extras := map[string]core.Centavos{}
	resto := valor
	for k := range parcelas {
		p := valor / core.Centavos(parcelas)
		if k == parcelas-1 {
			p = resto // a última leva o arredondamento
		}
		resto -= p
		d := core.SomarMeses(primeira, k)
		extras[d.Format(formatoData)] -= p
		if k == 0 {
			s.PrimeiraData, s.Parcela = d.Format(formatoData), p
		}
		s.UltimaData = d.Format(formatoData)
	}
	ultima, _ := time.Parse(formatoData, s.UltimaData)
	dias := max(90, int(ultima.Sub(dia(hoje)).Hours()/24)+15)
	dias = min(dias, MaxDiasFluxo)
	var err error
	if s.Sem, err = ProjetarFluxo(ctx, tx, ents, hoje, dias, nil); err != nil {
		return s, err
	}
	if s.Com, err = ProjetarFluxo(ctx, tx, ents, hoje, dias, extras); err != nil {
		return s, err
	}
	s.Sem.Pontos, s.Com.Pontos = nil, nil // a tela mostra os resumos; a curva vem de /api/projecoes/fluxo
	switch {
	case s.Com.Menor.Base < 0:
		s.Situacao = "nao_cabe"
	case s.Com.MenorBaixo.Baixo < 0:
		s.Situacao = "apertada"
	default:
		s.Situacao = "folgada"
	}
	return s, nil
}

// CDIMensal: o CDI do último dia conhecido, levado a um mês (21 dias úteis). Sem dado,
// 0,8 % ao mês.
func CDIMensal(ctx context.Context, tx pgx.Tx) (float64, error) {
	var diario float64
	err := tx.QueryRow(ctx, "select valor::float8 from indices where serie = 'cdi' order by data desc limit 1").Scan(&diario)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0.008, nil
	}
	if err != nil {
		return 0, err
	}
	return math.Round((math.Pow(1+diario, 21)-1)*1e6) / 1e6, nil
}

// Perfis para onde vão os aportes futuros na simulação de patrimônio ("atual": como a
// carteira está hoje).
var perfisAporte = map[string]map[string]float64{
	"conservador": {"renda_fixa": 0.7, "tesouro": 0.2, "acao": 0.1},
	"moderado":    {"renda_fixa": 0.4, "tesouro": 0.2, "acao": 0.2, "fii": 0.1, "exterior": 0.1},
	"arrojado":    {"renda_fixa": 0.2, "acao": 0.4, "fii": 0.15, "exterior": 0.2, "cripto": 0.05},
}

// EntradaFuturo da simulação de patrimônio.
type EntradaFuturo struct {
	Aporte       core.Centavos
	Anos         int
	Custo        core.Centavos // custo de vida mensal desejado (0: o médio dos últimos 6 meses)
	TaxaRetirada float64       // 0: 4 %
	Perfil       string        // atual, conservador, moderado, arrojado
	Premissas    map[string]core.PremissaClasse
}

// Futuro: Monte Carlo do que é investível (carteira + contas) e a independência financeira.
type Futuro struct {
	Classes      []core.ClasseMC `json:"classes"`
	Investivel   core.Centavos   `json:"investivel_centavos"`
	Aporte       core.Centavos   `json:"aporte_mensal_centavos"`
	Custo        core.Centavos   `json:"custo_mensal_centavos"`
	TaxaRetirada float64         `json:"taxa_retirada"`
	Alvo         core.Centavos   `json:"alvo_centavos"`
	RetornoMedio float64         `json:"retorno_medio_aa"`
	Pontos       []core.PontoMC  `json:"pontos"`
	Cenarios     []CenarioIF     `json:"cenarios"`
	Simulacoes   int             `json:"simulacoes"`
}

// CenarioIF: em quantos meses o patrimônio chega ao alvo com um retorno fixo.
type CenarioIF struct {
	Nome    string  `json:"nome"` // pessimista, base, otimista
	Retorno float64 `json:"retorno_aa"`
	Meses   int     `json:"meses"` // -1: não chega em 100 anos
}

// CenariosMC: quantidade de cenários do Monte Carlo (SPEC: 5.000).
const CenariosMC = 5000

// SimularFuturo monta as classes pela carteira e pelas contas e roda o Monte Carlo.
func SimularFuturo(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, e EntradaFuturo) (Futuro, error) {
	var f Futuro
	if e.Anos < 1 || e.Anos > 50 || e.Aporte < 0 || e.Aporte > 10_000_000_00 || e.Custo < 0 || e.TaxaRetirada < 0 || e.TaxaRetirada > 0.2 {
		return f, ErrCampo{"de 1 a 50 anos, aporte e custo positivos, retirada até 20 %"}
	}
	if e.Perfil == "" {
		e.Perfil = "atual"
	}
	if _, ok := perfisAporte[e.Perfil]; !ok && e.Perfil != "atual" {
		return f, ErrCampo{"perfil: atual, conservador, moderado ou arrojado"}
	}
	carteira, err := CalcularCarteira(ctx, tx, ents, hoje)
	if err != nil {
		return f, err
	}
	var caixa int64
	if err := tx.QueryRow(ctx, `select coalesce(sum(greatest(app_saldo_conta(id, $2), 0)), 0) from contas
		where entidade_id = any($1) and not arquivada and moeda = 'BRL'
		  and tipo in ('corrente', 'poupanca', 'carteira', 'dinheiro', 'beneficio')`, ents, hoje).Scan(&caixa); err != nil {
		return f, err
	}
	valores := map[string]core.Centavos{"caixa": core.Centavos(caixa)}
	for _, c := range carteira.Classes {
		valores[c.Classe] += c.Valor
	}
	pesos := perfisAporte[e.Perfil]
	if e.Perfil == "atual" {
		pesos = map[string]float64{}
		for c, v := range valores {
			pesos[c] = float64(v)
		}
	}
	for c := range pesos {
		if _, ok := valores[c]; !ok {
			valores[c] = 0
		}
	}
	var somaPeso, somaRetorno float64
	for _, c := range slices.Sorted(maps.Keys(valores)) {
		p, ok := e.Premissas[c]
		if !ok {
			p = core.PremissasPadrao[c]
			if _, existe := core.PremissasPadrao[c]; !existe {
				p = core.PremissasPadrao["outro"]
			}
		}
		if valores[c] == 0 && pesos[c] == 0 {
			continue
		}
		f.Classes = append(f.Classes, core.ClasseMC{Classe: c, Valor: valores[c], PesoAporte: pesos[c], PremissaClasse: p})
		f.Investivel += valores[c]
		peso := float64(valores[c]) + pesos[c]*float64(e.Aporte)*12
		somaPeso += peso
		somaRetorno += peso * p.Retorno
	}
	if somaPeso > 0 {
		f.RetornoMedio = math.Round(somaRetorno/somaPeso*1e4) / 1e4
	}
	f.Aporte, f.Custo, f.TaxaRetirada = e.Aporte, e.Custo, e.TaxaRetirada
	if f.TaxaRetirada == 0 {
		f.TaxaRetirada = 0.04
	}
	if f.Custo == 0 {
		cv, err := CalcularCustoDeVida(ctx, tx, ents, hoje)
		if err != nil {
			return f, err
		}
		f.Custo = core.Centavos(cv.Meses6)
	}
	f.Alvo = core.AlvoIndependencia(f.Custo, f.TaxaRetirada)
	f.Simulacoes = CenariosMC
	f.Pontos = core.MonteCarlo(core.EntradaMC{Classes: f.Classes, AporteMensal: f.Aporte, Anos: e.Anos,
		Cenarios: CenariosMC, Alvo: f.Alvo, Semente: 2026})
	for _, c := range []struct {
		nome  string
		delta float64
	}{{"pessimista", -0.02}, {"base", 0}, {"otimista", 0.02}} {
		r := f.RetornoMedio + c.delta
		f.Cenarios = append(f.Cenarios, CenarioIF{Nome: c.nome, Retorno: math.Round(r*1e4) / 1e4,
			Meses: core.MesesParaAlvo(f.Investivel, f.Aporte, f.Alvo, r)})
	}
	return f, nil
}

// SimularQuitacao de uma dívida cadastrada.
func SimularQuitacao(ctx context.Context, tx pgx.Tx, dividaID string, hoje time.Time, extra core.Centavos, modo string) (core.SimulacaoQuitacao, error) {
	var ent string
	if err := tx.QueryRow(ctx, "select entidade_id from dividas where id = $1", dividaID).Scan(&ent); err != nil {
		return core.SimulacaoQuitacao{}, err
	}
	ds, err := Dividas(ctx, tx, []string{ent}, hoje)
	if err != nil {
		return core.SimulacaoQuitacao{}, err
	}
	for _, d := range ds {
		if d.ID != dividaID {
			continue
		}
		taxa, err := core.LerTaxa(d.TaxaMensal)
		if err != nil {
			return core.SimulacaoQuitacao{}, err
		}
		s, err := core.SimularAmortizacaoExtra(d.Sistema, d.tabela, taxa, dia(hoje), extra, modo)
		if err != nil {
			return s, ErrCampo{err.Error()}
		}
		return s, nil
	}
	return core.SimulacaoQuitacao{}, pgx.ErrNoRows
}
