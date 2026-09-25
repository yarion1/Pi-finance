package auth_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/cripto"
	"github.com/yarion1/pi-finance/internal/db"
	"github.com/yarion1/pi-finance/internal/db/dbteste"
)

type relogio struct{ t time.Time }

func (r *relogio) agora() time.Time      { return r.t }
func (r *relogio) andar(d time.Duration) { r.t = r.t.Add(d) }

func novoServico(t *testing.T) (*auth.Servico, *dbteste.Banco, *relogio) {
	t.Helper()
	banco := dbteste.Novo(t)
	cif, err := cripto.Novo(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	wa, err := auth.NovoWebAuthn("https://pi.teste.ts.net:8443")
	if err != nil {
		t.Fatal(err)
	}
	r := &relogio{t: time.Now()}
	return &auth.Servico{Pool: banco.App, Cifrador: cif, WebAuthn: wa, Agora: r.agora}, banco, r
}

var origem = auth.Origem{IP: "100.64.0.1", UserAgent: "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0"}

const senha = "uma senha bem comprida"

// configurarTOTP cadastra o primeiro usuário, ativa o TOTP e devolve o segredo e o token completo.
func configurarTOTP(t *testing.T, s *auth.Servico, r *relogio, email string, convite string) (segredo, token string, codigos []string) {
	t.Helper()
	ctx := context.Background()
	parcial, err := s.Cadastrar(ctx, auth.DadosCadastro{Nome: "Pessoa", Email: email, Senha: senha, Convite: convite}, origem)
	if err != nil {
		t.Fatalf("cadastrar: %v", err)
	}
	ss, err := s.SessaoPorToken(ctx, parcial)
	if err != nil {
		t.Fatal(err)
	}
	if ss.MFAOK || !ss.PrecisaConfigurar2FA() {
		t.Fatal("sessão pós-cadastro deveria ser parcial e sem fator")
	}
	_, segredo, err = s.IniciarTOTP(ctx, ss)
	if err != nil {
		t.Fatal(err)
	}
	codigo, _ := totp.GenerateCode(segredo, r.agora())
	codigos, token, err = s.ConfirmarTOTP(ctx, ss, codigo, origem)
	if err != nil {
		t.Fatalf("confirmar TOTP: %v", err)
	}
	if len(codigos) != 10 || token == "" {
		t.Fatalf("esperava 10 códigos e token novo, veio %d e %q", len(codigos), token)
	}
	if _, err := s.SessaoPorToken(ctx, parcial); !errors.Is(err, auth.ErrSessao) {
		t.Fatal("o token parcial deveria deixar de valer após a elevação")
	}
	return segredo, token, codigos
}

func TestCadastroSoPorConvite(t *testing.T) {
	s, _, r := novoServico(t)
	ctx := context.Background()
	configurarTOTP(t, s, r, "ana@teste.com", "")

	_, err := s.Cadastrar(ctx, auth.DadosCadastro{Nome: "B", Email: "b@teste.com", Senha: senha}, origem)
	if !errors.Is(err, auth.ErrCadastroFechado) {
		t.Fatalf("segundo cadastro sem convite: %v", err)
	}
	_, err = s.Cadastrar(ctx, auth.DadosCadastro{Nome: "B", Email: "b@teste.com", Senha: senha, Convite: "inexistente"}, origem)
	if !errors.Is(err, auth.ErrConviteInvalido) {
		t.Fatalf("convite inexistente: %v", err)
	}
	_, err = s.Cadastrar(ctx, auth.DadosCadastro{Nome: "A", Email: "ANA@teste.com", Senha: senha}, origem)
	if !errors.Is(err, auth.ErrCadastroFechado) && !errors.Is(err, auth.ErrEmailEmUso) {
		t.Fatalf("e-mail repetido: %v", err)
	}
	_, err = s.Cadastrar(ctx, auth.DadosCadastro{Nome: "C", Email: "c@teste.com", Senha: "curta"}, origem)
	if !errors.Is(err, auth.ErrDadosInvalidos) {
		t.Fatalf("senha curta: %v", err)
	}
}

func TestConviteDaCasa(t *testing.T) {
	s, banco, r := novoServico(t)
	ctx := context.Background()
	_, tokenA, _ := configurarTOTP(t, s, r, "ana@teste.com", "")
	ssA, _ := s.SessaoPorToken(ctx, tokenA)

	convite := auth.NovoToken()
	var casa string
	err := db.ComUsuario(ctx, banco.App, ssA.UsuarioID, func(tx pgxTx) error {
		if err := tx.QueryRow(ctx, "select criar_casa('Casa')").Scan(&casa); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `insert into convites_casa (casa_id, papel, token_hash, criado_por, expira_em)
			values ($1, 'membro', $2, $3, now() + interval '48 hours')`, casa, auth.HashToken(convite), ssA.UsuarioID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, tokenB, _ := configurarTOTP(t, s, r, "bruno@teste.com", convite)
	ssB, _ := s.SessaoPorToken(ctx, tokenB)
	var n int
	_ = db.ComUsuario(ctx, banco.App, ssB.UsuarioID, func(tx pgxTx) error {
		return tx.QueryRow(ctx, "select count(*) from casas where id = $1", casa).Scan(&n)
	})
	if n != 1 {
		t.Fatal("B deveria ter entrado na casa pelo convite")
	}
	_, err = s.Cadastrar(ctx, auth.DadosCadastro{Nome: "C", Email: "c@teste.com", Senha: senha, Convite: convite}, origem)
	if !errors.Is(err, auth.ErrConviteInvalido) {
		t.Fatalf("convite reutilizado: %v", err)
	}
}

func TestLoginComSegundoFator(t *testing.T) {
	s, _, r := novoServico(t)
	ctx := context.Background()
	segredo, _, codigos := configurarTOTP(t, s, r, "ana@teste.com", "")

	if _, err := s.Entrar(ctx, "ana@teste.com", "senha errada qualquer", origem); !errors.Is(err, auth.ErrCredenciais) {
		t.Fatalf("senha errada: %v", err)
	}
	if _, err := s.Entrar(ctx, "ninguem@teste.com", senha, origem); !errors.Is(err, auth.ErrCredenciais) {
		t.Fatalf("e-mail inexistente: %v", err)
	}

	r.andar(time.Minute) // sai do passo TOTP usado na configuração
	parcial, err := s.Entrar(ctx, "Ana@Teste.com", senha, origem)
	if err != nil {
		t.Fatal(err)
	}
	ss, _ := s.SessaoPorToken(ctx, parcial)
	if ss.MFAOK || ss.PodeConfigurar() {
		t.Fatal("sessão parcial de quem já tem TOTP não pode configurar fatores")
	}
	if _, _, err := s.IniciarTOTP(ctx, ss); !errors.Is(err, auth.ErrSegundoFator) {
		t.Fatalf("trocar TOTP só com senha: %v", err)
	}
	if _, _, err := s.IniciarRegistroPasskey(ctx, ss); !errors.Is(err, auth.ErrSegundoFator) {
		t.Fatalf("registrar passkey só com senha: %v", err)
	}

	codigo, _ := totp.GenerateCode(segredo, r.agora())
	completo, err := s.VerificarSegundoFator(ctx, ss, codigo, origem)
	if err != nil {
		t.Fatalf("TOTP válido: %v", err)
	}
	ssC, err := s.SessaoPorToken(ctx, completo)
	if err != nil || !ssC.MFAOK {
		t.Fatal("sessão deveria estar completa")
	}
	if d := ssC.ExpiraEm.Sub(r.agora()); d > 12*time.Hour+time.Minute || d < 11*time.Hour {
		t.Fatalf("sessão de navegador deveria durar 12 h, dura %v", d)
	}

	// o mesmo código não serve duas vezes
	parcial2, _ := s.Entrar(ctx, "ana@teste.com", senha, origem)
	ss2, _ := s.SessaoPorToken(ctx, parcial2)
	if _, err := s.VerificarSegundoFator(ctx, ss2, codigo, origem); !errors.Is(err, auth.ErrCodigo) {
		t.Fatalf("replay do código TOTP: %v", err)
	}
	// código de recuperação: vale uma vez
	if _, err := s.VerificarSegundoFator(ctx, ss2, codigos[0], origem); err != nil {
		t.Fatalf("código de recuperação: %v", err)
	}
	parcial3, _ := s.Entrar(ctx, "ana@teste.com", senha, origem)
	ss3, _ := s.SessaoPorToken(ctx, parcial3)
	if _, err := s.VerificarSegundoFator(ctx, ss3, codigos[0], origem); !errors.Is(err, auth.ErrCodigo) {
		t.Fatalf("código de recuperação reutilizado: %v", err)
	}
}

func TestSessaoCelularDuraSeteDias(t *testing.T) {
	s, _, r := novoServico(t)
	ctx := context.Background()
	segredo, _, _ := configurarTOTP(t, s, r, "ana@teste.com", "")
	r.andar(time.Minute)
	cel := auth.Origem{IP: "100.64.0.2", UserAgent: "Mozilla/5.0 (Linux; Android 15) Mobile Safari"}
	parcial, _ := s.Entrar(ctx, "ana@teste.com", senha, cel)
	ss, _ := s.SessaoPorToken(ctx, parcial)
	codigo, _ := totp.GenerateCode(segredo, r.agora())
	tok, err := s.VerificarSegundoFator(ctx, ss, codigo, cel)
	if err != nil {
		t.Fatal(err)
	}
	ss, _ = s.SessaoPorToken(ctx, tok)
	if d := ss.ExpiraEm.Sub(r.agora()); d < 6*24*time.Hour {
		t.Fatalf("sessão de celular deveria durar 7 dias, dura %v", d)
	}
	r.andar(7*24*time.Hour + time.Minute)
	if _, err := s.SessaoPorToken(ctx, tok); !errors.Is(err, auth.ErrSessao) {
		t.Fatal("sessão vencida deveria ser recusada")
	}
}

func TestBloqueioProgressivo(t *testing.T) {
	s, banco, r := novoServico(t)
	ctx := context.Background()
	configurarTOTP(t, s, r, "ana@teste.com", "")

	for i := 0; i < 5; i++ {
		if _, err := s.Entrar(ctx, "ana@teste.com", "errada errada errada", origem); !errors.Is(err, auth.ErrCredenciais) {
			t.Fatalf("tentativa %d: %v", i+1, err)
		}
	}
	var bloq auth.ErrBloqueado
	if _, err := s.Entrar(ctx, "ana@teste.com", senha, origem); !errors.As(err, &bloq) {
		t.Fatalf("após 5 erros deveria bloquear mesmo com a senha certa, veio %v", err)
	}
	r.andar(16 * time.Minute)
	if _, err := s.Entrar(ctx, "ana@teste.com", senha, origem); err != nil {
		t.Fatalf("depois do bloqueio a senha certa volta a valer: %v", err)
	}
	var falhas int
	_ = banco.Dono.QueryRow(ctx, "select tentativas_falhas from usuarios").Scan(&falhas)
	if falhas != 0 {
		t.Fatalf("login certo deveria zerar as falhas, restaram %d", falhas)
	}

	if auth.TempoBloqueio(5) != 15*time.Minute || auth.TempoBloqueio(6) != 30*time.Minute ||
		auth.TempoBloqueio(7) != time.Hour || auth.TempoBloqueio(30) != 24*time.Hour || auth.TempoBloqueio(4) != 0 {
		t.Fatal("tabela de bloqueio progressivo")
	}
}

func TestSenha(t *testing.T) {
	h, err := auth.HashSenha("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := auth.ConferirSenha("correct horse battery staple", h); !ok {
		t.Fatal("senha certa")
	}
	if ok, _ := auth.ConferirSenha("correct horse battery stapl", h); ok {
		t.Fatal("senha errada")
	}
	if _, err := auth.ConferirSenha("x", "$bcrypt$lixo"); err == nil {
		t.Fatal("hash inválido")
	}
}

type pgxTx = pgx.Tx
