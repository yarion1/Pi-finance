package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/db"
)

// comUsuario roda fn com o usuário da sessão aplicado ao RLS.
func (s *Servidor) comUsuario(r *http.Request, fn func(context.Context, pgx.Tx) error) error {
	ctx := r.Context()
	return db.ComUsuario(ctx, s.Pool, sessaoDe(r).UsuarioID, func(tx pgx.Tx) error { return fn(ctx, tx) })
}

func (s *Servidor) auditar(ctx context.Context, tx pgx.Tx, r *http.Request, acao, alvo string, dados map[string]any) error {
	return auth.Auditar(ctx, tx, sessaoDe(r).UsuarioID, acao, alvo, s.origem(r), dados)
}

type casa struct {
	ID        string `json:"id"`
	Nome      string `json:"nome"`
	MoedaBase string `json:"moeda_base"`
	Papel     string `json:"papel"`
}

type membro struct {
	UsuarioID string    `json:"usuario_id"`
	Nome      string    `json:"nome"`
	Email     string    `json:"email"`
	Papel     string    `json:"papel"`
	EntrouEm  time.Time `json:"entrou_em"`
}

type convitePendente struct {
	ID       string    `json:"id"`
	Papel    string    `json:"papel"`
	CriadoEm time.Time `json:"criado_em"`
	ExpiraEm time.Time `json:"expira_em"`
}

func validarNome(nome string) (string, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" || len([]rune(nome)) > 100 {
		return "", invalido("nome precisa ter de 1 a 100 caracteres")
	}
	return nome, nil
}

func (s *Servidor) listarCasas(w http.ResponseWriter, r *http.Request) {
	var casas []casa
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, `select c.id, c.nome, c.moeda_base, m.papel::text from casas c
			join membros_casa m on m.casa_id = c.id and m.usuario_id = app_usuario_id() order by c.nome`)
		if err != nil {
			return err
		}
		casas, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[casa])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, casas)
}

func (s *Servidor) criarCasa(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nome string `json:"nome"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	nome, err := validarNome(c.Nome)
	if err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "select criar_casa($1)", nome).Scan(&id); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "casa_criada", id, nil)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) verCasa(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		casa
		Membros  []membro          `json:"membros"`
		Convites []convitePendente `json:"convites"`
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `select c.id, c.nome, c.moeda_base, m.papel::text from casas c
			join membros_casa m on m.casa_id = c.id and m.usuario_id = app_usuario_id() where c.id = $1`,
			r.PathValue("id")).Scan(&resp.ID, &resp.Nome, &resp.MoedaBase, &resp.Papel)
		if err != nil {
			return err
		}
		linhas, err := tx.Query(ctx, `select u.id, u.nome, u.email, m.papel::text, m.entrou_em from membros_casa m
			join usuarios u on u.id = m.usuario_id where m.casa_id = $1 order by m.entrou_em`, resp.ID)
		if err != nil {
			return err
		}
		if resp.Membros, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[membro]); err != nil {
			return err
		}
		// RLS só devolve convites ao dono
		linhas, err = tx.Query(ctx, `select id, papel::text, criado_em, expira_em from convites_casa
			where casa_id = $1 and aceito_em is null and expira_em > now() order by criado_em`, resp.ID)
		if err != nil {
			return err
		}
		resp.Convites, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[convitePendente])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

func (s *Servidor) renomearCasa(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nome string `json:"nome"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	nome, err := validarNome(c.Nome)
	if err != nil {
		falhar(w, r, err)
		return
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "update casas set nome = $2 where id = $1", r.PathValue("id"), nome))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func papelCasaValido(p string) bool { return p == "membro" || p == "leitor" || p == "dono" }

func (s *Servidor) criarConvite(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Papel string `json:"papel"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if c.Papel != "membro" && c.Papel != "leitor" {
		falhar(w, r, invalido("papel do convite: membro ou leitor"))
		return
	}
	token := auth.NovoToken()
	expira := time.Now().Add(auth.ValidadeConvite)
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `insert into convites_casa (casa_id, papel, token_hash, criado_por, expira_em)
			values ($1, $2, $3, app_usuario_id(), $4) returning id`,
			r.PathValue("id"), c.Papel, auth.HashToken(token), expira).Scan(&id)
		if err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "convite_criado", r.PathValue("id"), map[string]any{"papel": c.Papel, "convite": id})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]any{
		"link": s.Config.URLPublica + "/convite/" + token, "expira_em": expira,
	})
}

func (s *Servidor) cancelarConvite(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "delete from convites_casa where id = $1 and casa_id = $2 and aceito_em is null",
			r.PathValue("convite"), r.PathValue("id")))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) mudarPapelMembro(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Papel string `json:"papel"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if !papelCasaValido(c.Papel) {
		falhar(w, r, invalido("papel: dono, membro ou leitor"))
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		err := exigirUma(tx.Exec(ctx, "update membros_casa set papel = $3 where casa_id = $1 and usuario_id = $2",
			r.PathValue("id"), r.PathValue("usuario"), c.Papel))
		if err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "permissao_alterada", r.PathValue("id"),
			map[string]any{"membro": r.PathValue("usuario"), "papel": c.Papel})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) removerMembro(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		err := exigirUma(tx.Exec(ctx, "delete from membros_casa where casa_id = $1 and usuario_id = $2",
			r.PathValue("id"), r.PathValue("usuario")))
		if err != nil {
			return err
		}
		// as contas que o membro compartilhava somem para a casa pela política RLS
		return s.auditar(ctx, tx, r, "membro_removido", r.PathValue("id"), map[string]any{"membro": r.PathValue("usuario")})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) verConvite(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		Casa     string    `json:"casa"`
		Papel    string    `json:"papel"`
		ExpiraEm time.Time `json:"expira_em"`
		Valido   bool      `json:"valido"`
	}
	err := s.Pool.QueryRow(r.Context(), "select casa_nome, papel::text, expira_em, valido from consultar_convite($1)",
		auth.HashToken(r.PathValue("token"))).Scan(&resp.Casa, &resp.Papel, &resp.ExpiraEm, &resp.Valido)
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

func (s *Servidor) aceitarConvite(w http.ResponseWriter, r *http.Request) {
	var casaID string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "select aceitar_convite($1)", auth.HashToken(r.PathValue("token"))).Scan(&casaID); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "convite_aceito", casaID, nil)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]string{"casa_id": casaID})
}

// exigirUma transforma "nenhuma linha afetada" (inexistente ou barrado pelo RLS) em 404.
func exigirUma(tag interface{ RowsAffected() int64 }, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNaoEncontrado
	}
	return nil
}
