package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/core"
)

const contextoDocumento = "entidades.documento"

type entidade struct {
	ID           string  `json:"id"`
	Tipo         string  `json:"tipo"`
	Nome         string  `json:"nome"`
	Regime       *string `json:"regime"`
	Anexo        *string `json:"anexo"`
	CNAE         *string `json:"cnae"`
	Municipio    *string `json:"municipio"`
	DataAbertura *string `json:"data_abertura"`
	Papel        string  `json:"papel"`
	Documento    *string `json:"documento"` // sempre mascarado
}

type acesso struct {
	UsuarioID string    `json:"usuario_id"`
	Nome      string    `json:"nome"`
	Email     string    `json:"email"`
	Papel     string    `json:"papel"`
	CriadoEm  time.Time `json:"criado_em"`
}

const colunasEntidade = `id, tipo::text, nome, regime::text, anexo, cnae, municipio,
	to_char(data_abertura, 'YYYY-MM-DD'), app_papel_entidade(id), documento_cifrado`

func (s *Servidor) lerEntidade(row pgx.CollectableRow) (entidade, error) {
	var e entidade
	var doc *string
	if err := row.Scan(&e.ID, &e.Tipo, &e.Nome, &e.Regime, &e.Anexo, &e.CNAE, &e.Municipio, &e.DataAbertura, &e.Papel, &doc); err != nil {
		return e, err
	}
	if doc != nil {
		claro, err := s.Cifrador.Decifrar(*doc, contextoDocumento)
		if err != nil {
			return e, err
		}
		m := core.MascararDocumento(claro)
		e.Documento = &m
	}
	return e, nil
}

func (s *Servidor) listarEntidades(w http.ResponseWriter, r *http.Request) {
	var lista []entidade
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, "select "+colunasEntidade+" from entidades order by tipo, nome")
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, s.lerEntidade)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

type dadosEntidade struct {
	Tipo         string  `json:"tipo"`
	Nome         string  `json:"nome"`
	Documento    *string `json:"documento"`
	Regime       *string `json:"regime"`
	Anexo        *string `json:"anexo"`
	CNAE         *string `json:"cnae"`
	Municipio    *string `json:"municipio"`
	DataAbertura *string `json:"data_abertura"`
}

// normalizar valida os campos e devolve o documento já cifrado (ou nil).
func (d *dadosEntidade) normalizar(c interface {
	Cifrar(string, string) (string, error)
}) (*string, error) {
	if d.Tipo != "PF" && d.Tipo != "PJ" {
		return nil, invalido("tipo: PF ou PJ")
	}
	nome, err := validarNome(d.Nome)
	if err != nil {
		return nil, err
	}
	d.Nome = nome
	vazio := func(p **string) {
		if *p != nil && strings.TrimSpace(**p) == "" {
			*p = nil
		}
	}
	vazio(&d.Documento)
	vazio(&d.Regime)
	vazio(&d.Anexo)
	vazio(&d.CNAE)
	vazio(&d.Municipio)
	vazio(&d.DataAbertura)
	if d.Tipo == "PF" && (d.Regime != nil || d.Anexo != nil || d.CNAE != nil) {
		return nil, invalido("regime, anexo e CNAE são só de PJ")
	}
	if d.Regime != nil && *d.Regime != "MEI" && *d.Regime != "SIMPLES_ME" && *d.Regime != "SIMPLES_EPP" {
		return nil, invalido("regime: MEI, SIMPLES_ME ou SIMPLES_EPP")
	}
	if d.Anexo != nil && *d.Anexo != "III" && *d.Anexo != "V" {
		return nil, invalido("anexo: III ou V")
	}
	if d.DataAbertura != nil {
		if _, err := time.Parse("2006-01-02", *d.DataAbertura); err != nil {
			return nil, invalido("data de abertura inválida")
		}
	}
	if d.Documento == nil {
		return nil, nil
	}
	doc := core.SoDigitos(*d.Documento)
	if (d.Tipo == "PF" && !core.CPFValido(doc)) || (d.Tipo == "PJ" && !core.CNPJValido(doc)) {
		return nil, invalido("CPF ou CNPJ inválido")
	}
	cifrado, err := c.Cifrar(doc, contextoDocumento)
	return &cifrado, err
}

func (s *Servidor) criarEntidade(w http.ResponseWriter, r *http.Request) {
	var d dadosEntidade
	if !lerJSON(w, r, &d) {
		return
	}
	doc, err := d.normalizar(s.Cifrador)
	if err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `insert into entidades (dono_id, tipo, nome, documento_cifrado, regime, anexo, cnae, municipio, data_abertura)
			values (app_usuario_id(), $1, $2, $3, $4, $5, $6, $7, $8) returning id`,
			d.Tipo, d.Nome, doc, d.Regime, d.Anexo, d.CNAE, d.Municipio, d.DataAbertura).Scan(&id)
		if err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "entidade_criada", id, map[string]any{"tipo": d.Tipo})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) verEntidade(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		entidade
		Acessos []acesso `json:"acessos"`
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, "select "+colunasEntidade+" from entidades where id = $1", r.PathValue("id"))
		if err != nil {
			return err
		}
		if resp.entidade, err = pgx.CollectExactlyOneRow(linhas, s.lerEntidade); err != nil {
			return err
		}
		// RLS: o dono vê todos os acessos; os outros, só o próprio
		linhas, err = tx.Query(ctx, `select u.id, u.nome, u.email, a.papel::text, a.criado_em from acessos_entidade a
			join usuarios u on u.id = a.usuario_id where a.entidade_id = $1 order by a.criado_em`, resp.ID)
		if err != nil {
			return err
		}
		resp.Acessos, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[acesso])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

func (s *Servidor) editarEntidade(w http.ResponseWriter, r *http.Request) {
	var d dadosEntidade
	if !lerJSON(w, r, &d) {
		return
	}
	doc, err := d.normalizar(s.Cifrador)
	if err != nil {
		falhar(w, r, err)
		return
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		// documento ausente mantém o atual
		err := exigirUma(tx.Exec(ctx, `update entidades set tipo = $2, nome = $3, documento_cifrado = coalesce($4, documento_cifrado),
			regime = $5, anexo = $6, cnae = $7, municipio = $8, data_abertura = $9 where id = $1`,
			r.PathValue("id"), d.Tipo, d.Nome, doc, d.Regime, d.Anexo, d.CNAE, d.Municipio, d.DataAbertura))
		if err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "entidade_editada", r.PathValue("id"), nil)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarEntidade(w http.ResponseWriter, r *http.Request) {
	if !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := exigirUma(tx.Exec(ctx, "delete from entidades where id = $1", r.PathValue("id"))); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "entidade_apagada", r.PathValue("id"), nil)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) darAcesso(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Email string `json:"email"`
		Papel string `json:"papel"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if c.Papel != "membro" && c.Papel != "leitor" && c.Papel != "contador" {
		falhar(w, r, invalido("papel: membro, leitor ou contador"))
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var usuarioID string
		err := tx.QueryRow(ctx, "select id from usuarios where lower(email) = lower(trim($1))", c.Email).Scan(&usuarioID)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalido("nenhuma conta com este e-mail; convide a pessoa para a casa antes")
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `insert into acessos_entidade (entidade_id, usuario_id, papel) values ($1, $2, $3)
			on conflict (entidade_id, usuario_id) do update set papel = excluded.papel`,
			r.PathValue("id"), usuarioID, c.Papel); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "acesso_concedido", r.PathValue("id"), map[string]any{"usuario": usuarioID, "papel": c.Papel})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) removerAcesso(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		err := exigirUma(tx.Exec(ctx, "delete from acessos_entidade where entidade_id = $1 and usuario_id = $2",
			r.PathValue("id"), r.PathValue("usuario")))
		if err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "acesso_removido", r.PathValue("id"), map[string]any{"usuario": r.PathValue("usuario")})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
