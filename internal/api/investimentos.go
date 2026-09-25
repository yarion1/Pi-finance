package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/importadores"
)

// carteira: os ativos com posição e valor de hoje, e o total por classe.
func (s *Servidor) carteira(w http.ResponseWriter, r *http.Request) {
	var c financeiro.Carteira
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		c, err = financeiro.CalcularCarteira(ctx, tx, ents, financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, c)
}

// importarB3: extrato de negociação ou de movimentação da Área do Investidor da B3.
func (s *Servidor) importarB3(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID string `json:"entidade_id"`
		Conteudo   string `json:"conteudo"`
		Simular    bool   `json:"simular"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	bruto, err := decodificarArquivo(c.Conteudo)
	if err != nil {
		falhar(w, r, err)
		return
	}
	lido, err := importadores.LerB3(bruto)
	if err != nil {
		falhar(w, r, invalido(err.Error()))
		return
	}
	var res financeiro.ResultadoB3
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		res, err = financeiro.ImportarB3(ctx, tx, c.EntidadeID, lido, c.Simular)
		if err != nil || c.Simular {
			return err
		}
		return s.auditar(ctx, tx, r, "importacao_b3", c.EntidadeID, map[string]any{"novas": res.Novas, "duplicadas": res.Duplicadas})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, res)
}

func (s *Servidor) criarAtivo(w http.ResponseWriter, r *http.Request) {
	var d financeiro.DadosAtivo
	if !lerJSON(w, r, &d) {
		return
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = financeiro.CriarAtivo(ctx, tx, d)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarAtivo(w http.ResponseWriter, r *http.Request) {
	var d json.RawMessage
	if !lerJSON(w, r, &d) {
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.EditarAtivo(ctx, tx, r.PathValue("id"), d)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarAtivo(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.ApagarAtivo(ctx, tx, r.PathValue("id"))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) listarOperacoes(w http.ResponseWriter, r *http.Request) {
	var lista []financeiro.OperacaoAtivo
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var existe bool
		if err := tx.QueryRow(ctx, "select exists (select 1 from ativos where id = $1)", r.PathValue("id")).Scan(&existe); err != nil {
			return err
		}
		if !existe {
			return errNaoEncontrado
		}
		var err error
		lista, err = financeiro.ListarOperacoes(ctx, tx, r.PathValue("id"))
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) criarOperacao(w http.ResponseWriter, r *http.Request) {
	var d financeiro.DadosOperacao
	if !lerJSON(w, r, &d) {
		return
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = financeiro.CriarOperacao(ctx, tx, r.PathValue("id"), d)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) apagarOperacao(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.ApagarOperacao(ctx, tx, r.PathValue("id"), r.PathValue("operacao"))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// irInvestimentos: apuração mensal de renda variável de uma entidade.
func (s *Servidor) irInvestimentos(w http.ResponseWriter, r *http.Request) {
	var ap financeiro.ApuracaoIR
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		// cada CPF ou CNPJ apura o próprio IR; sem entidade na URL, a PF da pessoa
		pedida := r.URL.Query().Get("entidade_id")
		var id string
		var err error
		if pedida != "" {
			err = tx.QueryRow(ctx, "select id from entidades where id = $1", pedida).Scan(&id)
		} else {
			err = tx.QueryRow(ctx, `select id from entidades where dono_id = app_usuario_id()
				order by tipo = 'PF' desc, criada_em limit 1`).Scan(&id)
		}
		if errors.Is(err, pgx.ErrNoRows) && pedida == "" {
			ap = financeiro.ApuracaoIR{Meses: []core.ApuracaoMes{}, Vendas: []financeiro.VendaIR{}}
			return nil
		}
		if err != nil {
			return err
		}
		ap, err = financeiro.ApurarIR(ctx, tx, id)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, ap)
}
