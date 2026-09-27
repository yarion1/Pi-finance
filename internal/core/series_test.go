package core

import (
	"math"
	"testing"
	"time"
)

func perto(a *float64, b float64) bool { return a != nil && math.Abs(*a-b) < 1e-9 }

func TestSerieRetorno(t *testing.T) {
	d := []time.Time{dataUTC(2026, 1, 31), dataUTC(2026, 2, 28), dataUTC(2026, 3, 31)}
	// começa com 1.000; em fevereiro rende 10 %; em março aporta 1.100 e rende 0
	r := SerieRetorno(d, []Centavos{100000, 110000, 220000},
		[]Fluxo{{Data: dataUTC(2026, 1, 10), Valor: -99999}, {Data: dataUTC(2026, 3, 5), Valor: -110000}})
	if math.Abs(r[0]) > 1e-12 || math.Abs(r[1]-0.1) > 1e-12 || math.Abs(r[2]-0.1) > 1e-12 {
		t.Fatalf("retorno: %v", r)
	}
	if r := SerieRetorno(nil, nil, nil); len(r) != 0 {
		t.Fatal("vazio")
	}
}

func TestCDIAcumulado(t *testing.T) {
	d := []time.Time{dataUTC(2026, 1, 2), dataUTC(2026, 1, 5), dataUTC(2026, 3, 1)}
	cdi := []TaxaDia{{dataUTC(2026, 1, 2), 0.01}, {dataUTC(2026, 1, 3), 0.01}, {dataUTC(2026, 1, 5), 0.02}}
	r := CDIAcumulado(d, cdi)
	if !perto(r[0], 0) || !perto(r[1], 1.01*1.02-1) || r[2] != nil {
		t.Fatalf("cdi: %v %v %v", r[0], r[1], r[2])
	}
	if r := CDIAcumulado(d, []TaxaDia{{dataUTC(2026, 2, 1), 0.01}}); r[0] != nil {
		t.Fatal("série começa depois")
	}
	if r := CDIAcumulado(d, nil); r[0] != nil {
		t.Fatal("sem série")
	}
}

func TestIPCAAcumulado(t *testing.T) {
	d := []time.Time{dataUTC(2026, 1, 31), dataUTC(2026, 3, 31), dataUTC(2026, 4, 30)}
	r := IPCAAcumulado(d, map[string]float64{"2026-02": 0.01, "2026-03": 0.02})
	if !perto(r[0], 0) || !perto(r[1], 1.01*1.02-1) || r[2] != nil {
		t.Fatalf("ipca: %v %v %v", r[0], r[1], r[2])
	}
	if len(IPCAAcumulado(nil, nil)) != 0 {
		t.Fatal("vazio")
	}
}

func TestPrecoAcumulado(t *testing.T) {
	d := []time.Time{dataUTC(2026, 1, 31), dataUTC(2026, 2, 28)}
	r := PrecoAcumulado(d, []PrecoData{{dataUTC(2026, 1, 30), 100}, {dataUTC(2026, 2, 27), 110}, {dataUTC(2026, 3, 2), 999}})
	if !perto(r[0], 0) || !perto(r[1], 0.1) {
		t.Fatalf("preço: %v %v", r[0], r[1])
	}
	if r := PrecoAcumulado(d, []PrecoData{{dataUTC(2026, 2, 1), 100}}); r[0] != nil || r[1] != nil {
		t.Fatal("sem preço na primeira data")
	}
	if len(PrecoAcumulado(nil, nil)) != 0 {
		t.Fatal("vazio")
	}
}
