package api

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/yarion1/pi-finance/internal/config"
)

func TestIPDoCliente(t *testing.T) {
	s := &Servidor{Config: config.Config{ProxiesConfiaveis: []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")}}}
	casos := []struct {
		nome, remoto, cf, xff, quer string
	}{
		{"direto", "100.64.0.5:1234", "", "", "100.64.0.5"},
		{"cloudflared confiável", "172.18.0.1:5555", "203.0.113.7", "10.0.0.1", "203.0.113.7"},
		{"x-forwarded-for confiável", "172.18.0.1:5555", "", "10.0.0.1, 198.51.100.2", "198.51.100.2"},
		{"cabeçalho forjado de fora", "100.64.0.5:1234", "203.0.113.7", "1.2.3.4", "100.64.0.5"},
		{"cf inválido cai no xff", "172.18.0.1:5555", "lixo", "198.51.100.2", "198.51.100.2"},
	}
	for _, c := range casos {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remoto
		if c.cf != "" {
			r.Header.Set("CF-Connecting-IP", c.cf)
		}
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := s.ip(r); got != c.quer {
			t.Errorf("%s: %s, esperado %s", c.nome, got, c.quer)
		}
	}
}
