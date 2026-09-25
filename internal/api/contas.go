package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

type conta struct {
	ID            string  `json:"id"`
	EntidadeID    string  `json:"entidade_id"`
	EntidadeNome  *string `json:"entidade_nome"` // nulo quando a conta vem da casa
	Dono          *string `json:"dono"`
	InstituicaoID *string `json:"instituicao_id"`
	Instituicao   *string `json:"instituicao"`
	Nome          string  `json:"nome"`
	Tipo          string  `json:"tipo"`
	Moeda         string  `json:"moeda"`
	Visibilidade  string  `json:"visibilidade"`
	CasaID        *string `json:"casa_id"`
	SaldoInicial  int64   `json:"saldo_inicial_centavos"`
	Saldo         *int64  `json:"saldo_centavos"`
	Fechamento    *int16  `json:"fechamento"`
	Vencimento    *int16  `json:"vencimento"`
	Limite        *int64  `json:"limite_centavos"`
	Arquivada     bool    `json:"arquivada"`
	PodeEditar    bool    `json:"pode_editar"`
}

const consultaContas = `select c.id, c.entidade_id, e.nome, app_dono_conta(c.id), c.instituicao_id, i.nome, c.nome,
	c.tipo::text, c.moeda, c.visibilidade::text, c.casa_id, c.saldo_inicial_centavos, app_saldo_conta(c.id),
	c.fechamento, c.vencimento, c.limite_centavos, c.arquivada, app_pode_editar_entidade(c.entidade_id)
	from contas c
	left join entidades e on e.id = c.entidade_id
	left join instituicoes i on i.id = c.instituicao_id`

func (s *Servidor) listarContas(w http.ResponseWriter, r *http.Request) {
	var lista []conta
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		consulta, args := consultaContas, []any{}
		if e := r.URL.Query().Get("entidade_id"); e != "" {
			consulta += " where c.entidade_id = $1"
			args = append(args, e)
		}
		linhas, err := tx.Query(ctx, consulta+" order by c.arquivada, e.nome nulls last, c.nome", args...)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[conta])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

type dadosConta struct {
	EntidadeID    string  `json:"entidade_id"`
	InstituicaoID *string `json:"instituicao_id"`
	Nome          string  `json:"nome"`
	Tipo          string  `json:"tipo"`
	Moeda         string  `json:"moeda"`
	Visibilidade  string  `json:"visibilidade"`
	CasaID        *string `json:"casa_id"`
	SaldoInicial  int64   `json:"saldo_inicial_centavos"`
	Fechamento    *int16  `json:"fechamento"`
	Vencimento    *int16  `json:"vencimento"`
	Limite        *int64  `json:"limite_centavos"`
	Arquivada     bool    `json:"arquivada"`
}

var tiposConta = map[string]bool{"corrente": true, "poupanca": true, "carteira": true, "dinheiro": true,
	"beneficio": true, "investimento": true, "cartao": true}

func (d *dadosConta) validar() error {
	nome, err := validarNome(d.Nome)
	if err != nil {
		return err
	}
	d.Nome = nome
	if !tiposConta[d.Tipo] {
		return invalido("tipo de conta inválido")
	}
	d.Moeda = strings.ToUpper(strings.TrimSpace(d.Moeda))
	if d.Moeda == "" {
		d.Moeda = "BRL"
	}
	if len(d.Moeda) != 3 {
		return invalido("moeda com 3 letras (BRL, USD, EUR)")
	}
	switch d.Visibilidade {
	case "", "privada":
		d.Visibilidade, d.CasaID = "privada", nil
	case "saldo", "compartilhada":
		if d.CasaID == nil || *d.CasaID == "" {
			return invalido("escolha a casa para compartilhar")
		}
	default:
		return invalido("visibilidade: privada, saldo ou compartilhada")
	}
	for _, dia := range []*int16{d.Fechamento, d.Vencimento} {
		if dia != nil && (*dia < 1 || *dia > 31) {
			return invalido("dia de fechamento e vencimento entre 1 e 31")
		}
	}
	if d.InstituicaoID != nil && *d.InstituicaoID == "" {
		d.InstituicaoID = nil
	}
	return nil
}

func (s *Servidor) criarConta(w http.ResponseWriter, r *http.Request) {
	var d dadosConta
	if !lerJSON(w, r, &d) {
		return
	}
	if err := d.validar(); err != nil {
		falhar(w, r, err)
		return
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `insert into contas (entidade_id, instituicao_id, nome, tipo, moeda, visibilidade, casa_id,
				saldo_inicial_centavos, fechamento, vencimento, limite_centavos, arquivada)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) returning id`,
			d.EntidadeID, d.InstituicaoID, d.Nome, d.Tipo, d.Moeda, d.Visibilidade, d.CasaID, d.SaldoInicial,
			d.Fechamento, d.Vencimento, d.Limite, d.Arquivada).Scan(&id)
		if err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "conta_criada", id, map[string]any{"visibilidade": d.Visibilidade})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarConta(w http.ResponseWriter, r *http.Request) {
	var d dadosConta
	if !lerJSON(w, r, &d) {
		return
	}
	if err := d.validar(); err != nil {
		falhar(w, r, err)
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		// a entidade da conta não muda (as transações são dela)
		err := exigirUma(tx.Exec(ctx, `update contas set instituicao_id = $2, nome = $3, tipo = $4, moeda = $5,
				visibilidade = $6, casa_id = $7, saldo_inicial_centavos = $8, fechamento = $9, vencimento = $10,
				limite_centavos = $11, arquivada = $12
			where id = $1`, r.PathValue("id"), d.InstituicaoID, d.Nome, d.Tipo, d.Moeda, d.Visibilidade, d.CasaID,
			d.SaldoInicial, d.Fechamento, d.Vencimento, d.Limite, d.Arquivada))
		if err != nil {
			return err
		}
		// dias de fechamento/vencimento mudam a fatura de cada compra
		if err := financeiro.RecalcularFaturas(ctx, tx, r.PathValue("id")); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "conta_editada", r.PathValue("id"), map[string]any{"visibilidade": d.Visibilidade})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarConta(w http.ResponseWriter, r *http.Request) {
	if !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := exigirUma(tx.Exec(ctx, "delete from contas where id = $1", r.PathValue("id"))); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "conta_apagada", r.PathValue("id"), nil)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type instituicao struct {
	ID     string  `json:"id"`
	Nome   string  `json:"nome"`
	Codigo *string `json:"codigo_compe"`
}

func (s *Servidor) listarInstituicoes(w http.ResponseWriter, r *http.Request) {
	var lista []instituicao
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, "select id, nome, codigo_compe from instituicoes order by nome")
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[instituicao])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}
