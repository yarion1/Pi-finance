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
