// Package api expõe a API JSON e serve o front (SPA) embutido.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/config"
	"github.com/yarion1/pi-finance/internal/cripto"
)

const (
	cookieSessao  = "__Host-financas"
	cookieDesafio = "__Host-financas-wa"
	limiteCorpo   = 1 << 20
)

// Servidor reúne as dependências das rotas.
type Servidor struct {
	Auth     *auth.Servico
	Pool     *pgxpool.Pool
	Cifrador *cripto.Cifrador
	Config   config.Config
	Versao   string
	Front    fs.FS // conteúdo de web/dist; nil = sem front

	limiteAuth *limitador
}

// Handler monta as rotas com os middlewares.
func (s *Servidor) Handler() http.Handler {
	s.limiteAuth = novoLimitador(10, time.Minute)
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.saude)

	// autenticação (as regras de sessão parcial ficam no serviço)
	mux.HandleFunc("GET /api/auth/estado", s.estadoCadastro)
	mux.Handle("GET /api/auth/sessao", s.sessaoOpcional(s.verSessao))
	mux.Handle("POST /api/auth/cadastro", s.limitado(http.HandlerFunc(s.cadastrar)))
	mux.Handle("POST /api/auth/entrar", s.limitado(http.HandlerFunc(s.entrar)))
	mux.Handle("POST /api/auth/sair", s.sessaoParcial(s.sair))
	mux.Handle("POST /api/auth/2fa/verificar", s.limitado(s.sessaoParcial(s.verificar2FA)))
	mux.Handle("POST /api/auth/reautenticar", s.limitado(s.sessaoCompleta(s.reautenticar)))
	mux.Handle("POST /api/auth/totp/iniciar", s.sessaoParcial(s.iniciarTOTP))
	mux.Handle("POST /api/auth/totp/confirmar", s.limitado(s.sessaoParcial(s.confirmarTOTP)))
	mux.Handle("POST /api/auth/recuperacao/gerar", s.sessaoCompleta(s.gerarRecuperacao))
	mux.Handle("POST /api/auth/passkey/registro/iniciar", s.sessaoParcial(s.iniciarRegistroPasskey))
	mux.Handle("POST /api/auth/passkey/registro/concluir", s.sessaoParcial(s.concluirRegistroPasskey))
	mux.Handle("POST /api/auth/passkey/login/iniciar", s.limitado(s.sessaoOpcional(s.iniciarLoginPasskey)))
	mux.Handle("POST /api/auth/passkey/login/concluir", s.limitado(s.sessaoOpcional(s.concluirLoginPasskey)))
	mux.Handle("GET /api/auth/passkeys", s.sessaoCompleta(s.listarPasskeys))
	mux.Handle("DELETE /api/auth/passkeys/{id}", s.sessaoCompleta(s.removerPasskey))

	// convites
	mux.Handle("GET /api/convites/{token}", s.limitado(http.HandlerFunc(s.verConvite)))
	mux.Handle("POST /api/convites/{token}/aceitar", s.sessaoCompleta(s.aceitarConvite))

	// casas
	mux.Handle("GET /api/casas", s.sessaoCompleta(s.listarCasas))
	mux.Handle("POST /api/casas", s.sessaoCompleta(s.criarCasa))
	mux.Handle("GET /api/casas/{id}", s.sessaoCompleta(s.verCasa))
	mux.Handle("PATCH /api/casas/{id}", s.sessaoCompleta(s.renomearCasa))
	mux.Handle("POST /api/casas/{id}/convites", s.sessaoCompleta(s.criarConvite))
	mux.Handle("DELETE /api/casas/{id}/convites/{convite}", s.sessaoCompleta(s.cancelarConvite))
	mux.Handle("PATCH /api/casas/{id}/membros/{usuario}", s.sessaoCompleta(s.mudarPapelMembro))
	mux.Handle("DELETE /api/casas/{id}/membros/{usuario}", s.sessaoCompleta(s.removerMembro))

	// entidades
	mux.Handle("GET /api/entidades", s.sessaoCompleta(s.listarEntidades))
	mux.Handle("POST /api/entidades", s.sessaoCompleta(s.criarEntidade))
	mux.Handle("GET /api/entidades/{id}", s.sessaoCompleta(s.verEntidade))
	mux.Handle("PATCH /api/entidades/{id}", s.sessaoCompleta(s.editarEntidade))
	mux.Handle("DELETE /api/entidades/{id}", s.sessaoCompleta(s.apagarEntidade))
	mux.Handle("POST /api/entidades/{id}/acessos", s.sessaoCompleta(s.darAcesso))
	mux.Handle("DELETE /api/entidades/{id}/acessos/{usuario}", s.sessaoCompleta(s.removerAcesso))

	// dinheiro (leitura mínima; a fase 1 completa)
	mux.Handle("GET /api/contas", s.sessaoCompleta(s.listarContas))
	mux.Handle("GET /api/transacoes", s.sessaoCompleta(s.listarTransacoes))
	mux.Handle("GET /api/alertas", s.sessaoCompleta(s.listarAlertas))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		erroJSON(w, http.StatusNotFound, "nao_encontrado", "rota inexistente")
	})
	mux.Handle("/", s.spa())

	return s.recuperar(s.registrar(s.cabecalhos(s.origemConfere(limitarCorpo(mux)))))
}

// Rotas usadas no teste de isolamento: toda rota GET com dados precisa estar aqui.
var RotasLeitura = []string{
	"/api/auth/sessao", "/api/auth/passkeys", "/api/casas", "/api/casas/{casa}",
	"/api/entidades", "/api/entidades/{entidade}", "/api/contas", "/api/transacoes",
	"/api/transacoes?conta_id={conta}", "/api/alertas",
}

// ---------------------------------------------------------------------------
// middlewares
// ---------------------------------------------------------------------------

func (s *Servidor) recuperar(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("pânico", "erro", p, "rota", r.URL.Path, "pilha", string(debug.Stack()))
				erroJSON(w, http.StatusInternalServerError, "interno", "erro interno")
			}
		}()
		h.ServeHTTP(w, r)
	})
}

type gravadorStatus struct {
	http.ResponseWriter
	status int
}

func (g *gravadorStatus) WriteHeader(c int) { g.status = c; g.ResponseWriter.WriteHeader(c) }

func (s *Servidor) registrar(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		g := &gravadorStatus{ResponseWriter: w, status: 200}
		h.ServeHTTP(g, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/health" {
			slog.Info("req", "metodo", r.Method, "rota", r.URL.Path, "status", g.status, "ms", time.Since(inicio).Milliseconds())
		}
	})
}

// Política de segurança: sem scripts ou estilos de terceiros, sem inline.
const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; " +
	"font-src 'self'; connect-src 'self'; manifest-src 'self'; worker-src 'self'; object-src 'none'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

func (s *Servidor) cabecalhos(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := w.Header()
		c.Set("Content-Security-Policy", csp)
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Cross-Origin-Opener-Policy", "same-origin")
		c.Set("Cross-Origin-Resource-Policy", "same-origin")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		if s.Config.HSTS {
			c.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			c.Set("Cache-Control", "no-store")
		}
		h.ServeHTTP(w, r)
	})
}

// origemConfere é a proteção CSRF: toda escrita precisa vir da própria origem
// (além de os cookies serem SameSite=Strict) e mandar JSON.
func (s *Servidor) origemConfere(h http.Handler) http.Handler {
	esperada := s.Config.Origem()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			h.ServeHTTP(w, r)
			return
		}
		origem := r.Header.Get("Origin")
		mesmaOrigem := origem == esperada ||
			(origem == "" && r.Header.Get("Sec-Fetch-Site") == "same-origin")
		if !mesmaOrigem {
			erroJSON(w, http.StatusForbidden, "origem", "origem da requisição não permitida")
			return
		}
		if r.ContentLength != 0 && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			erroJSON(w, http.StatusUnsupportedMediaType, "tipo", "envie application/json")
			return
		}
		h.ServeHTTP(w, r)
	})
}

func limitarCorpo(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limiteCorpo)
		h.ServeHTTP(w, r)
	})
}

func (s *Servidor) limitado(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.limiteAuth.permitir(s.ip(r)) {
			w.Header().Set("Retry-After", "60")
			erroJSON(w, http.StatusTooManyRequests, "limite", "muitas tentativas; aguarde um minuto")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// ip do cliente: CF-Connecting-IP e X-Forwarded-For só são aceitos vindo de proxy
// confiável (cloudflared ou tailscale serve no próprio Pi, rede do Docker).
func (s *Servidor) ip(r *http.Request) string {
	remoto, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	endereco := remoto.Addr().Unmap()
	confiavel := false
	for _, p := range s.Config.ProxiesConfiaveis {
		if p.Contains(endereco) {
			confiavel = true
			break
		}
	}
	if confiavel {
		// cloudflared (Cloudflare Tunnel) manda o IP do visitante aqui
		if cf, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("Cf-Connecting-Ip"))); err == nil {
			return cf.String()
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			partes := strings.Split(xff, ",")
			if a, err := netip.ParseAddr(strings.TrimSpace(partes[len(partes)-1])); err == nil {
				return a.String()
			}
		}
	}
	return endereco.String()
}

func (s *Servidor) origem(r *http.Request) auth.Origem {
	return auth.Origem{IP: s.ip(r), UserAgent: r.UserAgent()}
}

// ---------------------------------------------------------------------------
// sessão
// ---------------------------------------------------------------------------

type chaveSessao struct{}

func sessaoDe(r *http.Request) *auth.Sessao {
	ss, _ := r.Context().Value(chaveSessao{}).(*auth.Sessao)
	return ss
}

func (s *Servidor) lerSessao(r *http.Request) *auth.Sessao {
	c, err := r.Cookie(cookieSessao)
	if err != nil {
		return nil
	}
	ss, err := s.Auth.SessaoPorToken(r.Context(), c.Value)
	if err != nil {
		return nil
	}
	return ss
}

func (s *Servidor) comSessao(h http.HandlerFunc, exigir, completa bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ss := s.lerSessao(r)
		if ss == nil && exigir {
			s.apagarCookie(w, cookieSessao)
			erroJSON(w, http.StatusUnauthorized, "sessao", "faça login")
			return
		}
		if ss != nil && completa && !ss.MFAOK {
			erroJSON(w, http.StatusForbidden, "segundo_fator", "confirme o segundo fator")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), chaveSessao{}, ss)))
	})
}

func (s *Servidor) sessaoOpcional(h http.HandlerFunc) http.Handler {
	return s.comSessao(h, false, false)
}
func (s *Servidor) sessaoParcial(h http.HandlerFunc) http.Handler  { return s.comSessao(h, true, false) }
func (s *Servidor) sessaoCompleta(h http.HandlerFunc) http.Handler { return s.comSessao(h, true, true) }

func (s *Servidor) gravarCookie(w http.ResponseWriter, nome, valor string, duracao time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: nome, Value: valor, Path: "/", MaxAge: int(duracao.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Servidor) apagarCookie(w http.ResponseWriter, nome string) {
	http.SetCookie(w, &http.Cookie{
		Name: nome, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Servidor) gravarSessao(w http.ResponseWriter, token string, completa bool, userAgent string) {
	duracao := auth.DuracaoParcial
	if completa {
		duracao = auth.DuracaoNavegador
		if auth.EhCelular(userAgent) {
			duracao = auth.DuracaoCelular
		}
	}
	s.gravarCookie(w, cookieSessao, token, duracao)
}
