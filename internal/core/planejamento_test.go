package core

import (
	"testing"
	"time"
)

func TestRitmoIdeal(t *testing.T) {
	// orçamento de R$ 900 em setembro (30 dias), dia 10: o ritmo ideal é R$ 300
	if got := RitmoIdeal(90000, 10, 30); got != 30000 {
		t.Errorf("ritmo: %d", got)
	}
	if RitmoIdeal(90000, 31, 30) != 90000 || RitmoIdeal(90000, 0, 30) != 0 || RitmoIdeal(100, 1, 3) != 33 {
		t.Error("limites do ritmo")
	}
	if DiasNoMes(2028, time.February) != 29 || DiasNoMes(2026, time.September) != 30 {
		t.Error("dias no mês")
	}
	casos := []struct {
		disp, gasto, ritmo Centavos
		quer               string
	}{{90000, 20000, 30000, "ok"}, {90000, 40000, 30000, "acima_do_ritmo"}, {90000, 95000, 30000, "estourado"}}
	for _, c := range casos {
		if got := SituacaoOrcamento(c.disp, c.gasto, c.ritmo); got != c.quer {
			t.Errorf("%+v → %s", c, got)
		}
	}
}

func TestMetas(t *testing.T) {
	hoje := dia("2026-09-25")
	alvo := dia("2027-08-01")
	// faltam R$ 12.000, de setembro/26 a agosto/27 são 12 meses: R$ 1.000 por mês
	aporte, meses := AporteNecessario(1500000, 300000, hoje, &alvo)
	if aporte != 100000 || meses != 12 {
		t.Errorf("aporte %d em %d meses", aporte, meses)
	}
	if a, _ := AporteNecessario(100, 200, hoje, &alvo); a != 0 {
		t.Error("meta alcançada")
	}
	if a, _ := AporteNecessario(100, 0, hoje, nil); a != 0 {
		t.Error("sem data")
	}
	passado := dia("2026-01-01")
	if a, m := AporteNecessario(1000, 0, hoje, &passado); a != 1000 || m != 1 {
		t.Error("data vencida: tudo agora")
	}

	d := DataPrevista(1500000, 300000, 100000, hoje)
	if d == nil || d.Format("2006-01-02") != "2027-08-31" {
		t.Errorf("data prevista %v", d)
	}
	if DataPrevista(10, 0, 0, hoje) != nil {
		t.Error("sem aporte não há previsão")
	}
	if DataPrevista(10, 20, 0, hoje).Format("2006-01-02") != "2026-09-25" {
		t.Error("já alcançada")
	}
	if MesesCobertos(900000, 200000) != 45 || MesesCobertos(1, 0) != -1 || MesesCobertos(-5, 10) != 0 {
		t.Error("meses cobertos")
	}
	if Media([]Centavos{100, 200, 301}) != 200 || Media(nil) != 0 {
		t.Error("média")
	}
}

func TestDetectarRecorrencia(t *testing.T) {
	netflix := []Ocorrencia{
		{dia("2026-06-12"), -3990}, {dia("2026-07-12"), -3990}, {dia("2026-08-13"), -3990}, {dia("2026-09-12"), -4490},
	}
	r, ok := DetectarRecorrencia(netflix)
	if !ok || r.Frequencia != "mensal" || r.Dia != 12 || r.Valor != -4490 || !r.Subiu || r.Proxima.Format("2006-01-02") != "2026-10-12" {
		t.Fatalf("netflix: %+v %v", r, ok)
	}

	// duas vezes só não basta para mensal
	if _, ok := DetectarRecorrencia(netflix[:2]); ok {
		t.Error("2 meses não é recorrência mensal")
	}
	// valores muito diferentes: não é conta fixa
	mercado := []Ocorrencia{{dia("2026-06-05"), -30000}, {dia("2026-07-05"), -12000}, {dia("2026-08-05"), -45000}}
	if _, ok := DetectarRecorrencia(mercado); ok {
		t.Error("mercado não é recorrência")
	}
	// intervalos irregulares
	irregular := []Ocorrencia{{dia("2026-06-01"), -1000}, {dia("2026-06-10"), -1000}, {dia("2026-08-20"), -1000}}
	if _, ok := DetectarRecorrencia(irregular); ok {
		t.Error("intervalos irregulares")
	}
	anual := []Ocorrencia{{dia("2025-03-10"), -19900}, {dia("2026-03-12"), -19900}}
	if r, ok := DetectarRecorrencia(anual); !ok || r.Frequencia != "anual" || r.Proxima.Format("2006-01-02") != "2027-03-12" || r.Subiu {
		t.Errorf("anual: %+v %v", r, ok)
	}

	if p := ProximaOcorrencia(dia("2026-09-12"), "mensal", 12, dia("2026-12-01")); p.Format("2006-01-02") != "2026-12-12" {
		t.Errorf("próxima mensal: %s", p.Format("2006-01-02"))
	}
	if p := ProximaOcorrencia(dia("2026-01-31"), "mensal", 31, dia("2026-01-31")); p.Format("2006-01-02") != "2026-02-28" {
		t.Errorf("31 em fevereiro: %s", p.Format("2006-01-02"))
	}
	if p := ProximaOcorrencia(dia("2026-09-01"), "semanal", 1, dia("2026-09-10")); p.Format("2006-01-02") != "2026-09-15" {
		t.Errorf("semanal: %s", p.Format("2006-01-02"))
	}
	if p := ProximaOcorrencia(dia("2026-03-12"), "anual", 12, dia("2026-09-25")); p.Format("2006-01-02") != "2027-03-12" {
		t.Errorf("anual: %s", p.Format("2006-01-02"))
	}
}
