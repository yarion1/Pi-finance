package core

import (
	"math/big"
	"sort"
	"time"
)

// Classes de ativo que entram na apuração de renda variável.
const (
	ClasseAcao = "acao"
	ClasseETF  = "etf"
	ClasseBDR  = "bdr"
	ClasseFII  = "fii"
)

// Grupos de apuração: o prejuízo de um só compensa ganho do mesmo grupo.
const (
	GrupoComum    = "comum"     // ações, ETFs e BDRs, operações normais
	GrupoDayTrade = "day_trade" // ações, ETFs e BDRs no mesmo dia
	GrupoFII      = "fii"       // FIIs (normal e day trade)
)

// RegrasRendaVariavel vêm de regras_fiscais (chave ir_renda_variavel), com vigência.
type RegrasRendaVariavel struct {
	AliquotaComum    string   `json:"comum"`
	AliquotaDayTrade string   `json:"day_trade"`
	AliquotaFII      string   `json:"fii"`
	IsencaoAcoes     Centavos `json:"isencao_vendas_acoes_centavos"`
	DARFMinimo       Centavos `json:"darf_minimo_centavos"`
	CodigoDARF       string   `json:"codigo_darf"`
}

// VendaClassificada é uma venda com a classe do ativo.
type VendaClassificada struct {
	Venda
	Classe string
	Ativo  string
}

// ResultadoGrupo de um mês.
type ResultadoGrupo struct {
	Grupo              string   `json:"grupo"`
	Vendas             Centavos `json:"vendas_centavos"`
	Ganho              Centavos `json:"ganho_centavos"`  // resultado do mês (negativo = prejuízo)
	Isento             Centavos `json:"isento_centavos"` // ganho de ações isento (vendas até o limite)
	PrejuizoAnterior   Centavos `json:"prejuizo_anterior_centavos"`
	Base               Centavos `json:"base_centavos"`
	Imposto            Centavos `json:"imposto_centavos"`
	PrejuizoAcumulado  Centavos `json:"prejuizo_acumulado_centavos"` // para os próximos meses
	IRRetido           Centavos `json:"ir_retido_centavos"`
	AliquotaPercentual string   `json:"aliquota"`
}

// ApuracaoMes de renda variável.
type ApuracaoMes struct {
	Mes         string           `json:"mes"` // AAAA-MM
	Grupos      []ResultadoGrupo `json:"grupos"`
	VendasAcoes Centavos         `json:"vendas_acoes_centavos"`
	Isento      bool             `json:"isento_acoes"`
	Imposto     Centavos         `json:"imposto_centavos"`
	IRRetido    Centavos         `json:"ir_retido_centavos"`
	DARF        Centavos         `json:"darf_centavos"`           // a pagar neste mês
	Acumulado   Centavos         `json:"darf_acumulado_centavos"` // abaixo do mínimo: vai para o próximo
	Vencimento  time.Time        `json:"vencimento"`
	CodigoDARF  string           `json:"codigo_darf"`
}

func grupoDe(v VendaClassificada) string {
	switch {
	case v.Classe == ClasseFII:
		return GrupoFII
	case v.DayTrade:
		return GrupoDayTrade
	default:
		return GrupoComum
	}
}

// aplicarAliquota: base × alíquota ("0.15"), arredondado ao centavo.
func aplicarAliquota(base Centavos, aliquota string) Centavos {
	a, ok := new(big.Rat).SetString(aliquota)
	if !ok || base <= 0 {
		return 0
	}
	r := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(base)), a)
	return Centavos(dividirArredondando(new(big.Int).Set(r.Num()), r.Denom()).Int64())
}

// ApurarRendaVariavel apura mês a mês, em ordem: ganho por grupo, isenção das vendas de
// ações até o limite mensal (o prejuízo continua compensável), compensação do prejuízo
// acumulado de cada grupo, IR retido na fonte abatido, e DARF abaixo do mínimo somado
// ao mês seguinte. regras devolve as regras vigentes em cada mês.
func ApurarRendaVariavel(vendas []VendaClassificada, regras func(mes time.Time) RegrasRendaVariavel) []ApuracaoMes {
	porMes := map[string][]VendaClassificada{}
	var meses []string
	for _, v := range vendas {
		k := v.Data.Format("2006-01")
		if _, ok := porMes[k]; !ok {
			meses = append(meses, k)
		}
		porMes[k] = append(porMes[k], v)
	}
	sort.Strings(meses)

	prejuizo := map[string]Centavos{}
	var irRetidoSobra, darfSobra Centavos
	var out []ApuracaoMes
	for _, k := range meses {
		inicio, _ := time.Parse("2006-01", k)
		r := regras(inicio)
		ap := ApuracaoMes{Mes: k, CodigoDARF: r.CodigoDARF, Vencimento: UltimoDiaUtil(inicio.AddDate(0, 1, 0))}
		ganhos := map[string]Centavos{}
		vendasGrupo := map[string]Centavos{}
		var ganhoAcoes Centavos
		for _, v := range porMes[k] {
			g := grupoDe(v)
			ganhos[g] += v.Ganho
			vendasGrupo[g] += v.Valor
			ap.IRRetido += v.IRRetido
			if v.Classe == ClasseAcao && !v.DayTrade {
				ap.VendasAcoes += v.Valor
				ganhoAcoes += v.Ganho
			}
		}
		ap.Isento = ap.VendasAcoes > 0 && ap.VendasAcoes <= r.IsencaoAcoes
		for _, g := range []string{GrupoComum, GrupoDayTrade, GrupoFII} {
			if _, ok := ganhos[g]; !ok && prejuizo[g] == 0 {
				continue
			}
			rg := ResultadoGrupo{Grupo: g, Vendas: vendasGrupo[g], Ganho: ganhos[g], PrejuizoAnterior: prejuizo[g]}
			tributavel := ganhos[g]
			if g == GrupoComum && ap.Isento && ganhoAcoes > 0 {
				// ganho das ações é isento; prejuízo delas segue compensável
				rg.Isento = ganhoAcoes
				tributavel -= ganhoAcoes
			}
			switch g {
			case GrupoComum:
				rg.AliquotaPercentual = r.AliquotaComum
			case GrupoDayTrade:
				rg.AliquotaPercentual = r.AliquotaDayTrade
			default:
				rg.AliquotaPercentual = r.AliquotaFII
			}
			if tributavel > 0 {
				compensar := min(tributavel, prejuizo[g])
				rg.Base = tributavel - compensar
				prejuizo[g] -= compensar
			} else {
				prejuizo[g] -= tributavel
			}
			rg.Imposto = aplicarAliquota(rg.Base, rg.AliquotaPercentual)
			rg.PrejuizoAcumulado = prejuizo[g]
			ap.Imposto += rg.Imposto
			ap.Grupos = append(ap.Grupos, rg)
		}
		// IR retido abate o imposto; o que sobra fica para os meses seguintes
		retido := ap.IRRetido + irRetidoSobra
		abatido := min(retido, ap.Imposto)
		irRetidoSobra = retido - abatido
		devido := ap.Imposto - abatido + darfSobra
		if devido < r.DARFMinimo {
			ap.Acumulado, darfSobra = devido, devido
		} else {
			ap.DARF, darfSobra = devido, 0
		}
		out = append(out, ap)
	}
	return out
}

// ---------------------------------------------------------------------------
// Dias úteis (feriados nacionais e bancários)
// ---------------------------------------------------------------------------

// pascoa pelo algoritmo de Meeus/Jones/Butcher.
func pascoa(ano int) time.Time {
	a, b, c := ano%19, ano/100, ano%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	mes := (h + l - 7*m + 114) / 31
	dia := (h+l-7*m+114)%31 + 1
	return time.Date(ano, time.Month(mes), dia, 0, 0, 0, 0, time.UTC)
}

// Feriado nacional ou bancário (carnaval, sexta-feira santa e Corpus Christi fecham os bancos).
func Feriado(d time.Time) bool {
	d = truncarDia(d)
	fixos := map[[2]int]bool{{1, 1}: true, {4, 21}: true, {5, 1}: true, {9, 7}: true, {10, 12}: true,
		{11, 2}: true, {11, 15}: true, {11, 20}: true, {12, 25}: true}
	if fixos[[2]int{int(d.Month()), d.Day()}] {
		return true
	}
	p := pascoa(d.Year())
	for _, delta := range []int{-48, -47, -2, 60} { // carnaval (seg, ter), sexta santa, Corpus Christi
		if p.AddDate(0, 0, delta).Equal(d) {
			return true
		}
	}
	return false
}

// DiaUtil: nem fim de semana nem feriado.
func DiaUtil(d time.Time) bool {
	w := d.Weekday()
	return w != time.Saturday && w != time.Sunday && !Feriado(d)
}

// UltimoDiaUtil do mês da data (vencimento do DARF de renda variável).
func UltimoDiaUtil(mes time.Time) time.Time {
	d := DiaNoMes(mes.Year(), mes.Month(), 31)
	for !DiaUtil(d) {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// DiasUteis entre de (exclusive) e ate (inclusive), a contagem da renda fixa (base 252).
func DiasUteis(de, ate time.Time) int {
	n := 0
	for d := truncarDia(de).AddDate(0, 0, 1); !d.After(truncarDia(ate)); d = d.AddDate(0, 0, 1) {
		if DiaUtil(d) {
			n++
		}
	}
	return n
}
