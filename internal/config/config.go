// Package config lê a configuração do ambiente (/etc/financas/.env no Pi).
package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config do servidor e do worker.
type Config struct {
	DatabaseURL         string
	DatabaseURLMigracao string
	SenhaAppDB          string
	ArquivoChave        string
	URLPublica          string
	Endereco            string
	ProxiesConfiaveis   []netip.Prefix
	ForcarFalhaSaude    bool
	LimiteAuthPorMinuto int
	HSTS                bool
	URLPluggy           string
	TokenBrapi          string
}

func env(chave, padrao string) string {
	if v := strings.TrimSpace(os.Getenv(chave)); v != "" {
		return v
	}
	return padrao
}

// Carregar lê as variáveis. Campos obrigatórios são conferidos por quem usa.
func Carregar() (Config, error) {
	c := Config{
		DatabaseURL:         env("DATABASE_URL", ""),
		DatabaseURLMigracao: env("DATABASE_URL_MIGRACAO", ""),
		SenhaAppDB:          env("APP_DB_PASSWORD", ""),
		ArquivoChave:        env("MASTER_KEY_FILE", "/etc/financas/master.key"),
		URLPublica:          strings.TrimRight(env("PUBLIC_URL", ""), "/"),
		Endereco:            env("ENDERECO", ":3100"),
		ForcarFalhaSaude:    env("FORCAR_FALHA_HEALTH", "") == "1",
		LimiteAuthPorMinuto: 10,
		HSTS:                strings.HasPrefix(env("PUBLIC_URL", ""), "https:"),
		URLPluggy:           env("PLUGGY_URL", ""),
		TokenBrapi:          env("BRAPI_TOKEN", ""),
	}
	if v := env("LIMITE_AUTH_POR_MINUTO", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return c, fmt.Errorf("LIMITE_AUTH_POR_MINUTO inválido: %q", v)
		}
		c.LimiteAuthPorMinuto = n
	}
	for _, p := range strings.Split(env("PROXIES_CONFIAVEIS", "127.0.0.1/32,::1/128,172.16.0.0/12"), ",") {
		pref, err := netip.ParsePrefix(strings.TrimSpace(p))
		if err != nil {
			return c, fmt.Errorf("PROXIES_CONFIAVEIS: %w", err)
		}
		c.ProxiesConfiaveis = append(c.ProxiesConfiaveis, pref)
	}
	return c, nil
}

// ExigirServidor confere o que serve e worker precisam.
func (c Config) ExigirServidor() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL não definido")
	}
	u, err := url.Parse(c.URLPublica)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("PUBLIC_URL inválida ou vazia: %q", c.URLPublica)
	}
	return nil
}

// Segura: HTTPS, ou localhost (que os navegadores tratam como seguro).
// Sem isso não há cookie Secure, prefixo __Host-, HSTS nem passkey.
func (c Config) Segura() bool {
	u, err := url.Parse(c.URLPublica)
	if err != nil {
		return false
	}
	return u.Scheme == "https" || u.Hostname() == "localhost"
}

// Origem é esquema://host[:porta] da URL pública (comparada com o cabeçalho Origin).
func (c Config) Origem() string {
	u, err := url.Parse(c.URLPublica)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
