package core

import (
	"testing"
	"time"
)

func TestEstadoWorker(t *testing.T) {
	agora := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	em := func(d time.Duration) *time.Time { v := agora.Add(-d); return &v }
	if EstadoWorker(nil, agora) != "down" {
		t.Error("sem sinal deveria ser down")
	}
	if EstadoWorker(em(time.Minute), agora) != "up" {
		t.Error("1 min deveria ser up")
	}
	if EstadoWorker(em(3*time.Minute), agora) != "up" {
		t.Error("3 min exatos ainda é up")
	}
	if EstadoWorker(em(3*time.Minute+time.Second), agora) != "down" {
		t.Error("mais de 3 min deveria ser down")
	}
}

func TestEstadoBackup(t *testing.T) {
	agora := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	em := func(d time.Duration) *time.Time { v := agora.Add(-d); return &v }
	casos := []struct {
		ultimo *time.Time
		quer   string
	}{{nil, "pendente"}, {em(time.Hour), "ok"}, {em(9 * time.Hour), "atrasado"}}
	for _, c := range casos {
		if got := EstadoBackup(c.ultimo, agora); got != c.quer {
			t.Errorf("EstadoBackup = %s, esperado %s", got, c.quer)
		}
	}
}

func TestEstadoPluggy(t *testing.T) {
	agora := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	ontem, velha := agora.Add(-20*time.Hour), agora.Add(-40*time.Hour)
	casos := []struct {
		conexoes, erros int
		ultima          *time.Time
		quer            string
	}{
		{0, 0, nil, "ok"},
		{2, 0, &ontem, "ok"},
		{2, 1, &ontem, "erro"},
		{1, 0, &velha, "atrasada"},
		{1, 0, nil, "atrasada"},
	}
	for _, c := range casos {
		if got := EstadoPluggy(c.conexoes, c.erros, c.ultima, agora); got != c.quer {
			t.Errorf("EstadoPluggy(%d, %d, %v) = %s, esperado %s", c.conexoes, c.erros, c.ultima, got, c.quer)
		}
	}
}
