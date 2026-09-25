package db_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yarion1/pi-finance/internal/db"
	"github.com/yarion1/pi-finance/internal/db/dbteste"
)

// Tabelas sem RLS de propósito: autenticação (acessadas só pelo código de login,
// nunca por usuário) e operação. Qualquer tabela nova fora desta lista precisa de RLS.
var semRLS = map[string]bool{
	"usuarios": true, "sessoes": true, "dois_fatores": true, "codigos_recuperacao": true,
	"passkeys": true, "desafios_webauthn": true, "sinais_vida": true, "goose_db_version": true,
}

func TestToda_tabela_tem_RLS(t *testing.T) {
	b := dbteste.Novo(t)
	ctx := context.Background()

	linhas, err := b.Dono.Query(ctx, `
		select c.relname, c.relrowsecurity, pg_get_userbyid(c.relowner)
		from pg_class c join pg_namespace n on n.oid = c.relnamespace
		where n.nspname = 'public' and c.relkind = 'r'`)
	if err != nil {
		t.Fatal(err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var nome, dono string
		var rls bool
		if err := linhas.Scan(&nome, &rls, &dono); err != nil {
			t.Fatal(err)
		}
		if dono == db.PapelApp {
			t.Errorf("%s pertence ao papel do app; RLS não valeria para ele", nome)
		}
		if semRLS[nome] || strings.HasPrefix(nome, "river_") {
			continue
		}
		if !rls {
			t.Errorf("tabela %s sem row level security", nome)
		}
	}

	var super, bypass bool
	if err := b.Dono.QueryRow(ctx, "select rolsuper, rolbypassrls from pg_roles where rolname = $1", db.PapelApp).
		Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if super || bypass {
		t.Fatalf("papel do app com superuser=%v bypassrls=%v", super, bypass)
	}
}

// cenario prepara: A com PF (conta privada, conta só saldo, conta compartilhada) e PJ;
// B membro da casa de A; C contador da PJ de A; D sem relação nenhuma.
type cenario struct {
	a, b, c, d                      string
	casa                            string
	pfA, pjA                        string
	privada, soSaldo, compartilhada string
	contaPJ                         string
}

func montar(t *testing.T, banco *dbteste.Banco) cenario {
	t.Helper()
	ctx := context.Background()
	s := cenario{
		a: banco.CriarUsuario(t, "Ana", "a@teste"),
		b: banco.CriarUsuario(t, "Bruno", "b@teste"),
		c: banco.CriarUsuario(t, "Carla (contadora)", "c@teste"),
		d: banco.CriarUsuario(t, "Davi", "d@teste"),
	}
	token := sha256.Sum256([]byte("convite-b"))

	comoA := func(fn func(pgx.Tx) error) {
		t.Helper()
		if err := db.ComUsuario(ctx, banco.App, s.a, fn); err != nil {
			t.Fatalf("preparo como A: %v", err)
		}
	}
	comoA(func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "select criar_casa('Casa da Ana')").Scan(&s.casa); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into convites_casa (casa_id, papel, token_hash, criado_por, expira_em)
			values ($1, 'membro', $2, $3, now() + interval '48 hours')`, s.casa, token[:], s.a); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "insert into entidades (dono_id, tipo, nome) values ($1, 'PF', 'Ana PF') returning id", s.a).Scan(&s.pfA); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "insert into entidades (dono_id, tipo, nome, regime) values ($1, 'PJ', 'Ana MEI', 'MEI') returning id", s.a).Scan(&s.pjA); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "insert into acessos_entidade (entidade_id, usuario_id, papel) values ($1, $2, 'contador')", s.pjA, s.c); err != nil {
			return err
		}
		contas := []struct {
			dest          *string
			entidade, vis string
			casa          *string
		}{
			{&s.privada, s.pfA, "privada", nil},
			{&s.soSaldo, s.pfA, "saldo", &s.casa},
			{&s.compartilhada, s.pfA, "compartilhada", &s.casa},
			{&s.contaPJ, s.pjA, "privada", nil},
		}
		for _, c := range contas {
			if err := tx.QueryRow(ctx, `insert into contas (entidade_id, nome, tipo, visibilidade, casa_id)
				values ($1, $2, 'corrente', $3, $4) returning id`, c.entidade, "conta "+c.vis, c.vis, c.casa).Scan(c.dest); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao, valor_centavos, tipo)
				values ($1, $2, current_date, $3, $3, -4500, 'gasto')`, c.entidade, *c.dest, "SEGREDO-"+c.vis+"-"+c.entidade); err != nil {
				return err
			}
		}
		return nil
	})
	if err := db.ComUsuario(ctx, banco.App, s.b, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "select aceitar_convite($1)", token[:])
		return err
	}); err != nil {
		t.Fatalf("B aceitar convite: %v", err)
	}
	return s
}

func contar(t *testing.T, banco *dbteste.Banco, usuario, consulta string, args ...any) int {
	t.Helper()
	var n int
	err := db.ComUsuario(context.Background(), banco.App, usuario, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), consulta, args...).Scan(&n)
	})
	if err != nil {
		t.Fatalf("%s: %v", consulta, err)
	}
	return n
}

func TestIsolamento_entre_usuarios(t *testing.T) {
	banco := dbteste.Novo(t)
	s := montar(t, banco)

	t.Run("A vê tudo o que é dela", func(t *testing.T) {
		if n := contar(t, banco, s.a, "select count(*) from transacoes"); n != 4 {
			t.Fatalf("A vê %d transações, esperado 4", n)
		}
	})

	t.Run("D, sem relação, não vê nada", func(t *testing.T) {
		for _, tabela := range []string{"entidades", "contas", "transacoes", "casas", "membros_casa", "convites_casa", "acessos_entidade"} {
			if n := contar(t, banco, s.d, "select count(*) from "+tabela); n != 0 {
				t.Errorf("D vê %d linhas de %s", n, tabela)
			}
		}
	})

	t.Run("sem usuário na sessão ninguém vê nada", func(t *testing.T) {
		var n int
		if err := banco.App.QueryRow(context.Background(), "select count(*) from transacoes").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("sem usuário: %d transações visíveis", n)
		}
	})

	t.Run("membro da casa: privada não, só saldo sem transações, compartilhada inteira", func(t *testing.T) {
		if n := contar(t, banco, s.b, "select count(*) from contas where id = $1", s.privada); n != 0 {
			t.Error("B vê a conta privada de A")
		}
		if n := contar(t, banco, s.b, "select count(*) from contas where id = $1", s.soSaldo); n != 1 {
			t.Error("B deveria ver a conta só saldo")
		}
		if n := contar(t, banco, s.b, "select count(*) from transacoes where conta_id = $1", s.soSaldo); n != 0 {
			t.Error("B vê transações da conta só saldo")
		}
		if n := contar(t, banco, s.b, "select count(*) from transacoes where conta_id = $1", s.compartilhada); n != 1 {
			t.Error("B deveria ver as transações da conta compartilhada")
		}
		if n := contar(t, banco, s.b, "select count(*) from transacoes"); n != 1 {
			t.Errorf("B vê %d transações no total, esperado 1", n)
		}
		if n := contar(t, banco, s.b, "select count(*) from entidades"); n != 0 {
			t.Error("B vê entidades de A")
		}
	})

	t.Run("contador vê só a PJ", func(t *testing.T) {
		if n := contar(t, banco, s.c, "select count(*) from entidades"); n != 1 {
			t.Errorf("contador vê %d entidades", n)
		}
		if n := contar(t, banco, s.c, "select count(*) from transacoes where entidade_id <> $1", s.pjA); n != 0 {
			t.Error("contador vê transações da PF")
		}
		if n := contar(t, banco, s.c, "select count(*) from transacoes where entidade_id = $1", s.pjA); n != 1 {
			t.Error("contador deveria ver as transações da PJ")
		}
	})

	t.Run("ninguém além de A altera dados de A", func(t *testing.T) {
		for _, u := range []string{s.b, s.c, s.d} {
			err := db.ComUsuario(context.Background(), banco.App, u, func(tx pgx.Tx) error {
				for _, cmd := range []string{
					"update transacoes set valor_centavos = 1",
					"delete from transacoes",
					"update contas set nome = 'x'",
					"delete from contas",
					"update entidades set nome = 'x'",
					"delete from entidades",
				} {
					tag, err := tx.Exec(context.Background(), cmd)
					if err != nil {
						return err
					}
					if tag.RowsAffected() != 0 {
						t.Errorf("usuário %s alterou %d linhas com %q", u, tag.RowsAffected(), cmd)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		if n := contar(t, banco, s.a, "select count(*) from transacoes where valor_centavos = -4500"); n != 4 {
			t.Fatalf("transações de A alteradas: restaram %d intactas", n)
		}
	})

	t.Run("inserções proibidas", func(t *testing.T) {
		casos := []struct {
			nome, usuario, sql string
			args               []any
		}{
			{"B lança na conta compartilhada de A", s.b,
				`insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao, valor_centavos, tipo)
				 values ($1, $2, current_date, 'x', 'x', 1, 'gasto')`, []any{s.pfA, s.compartilhada}},
			{"contador lança na PJ", s.c,
				`insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao, valor_centavos, tipo)
				 values ($1, $2, current_date, 'x', 'x', 1, 'gasto')`, []any{s.pjA, s.contaPJ}},
			{"D cria entidade em nome de A", s.d,
				"insert into entidades (dono_id, tipo, nome) values ($1, 'PF', 'falsa')", []any{s.a}},
			{"B se dá acesso à PF de A", s.b,
				"insert into acessos_entidade (entidade_id, usuario_id, papel) values ($1, $2, 'membro')", []any{s.pfA, s.b}},
			{"B adiciona D à casa", s.b,
				"insert into membros_casa (casa_id, usuario_id, papel) values ($1, $2, 'membro')", []any{s.casa, s.d}},
			{"B cria convite para a casa (não é dono)", s.b,
				`insert into convites_casa (casa_id, papel, token_hash, criado_por, expira_em)
				 values ($1, 'membro', '\x00', $2, now() + interval '1 day')`, []any{s.casa, s.b}},
			{"D cria conta compartilhando com casa alheia", s.d,
				"insert into contas (entidade_id, nome, tipo, visibilidade, casa_id) values ($1, 'x', 'corrente', 'compartilhada', $2)", []any{s.pfA, s.casa}},
		}
		for _, c := range casos {
			err := db.ComUsuario(context.Background(), banco.App, c.usuario, func(tx pgx.Tx) error {
				_, err := tx.Exec(context.Background(), c.sql, c.args...)
				return err
			})
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
				t.Errorf("%s: esperado erro de RLS (42501), veio %v", c.nome, err)
			}
		}
	})

	t.Run("quem sai da casa leva as contas compartilhadas", func(t *testing.T) {
		ctx := context.Background()
		var pfB, contaB string
		err := db.ComUsuario(ctx, banco.App, s.b, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, "insert into entidades (dono_id, tipo, nome) values ($1, 'PF', 'Bruno PF') returning id", s.b).Scan(&pfB); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `insert into contas (entidade_id, nome, tipo, visibilidade, casa_id)
				values ($1, 'conta do Bruno', 'corrente', 'compartilhada', $2) returning id`, pfB, s.casa).Scan(&contaB); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `insert into transacoes (entidade_id, conta_id, data, descricao_original, descricao, valor_centavos, tipo)
				values ($1, $2, current_date, 'mercado', 'mercado', -1000, 'gasto')`, pfB, contaB)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := contar(t, banco, s.a, "select count(*) from transacoes where conta_id = $1", contaB); n != 1 {
			t.Fatal("A deveria ver a conta que B compartilhou")
		}
		if err := db.ComUsuario(ctx, banco.App, s.b, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "delete from membros_casa where usuario_id = $1", s.b)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if n := contar(t, banco, s.a, "select count(*) from contas where id = $1", contaB); n != 0 {
			t.Fatal("A continua vendo a conta de B depois que ele saiu da casa")
		}
		if n := contar(t, banco, s.b, "select count(*) from transacoes where conta_id = $1", s.compartilhada); n != 0 {
			t.Fatal("B continua vendo a conta de A depois que saiu da casa")
		}
	})

	t.Run("contador só em PJ", func(t *testing.T) {
		err := db.ComUsuario(context.Background(), banco.App, s.a, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), "insert into acessos_entidade (entidade_id, usuario_id, papel) values ($1, $2, 'contador')", s.pfA, s.d)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "contador") {
			t.Fatalf("contador em PF deveria falhar, veio %v", err)
		}
	})

	t.Run("a casa não fica sem dono", func(t *testing.T) {
		err := db.ComUsuario(context.Background(), banco.App, s.a, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), "delete from membros_casa where usuario_id = $1", s.a)
			return err
		})
		if err == nil || !strings.Contains(err.Error(), "dono") {
			t.Fatalf("remover o único dono deveria falhar, veio %v", err)
		}
	})

	t.Run("convite é de uso único", func(t *testing.T) {
		token := sha256.Sum256([]byte("convite-b"))
		err := db.ComUsuario(context.Background(), banco.App, s.d, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), "select aceitar_convite($1)", token[:])
			return err
		})
		if err == nil {
			t.Fatal("D reutilizou o convite de B")
		}
		if n := contar(t, banco, s.d, "select count(*) from casas"); n != 0 {
			t.Fatal("D entrou na casa")
		}
	})

	t.Run("referência fiscal é só leitura", func(t *testing.T) {
		err := db.ComUsuario(context.Background(), banco.App, s.a, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(), "update regras_fiscais set fonte = 'x'")
			return err
		})
		if err == nil {
			t.Fatal("app alterou regras fiscais")
		}
		if n := contar(t, banco, s.d, "select count(*) from regras_fiscais where chave = 'teto_mei'"); n != 1 {
			t.Fatal("regras fiscais deveriam ser visíveis a todos")
		}
	})
}
