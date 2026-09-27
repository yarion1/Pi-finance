package core

import (
	"errors"
	"reflect"
	"testing"
)

func TestDividirIgual(t *testing.T) {
	p, err := DividirIgual(10001, []string{"a", "b", "c"})
	if err != nil || !reflect.DeepEqual(p, []Parte{{"a", 3334}, {"b", 3334}, {"c", 3333}}) {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := DividirIgual(100, []string{"a"}); !errors.Is(err, ErrDivisaoVazia) {
		t.Fatal("uma pessoa só")
	}
	if _, err := DividirIgual(100, []string{"a", "a"}); !errors.Is(err, ErrDivisaoDupla) {
		t.Fatal("repetida")
	}
	if _, err := DividirIgual(0, []string{"a", "b"}); !errors.Is(err, ErrDivisaoParte) {
		t.Fatal("zero")
	}
}

func TestDividirPorPercentual(t *testing.T) {
	p, err := DividirPorPercentual(10000, []string{"a", "b"}, []int64{6000, 4000})
	if err != nil || !reflect.DeepEqual(p, []Parte{{"a", 6000}, {"b", 4000}}) {
		t.Fatalf("60/40: %+v %v", p, err)
	}
	// 100 centavos em terços: 33,33 cada; o centavo que sobra vai para quem perdeu mais
	p, err = DividirPorPercentual(100, []string{"a", "b", "c"}, []int64{3333, 3333, 3334})
	if err != nil || p[0].Valor+p[1].Valor+p[2].Valor != 100 || !reflect.DeepEqual(p, []Parte{{"a", 33}, {"b", 33}, {"c", 34}}) {
		t.Fatalf("terços: %+v %v", p, err)
	}
	p, err = DividirPorPercentual(7, []string{"a", "b"}, []int64{5000, 5000})
	if err != nil || !reflect.DeepEqual(p, []Parte{{"a", 4}, {"b", 3}}) {
		t.Fatalf("ímpar: %+v %v", p, err)
	}
	for _, c := range []struct {
		basis []int64
		total Centavos
		erro  error
	}{
		{[]int64{5000, 4000}, 100, ErrDivisaoSoma},
		{[]int64{10000, 0}, 100, ErrDivisaoParte},
		{[]int64{5000}, 100, ErrDivisaoSoma},
		{[]int64{5000, 5000}, 0, ErrDivisaoSoma},
		{[]int64{9999, 1}, 100, ErrDivisaoParte}, // a parte de 0,01 % de R$ 1 dá zero
	} {
		if _, err := DividirPorPercentual(c.total, []string{"a", "b"}, c.basis); !errors.Is(err, c.erro) {
			t.Errorf("%v: %v", c.basis, err)
		}
	}
	if _, err := DividirPorPercentual(100, []string{"a"}, []int64{10000}); !errors.Is(err, ErrDivisaoVazia) {
		t.Fatal("uma pessoa")
	}
}

func TestDividirPorValor(t *testing.T) {
	p, err := DividirPorValor(5000, []Parte{{"a", 3000}, {"b", 2000}})
	if err != nil || len(p) != 2 {
		t.Fatalf("%+v %v", p, err)
	}
	for _, c := range []struct {
		partes []Parte
		erro   error
	}{
		{[]Parte{{"a", 3000}, {"b", 1000}}, ErrDivisaoSoma},
		{[]Parte{{"a", 5000}, {"b", 0}}, ErrDivisaoParte},
		{[]Parte{{"a", 2500}, {"a", 2500}}, ErrDivisaoDupla},
		{[]Parte{{"a", 5000}}, ErrDivisaoVazia},
	} {
		if _, err := DividirPorValor(5000, c.partes); !errors.Is(err, c.erro) {
			t.Errorf("%+v: %v", c.partes, err)
		}
	}
}

func TestQuemDeveQuem(t *testing.T) {
	d := []Divida{
		{Devedor: "bia", Credor: "ana", Valor: 5000},  // Bia deve 50 à Ana
		{Devedor: "ana", Credor: "bia", Valor: 2000},  // Ana deve 20 à Bia
		{Devedor: "caio", Credor: "ana", Valor: 1000}, // Caio deve 10 à Ana
		{Devedor: "ana", Credor: "davi", Valor: 7000}, // Ana deve 70 ao Davi
		{Devedor: "ana", Credor: "ana", Valor: 9999},  // a parte de quem pagou
		{Devedor: "bia", Credor: "caio", Valor: 300},  // entre outros: não é comigo
		{Devedor: "eva", Credor: "ana", Valor: 500},
		{Devedor: "ana", Credor: "eva", Valor: 500}, // zerou
	}
	got := QuemDeveQuem("ana", d)
	want := []SaldoCom{{"davi", -7000}, {"bia", 3000}, {"caio", 1000}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	if got := QuemDeveQuem("x", nil); len(got) != 0 {
		t.Fatal("vazio")
	}
	// empate no valor: ordem pelo id
	got = QuemDeveQuem("ana", []Divida{{"z", "ana", 100}, {"ana", "b", 100}})
	if got[0].UsuarioID != "b" || got[1].UsuarioID != "z" {
		t.Fatalf("empate: %+v", got)
	}
}
