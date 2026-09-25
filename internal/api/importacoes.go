package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/importadores"
)

// limiteArquivo: extratos de anos cabem folgados; o corpo vem em base64.
const limiteArquivo = 8 << 20

func decodificarArquivo(conteudo string) ([]byte, error) {
	if i := strings.Index(conteudo, ","); strings.HasPrefix(conteudo, "data:") && i > 0 {
		conteudo = conteudo[i+1:] // aceita data URL
	}
	b, err := base64.StdEncoding.DecodeString(conteudo)
	if err != nil {
		return nil, invalido("arquivo inválido")
	}
	if len(b) == 0 || len(b) > limiteArquivo {
		return nil, invalido("arquivo vazio ou maior que 8 MB")
	}
	return b, nil
}

func (s *Servidor) importar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		ContaID    string                   `json:"conta_id"`
		Arquivo    string                   `json:"arquivo"`
		Conteudo   string                   `json:"conteudo"`
		Mapeamento *importadores.Mapeamento `json:"mapeamento"`
		Simular    bool                     `json:"simular"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	bruto, err := decodificarArquivo(c.Conteudo)
	if err != nil {
		falhar(w, r, err)
		return
	}
	nome := strings.TrimSpace(c.Arquivo)
	if nome == "" || len(nome) > 200 {
		nome = "arquivo"
	}
	lido, err := importadores.Ler(bruto, c.Mapeamento)
	if errors.Is(err, importadores.ErrFormato) {
		cab, _ := importadores.Cabecalhos(bruto, "")
		escreverJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"erro": "mapeamento", "mensagem": "Não reconheci este CSV. Diga quais colunas são data, descrição e valor.",
			"cabecalhos": cab,
		})
		return
	}
	if err != nil {
		falhar(w, r, invalido(err.Error()))
		return
	}
	var res financeiro.ResultadoImportacao
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		res, err = financeiro.Importar(ctx, tx, sessaoDe(r).UsuarioID, c.ContaID, nome, lido, c.Simular)
		if err != nil || c.Simular {
			return err
		}
		return s.auditar(ctx, tx, r, "importacao", res.ImportacaoID, map[string]any{"novas": res.Novas, "duplicadas": res.Duplicadas})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	status := http.StatusCreated
	if c.Simular {
		status = http.StatusOK
	}
	escreverJSON(w, status, res)
}

func (s *Servidor) cabecalhosCSV(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Conteudo  string `json:"conteudo"`
		Separador string `json:"separador"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	bruto, err := decodificarArquivo(c.Conteudo)
	if err != nil {
		falhar(w, r, err)
		return
	}
	cab, err := importadores.Cabecalhos(bruto, c.Separador)
	if err != nil {
		falhar(w, r, invalido(err.Error()))
		return
	}
	escreverJSON(w, http.StatusOK, map[string]any{"cabecalhos": cab})
}

type importacao struct {
	ID             string     `json:"id"`
	EntidadeID     string     `json:"entidade_id"`
	ContaID        string     `json:"conta_id"`
	Conta          string     `json:"conta"`
	Fonte          string     `json:"fonte"`
	Arquivo        string     `json:"arquivo"`
	Linhas         int        `json:"linhas"`
	Novas          int        `json:"novas"`
	Duplicadas     int        `json:"duplicadas"`
	Transferencias int        `json:"transferencias"`
	CriadaEm       time.Time  `json:"criada_em"`
	DesfeitaEm     *time.Time `json:"desfeita_em"`
}

func (s *Servidor) listarImportacoes(w http.ResponseWriter, r *http.Request) {
	var lista []importacao
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		consulta := `select i.id, i.entidade_id, i.conta_id, c.nome, i.fonte, i.arquivo, i.linhas, i.novas, i.duplicadas,
			i.transferencias, i.criada_em, i.desfeita_em from importacoes i join contas c on c.id = i.conta_id`
		args := []any{}
		if e := r.URL.Query().Get("entidade_id"); e != "" {
			consulta += " where i.entidade_id = $1"
			args = append(args, e)
		}
		linhas, err := tx.Query(ctx, consulta+" order by i.criada_em desc limit 100", args...)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[importacao])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) desfazerImportacao(w http.ResponseWriter, r *http.Request) {
	var apagadas int
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if apagadas, err = financeiro.DesfazerImportacao(ctx, tx, r.PathValue("id")); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "importacao_desfeita", r.PathValue("id"), map[string]any{"apagadas": apagadas})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]int{"apagadas": apagadas})
}
