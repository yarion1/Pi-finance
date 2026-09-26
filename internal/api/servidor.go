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
	"github.com/yarion1/pi-finance/internal/openfinance"
)

const (
	cookieSessao  = "financas"
	cookieDesafio = "financas-wa"
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
	// OpenFinance sincroniza com o Meu Pluggy (fase 3).
	OpenFinance *openfinance.Servico

	limiteAuth    *limitador
	limiteConvite *limitador
	limiteGeral   *limitador // toda a API, por IP (contra abuso e raspagem)
	limiteEscrita *limitador // POST, PUT, PATCH e DELETE, por IP
}

// Limites gerais por IP e minuto; folgados para uso normal (uma tela faz poucas chamadas).
const (
	LimiteGeralPorMinuto   = 600
	LimiteEscritaPorMinuto = 120
)

// Handler monta as rotas com os middlewares.
func (s *Servidor) Handler() http.Handler {
	porMinuto := s.Config.LimiteAuthPorMinuto
	if porMinuto < 1 {
		porMinuto = 10
	}
	s.limiteAuth = novoLimitador(porMinuto, time.Minute)
	s.limiteConvite = novoLimitador(30, time.Minute)
	s.limiteGeral = novoLimitador(LimiteGeralPorMinuto, time.Minute)
	s.limiteEscrita = novoLimitador(LimiteEscritaPorMinuto, time.Minute)
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
	mux.Handle("GET /api/convites/{token}", s.limitadoPor(s.limiteConvite, http.HandlerFunc(s.verConvite)))
	mux.Handle("POST /api/convites/{token}/aceitar", s.sessaoCompleta(s.aceitarConvite))
	mux.Handle("GET /api/convites-conta", s.sessaoCompleta(s.listarConvitesConta))
	mux.Handle("POST /api/convites-conta", s.sessaoCompleta(s.criarConviteConta))
	mux.Handle("DELETE /api/convites-conta/{id}", s.sessaoCompleta(s.cancelarConviteConta))

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

	// contas e transações (fase 1)
	mux.Handle("GET /api/instituicoes", s.sessaoCompleta(s.listarInstituicoes))
	mux.Handle("GET /api/contas", s.sessaoCompleta(s.listarContas))
	mux.Handle("POST /api/contas", s.sessaoCompleta(s.criarConta))
	mux.Handle("PATCH /api/contas/{id}", s.sessaoCompleta(s.editarConta))
	mux.Handle("DELETE /api/contas/{id}", s.sessaoCompleta(s.apagarConta))
	mux.Handle("GET /api/transacoes", s.sessaoCompleta(s.listarTransacoes))
	mux.Handle("POST /api/transacoes", s.sessaoCompleta(s.criarTransacao))
	mux.Handle("POST /api/transacoes/lote", s.sessaoCompleta(s.categorizarLote))
	mux.Handle("PATCH /api/transacoes/{id}", s.sessaoCompleta(s.editarTransacao))
	mux.Handle("DELETE /api/transacoes/{id}", s.sessaoCompleta(s.apagarTransacao))
	mux.Handle("GET /api/importacoes", s.sessaoCompleta(s.listarImportacoes))
	mux.Handle("POST /api/importacoes", s.sessaoCompleta(s.importar))
	mux.Handle("POST /api/importacoes/cabecalhos", s.sessaoCompleta(s.cabecalhosCSV))
	mux.Handle("DELETE /api/importacoes/{id}", s.sessaoCompleta(s.desfazerImportacao))
	mux.Handle("GET /api/mapeamentos", s.sessaoCompleta(s.listarMapeamentos))
	mux.Handle("POST /api/mapeamentos", s.sessaoCompleta(s.salvarMapeamento))
	mux.Handle("DELETE /api/mapeamentos/{id}", s.sessaoCompleta(s.apagarMapeamento))
	mux.Handle("GET /api/categorias", s.sessaoCompleta(s.listarCategorias))
	mux.Handle("POST /api/categorias", s.sessaoCompleta(s.criarCategoria))
	mux.Handle("PATCH /api/categorias/{id}", s.sessaoCompleta(s.editarCategoria))
	mux.Handle("DELETE /api/categorias/{id}", s.sessaoCompleta(s.apagarCategoria))
	mux.Handle("GET /api/regras", s.sessaoCompleta(s.listarRegras))
	mux.Handle("POST /api/regras", s.sessaoCompleta(s.criarRegra))
	mux.Handle("POST /api/regras/aplicar", s.sessaoCompleta(s.aplicarRegras))
	mux.Handle("DELETE /api/regras/{id}", s.sessaoCompleta(s.apagarRegra))
	mux.Handle("GET /api/resumo", s.sessaoCompleta(s.resumo))

	// planejamento (fase 2)
	mux.Handle("GET /api/contas/{id}/faturas", s.sessaoCompleta(s.faturas))
	mux.Handle("GET /api/indicadores", s.sessaoCompleta(s.indicadores))
	mux.Handle("GET /api/agenda", s.sessaoCompleta(s.agenda))
	mux.Handle("GET /api/recorrencias", s.sessaoCompleta(s.listarRecorrencias))
	mux.Handle("POST /api/recorrencias", s.sessaoCompleta(s.criarRecorrencia))
	mux.Handle("POST /api/recorrencias/detectar", s.sessaoCompleta(s.detectarRecorrencias))
	mux.Handle("PATCH /api/recorrencias/{id}", s.sessaoCompleta(s.editarRecorrencia))
	mux.Handle("DELETE /api/recorrencias/{id}", s.sessaoCompleta(s.apagarRecorrencia))
	mux.Handle("GET /api/orcamento", s.sessaoCompleta(s.orcamento))
	mux.Handle("PUT /api/orcamento", s.sessaoCompleta(s.salvarOrcamento))
	mux.Handle("POST /api/orcamento/copiar", s.sessaoCompleta(s.copiarOrcamento))
	mux.Handle("PUT /api/orcamento/config", s.sessaoCompleta(s.configurarOrcamento))
	mux.Handle("GET /api/metas", s.sessaoCompleta(s.listarMetasHTTP))
	mux.Handle("POST /api/metas", s.sessaoCompleta(s.criarMeta))
	mux.Handle("PATCH /api/metas/{id}", s.sessaoCompleta(s.editarMeta))
	mux.Handle("DELETE /api/metas/{id}", s.sessaoCompleta(s.apagarMeta))
	mux.Handle("GET /api/patrimonio", s.sessaoCompleta(s.patrimonio))
	mux.Handle("POST /api/bens", s.sessaoCompleta(s.criarBem))
	mux.Handle("PATCH /api/bens/{id}", s.sessaoCompleta(s.editarBem))
	mux.Handle("DELETE /api/bens/{id}", s.sessaoCompleta(s.apagarBem))
	mux.Handle("POST /api/dividas", s.sessaoCompleta(s.criarDivida))
	mux.Handle("PATCH /api/dividas/{id}", s.sessaoCompleta(s.editarDivida))
	mux.Handle("DELETE /api/dividas/{id}", s.sessaoCompleta(s.apagarDivida))
	mux.Handle("GET /api/dividas/{id}/tabela", s.sessaoCompleta(s.tabelaDivida))
	mux.Handle("GET /api/compromissos", s.sessaoCompleta(s.listarCompromissos))
	mux.Handle("POST /api/compromissos", s.sessaoCompleta(s.criarCompromisso))
	mux.Handle("PATCH /api/compromissos/{id}", s.sessaoCompleta(s.editarCompromisso))
	mux.Handle("DELETE /api/compromissos/{id}", s.sessaoCompleta(s.apagarCompromisso))
	mux.Handle("GET /api/alertas", s.sessaoCompleta(s.listarAlertas))

	// Open Finance (Meu Pluggy): cada pessoa só vê e mexe na própria conexão
	mux.Handle("GET /api/open-finance", s.sessaoCompleta(s.openFinance))
	mux.Handle("PUT /api/open-finance/credenciais", s.sessaoCompleta(s.salvarCredenciais))
	mux.Handle("DELETE /api/open-finance", s.sessaoCompleta(s.apagarConexao))
	mux.Handle("POST /api/open-finance/itens", s.sessaoCompleta(s.adicionarItem))
	mux.Handle("DELETE /api/open-finance/itens/{id}", s.sessaoCompleta(s.apagarItem))
	mux.Handle("PATCH /api/open-finance/contas/{id}", s.sessaoCompleta(s.vincularConta))
	mux.Handle("POST /api/open-finance/sincronizar", s.sessaoCompleta(s.sincronizarAgora))
	mux.Handle("POST /api/contas/{id}/acertar-saldo", s.sessaoCompleta(s.acertarSaldo))

	// Investimentos (fase 4)
	mux.Handle("GET /api/investimentos", s.sessaoCompleta(s.carteira))
	mux.Handle("GET /api/investimentos/ir", s.sessaoCompleta(s.irInvestimentos))
	mux.Handle("POST /api/investimentos/b3", s.sessaoCompleta(s.importarB3))
	mux.Handle("POST /api/investimentos/ativos", s.sessaoCompleta(s.criarAtivo))
	mux.Handle("PATCH /api/investimentos/ativos/{id}", s.sessaoCompleta(s.editarAtivo))
	mux.Handle("DELETE /api/investimentos/ativos/{id}", s.sessaoCompleta(s.apagarAtivo))
	mux.Handle("GET /api/investimentos/ativos/{id}/operacoes", s.sessaoCompleta(s.listarOperacoes))
	mux.Handle("POST /api/investimentos/ativos/{id}/operacoes", s.sessaoCompleta(s.criarOperacao))
	mux.Handle("DELETE /api/investimentos/ativos/{id}/operacoes/{operacao}", s.sessaoCompleta(s.apagarOperacao))

	// CNPJ (fase 5)
	mux.Handle("GET /api/cnpj/simulacao", s.sessaoCompleta(s.simulacaoCNPJ))
	mux.Handle("GET /api/cnpj/{entidade}/painel", s.sessaoCompleta(s.painelCNPJ))
	mux.Handle("GET /api/cnpj/{entidade}/notas", s.sessaoCompleta(s.listarNotas))
	mux.Handle("POST /api/cnpj/{entidade}/notas", s.sessaoCompleta(s.criarNota))
	mux.Handle("POST /api/cnpj/{entidade}/notas/xml", s.sessaoCompleta(s.importarNFSe))
	mux.Handle("PATCH /api/cnpj/notas/{id}", s.sessaoCompleta(s.editarNota))
	mux.Handle("DELETE /api/cnpj/notas/{id}", s.sessaoCompleta(s.apagarNota))
	mux.Handle("PUT /api/cnpj/{entidade}/das/{competencia}", s.sessaoCompleta(s.marcarDAS))
	mux.Handle("GET /api/cnpj/{entidade}/folha", s.sessaoCompleta(s.listarFolha))
	mux.Handle("PUT /api/cnpj/{entidade}/folha", s.sessaoCompleta(s.salvarFolha))
	mux.Handle("GET /api/cnpj/{entidade}/distribuicoes", s.sessaoCompleta(s.listarDistribuicoes))
	mux.Handle("POST /api/cnpj/{entidade}/distribuicoes", s.sessaoCompleta(s.distribuirLucro))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		erroJSON(w, http.StatusNotFound, "nao_encontrado", "rota inexistente")
	})
	mux.Handle("/", s.spa())

	return s.recuperar(s.registrar(s.cabecalhos(s.limitarAPI(s.origemConfere(limitarCorpo(mux))))))
}

// Rotas usadas no teste de isolamento: toda rota GET com dados precisa estar aqui.
var RotasLeitura = []string{
	"/api/auth/sessao", "/api/auth/passkeys", "/api/casas", "/api/casas/{casa}",
	"/api/entidades", "/api/entidades/{entidade}", "/api/alertas", "/api/instituicoes",
	"/api/contas", "/api/contas?entidade_id={entidade}",
	"/api/transacoes", "/api/transacoes?conta_id={conta}", "/api/transacoes?entidade_id={entidade}",
	"/api/transacoes?busca=SEGREDO", "/api/transacoes?importacao_id={importacao}",
	"/api/importacoes", "/api/importacoes?entidade_id={entidade}", "/api/mapeamentos",
	"/api/categorias", "/api/categorias?entidade_id={entidade}", "/api/regras", "/api/regras?entidade_id={entidade}",
	"/api/resumo", "/api/resumo?entidade_id={entidade}",
	"/api/contas/{conta}/faturas", "/api/contas/{cartao}/faturas",
	"/api/indicadores", "/api/indicadores?entidade_id={entidade}&serie=1",
	"/api/agenda", "/api/agenda?entidade_id={entidade}&dias=365",
	"/api/recorrencias", "/api/recorrencias?entidade_id={entidade}",
	"/api/orcamento", "/api/orcamento?entidade_id={entidade}",
	"/api/metas", "/api/metas?entidade_id={entidade}",
	"/api/patrimonio", "/api/patrimonio?entidade_id={entidade}", "/api/dividas/{divida}/tabela",
	"/api/compromissos", "/api/compromissos?entidade_id={entidade}",
	"/api/open-finance", "/api/convites-conta",
	"/api/investimentos", "/api/investimentos?entidade_id={entidade}",
	"/api/investimentos/ir", "/api/investimentos/ir?entidade_id={entidade}", "/api/investimentos/ativos/{ativo}/operacoes",
	"/api/cnpj/{pj}/painel", "/api/cnpj/{pj}/notas", "/api/cnpj/{pj}/notas?ano=2026", "/api/cnpj/{pj}/folha",
	"/api/cnpj/{pj}/distribuicoes", "/api/cnpj/simulacao?receita_mensal_centavos=1000000&contador_centavos=0",
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
			slog.Info("req", "metodo", r.Method, "rota", rotaParaLog(r.URL.Path), "status", g.status, "ms", time.Since(inicio).Milliseconds())
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
		limite := int64(limiteCorpo)
		if strings.HasPrefix(r.URL.Path, "/api/importacoes") {
			limite = limiteArquivo*4/3 + limiteCorpo // arquivo em base64
		}
		r.Body = http.MaxBytesReader(w, r.Body, limite)
		h.ServeHTTP(w, r)
	})
}

// limitarAPI: teto geral de requisições por IP na API (o /api/health fica de fora, é o
// deploy e o PiControl que chamam).
func (s *Servidor) limitarAPI(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/health" {
			ip := s.ip(r)
			escrita := r.Method != http.MethodGet && r.Method != http.MethodHead
			if !s.limiteGeral.permitir(ip) || (escrita && !s.limiteEscrita.permitir(ip)) {
				w.Header().Set("Retry-After", "60")
				erroJSON(w, http.StatusTooManyRequests, "limite", "muitas requisições; aguarde um minuto")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// rotaParaLog esconde tokens que vão no caminho (links de convite).
func rotaParaLog(caminho string) string {
	if resto, ok := strings.CutPrefix(caminho, "/api/convites/"); ok {
		if _, depois, tem := strings.Cut(resto, "/"); tem {
			return "/api/convites/***/" + depois
		}
		return "/api/convites/***"
	}
	return caminho
}

func (s *Servidor) limitado(h http.Handler) http.Handler { return s.limitadoPor(s.limiteAuth, h) }

func (s *Servidor) limitadoPor(l *limitador, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.permitir(s.ip(r)) {
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
	c, err := r.Cookie(s.nomeCookie(cookieSessao))
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

// nomeCookie: com origem segura, prefixo __Host- (só vale com Secure, Path=/ e sem
// Domain). Em http na rede de casa o navegador descartaria um cookie Secure.
func (s *Servidor) nomeCookie(base string) string {
	if s.Config.Segura() {
		return "__Host-" + base
	}
	return base
}

func (s *Servidor) lerCookie(r *http.Request, base string) (*http.Cookie, error) {
	return r.Cookie(s.nomeCookie(base))
}

func (s *Servidor) gravarCookie(w http.ResponseWriter, base, valor string, duracao time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: s.nomeCookie(base), Value: valor, Path: "/", MaxAge: int(duracao.Seconds()),
		HttpOnly: true, Secure: s.Config.Segura(), SameSite: http.SameSiteStrictMode,
	})
}

func (s *Servidor) apagarCookie(w http.ResponseWriter, base string) {
	http.SetCookie(w, &http.Cookie{
		Name: s.nomeCookie(base), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.Config.Segura(), SameSite: http.SameSiteStrictMode,
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
