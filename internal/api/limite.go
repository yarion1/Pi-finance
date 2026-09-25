package api

import (
	"sync"
	"time"
)

// limitador de requisições por chave (IP) em janela fixa, em memória.
// Complementa o bloqueio por conta, que fica no banco.
type limitador struct {
	mu      sync.Mutex
	max     int
	janela  time.Duration
	contas  map[string]*janela
	limpeza time.Time
}

type janela struct {
	inicio time.Time
	n      int
}

func novoLimitador(max int, j time.Duration) *limitador {
	return &limitador{max: max, janela: j, contas: map[string]*janela{}}
}

func (l *limitador) permitir(chave string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	agora := time.Now()
	if agora.Sub(l.limpeza) > 10*l.janela {
		for k, j := range l.contas {
			if agora.Sub(j.inicio) > l.janela {
				delete(l.contas, k)
			}
		}
		l.limpeza = agora
	}
	j, ok := l.contas[chave]
	if !ok || agora.Sub(j.inicio) > l.janela {
		l.contas[chave] = &janela{inicio: agora, n: 1}
		return true
	}
	j.n++
	return j.n <= l.max
}
