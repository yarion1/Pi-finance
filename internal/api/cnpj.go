package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/importadores"
)

// contextoDocumentoCliente: CPF/CNPJ do tomador, cifrado como os outros documentos.
const contextoDocumentoCliente = "clientes.documento"

func anoDaURL(r *http.Request) int {
	if a, err := strconv.Atoi(r.URL.Query().Get("ano")); err == nil && a > 2000 && a < 2200 {
		return a
	}
	return financeiro.Hoje().Year()
}

func (s *Servidor) painelCNPJ(w http.ResponseWriter, r *http.Request) {
	var p financeiro.PainelCNPJ
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		p, err = financeiro.CalcularPainelCNPJ(ctx, tx, r.PathValue("entidade"), financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, p)
}

func (s *Servidor) listarNotas(w http.ResponseWriter, r *http.Request) {
	var lista []financeiro.Nota
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		lista, err = financeiro.ListarNotas(ctx, tx, r.PathValue("entidade"), anoDaURL(r))
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) criarNota(w http.ResponseWriter, r *http.Request) {
	var d financeiro.DadosNota
	if !lerJSON(w, r, &d) {
		return
	}
	d.EntidadeID = r.PathValue("entidade")
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = financeiro.CriarNota(ctx, tx, d)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// importarNFSe: XMLs do Emissor Nacional (um por nota). Notas já importadas são
// ignoradas pela chave de acesso.
func (s *Servidor) importarNFSe(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Arquivos []struct {
			Nome     string `json:"nome"`
			Conteudo string `json:"conteudo"`
		} `json:"arquivos"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if len(c.Arquivos) == 0 || len(c.Arquivos) > 100 {
		falhar(w, r, invalido("envie de 1 a 100 arquivos XML"))
		return
	}
	type resultado struct {
		Novas      int      `json:"novas"`
		Duplicadas int      `json:"duplicadas"`
		Erros      []string `json:"erros"`
		Avisos     []string `json:"avisos"`
	}
	res := resultado{Erros: []string{}, Avisos: []string{}}
	var notas []importadores.NFSe
	for _, a := range c.Arquivos {
		bruto, err := decodificarArquivo(a.Conteudo)
		if err != nil {
			res.Erros = append(res.Erros, nomeCurto(a.Nome)+": arquivo inválido")
			continue
		}
		n, err := importadores.LerNFSe(bruto)
		if err != nil {
			res.Erros = append(res.Erros, nomeCurto(a.Nome)+": "+err.Error())
			continue
		}
		notas = append(notas, n)
	}
	entidade := r.PathValue("entidade")
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		for _, n := range notas {
			var doc *string
			if n.TomadorDoc != "" {
				cifrado, err := s.Cifrador.Cifrar(n.TomadorDoc, contextoDocumentoCliente)
				if err != nil {
					return err
				}
				doc = &cifrado
			}
			d := financeiro.DadosNota{EntidadeID: entidade, Cliente: n.TomadorNome, Pais: n.TomadorPais,
				Numero: n.Numero, ChaveAcesso: n.Chave, DataEmissao: n.Competencia.Format("2006-01-02"),
				Descricao: truncar(n.Descricao, 500), Valor: int64(n.Valor), Exportacao: n.Exportacao,
				Origem: "nfse", DocumentoCliente: doc}
			if n.Exportacao {
				res.Avisos = append(res.Avisos, "Nota "+n.Numero+" marcada como exportação (tomador no exterior): confirme com o contador.")
			}
			id, err := financeiro.CriarNota(ctx, tx, d)
			if err != nil {
				return err
			}
			if id == "" {
				res.Duplicadas++
			} else {
				res.Novas++
			}
		}
		return s.auditar(ctx, tx, r, "importacao_nfse", entidade, map[string]any{"novas": res.Novas, "duplicadas": res.Duplicadas})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, res)
}

func nomeCurto(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return "arquivo"
	}
	return truncar(n, 80)
}

func truncar(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (s *Servidor) editarNota(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Cancelada bool `json:"cancelada"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.CancelarNota(ctx, tx, r.PathValue("id"), c.Cancelada)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarNota(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.ApagarNota(ctx, tx, r.PathValue("id"))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) marcarDAS(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Valor  int64   `json:"valor_centavos"`
		PagoEm *string `json:"pago_em"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.MarcarDAS(ctx, tx, r.PathValue("entidade"), r.PathValue("competencia"), core.Centavos(c.Valor), c.PagoEm, financeiro.Hoje())
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) listarFolha(w http.ResponseWriter, r *http.Request) {
	var lista []financeiro.Folha
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := existeEntidade(ctx, tx, r.PathValue("entidade")); err != nil {
			return err
		}
		var err error
		lista, err = financeiro.ListarFolha(ctx, tx, r.PathValue("entidade"), financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func existeEntidade(ctx context.Context, tx pgx.Tx, id string) error {
	var x string
	return tx.QueryRow(ctx, "select id from entidades where id = $1", id).Scan(&x)
}

func (s *Servidor) salvarFolha(w http.ResponseWriter, r *http.Request) {
	var f financeiro.Folha
	if !lerJSON(w, r, &f) {
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		f, err = financeiro.SalvarFolha(ctx, tx, r.PathValue("entidade"), f)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, f)
}

// simulacaoCNPJ: continuar MEI × migrar para ME, com a receita mensal e o contador.
func (s *Servidor) simulacaoCNPJ(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	receita, err1 := strconv.ParseInt(q.Get("receita_mensal_centavos"), 10, 64)
	contador, err2 := strconv.ParseInt(q.Get("contador_centavos"), 10, 64)
	if err1 != nil || err2 != nil || receita < 0 || contador < 0 || receita > 1_000_000_000_00 {
		falhar(w, r, invalido("informe a receita mensal e o custo do contador em centavos"))
		return
	}
	atividade := q.Get("atividade")
	if atividade != "comercio" && atividade != "ambos" {
		atividade = "servicos"
	}
	var sim financeiro.Simulacao
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		sim, err = financeiro.Simular(ctx, tx, atividade, core.Centavos(receita), core.Centavos(contador), financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, sim)
}

func (s *Servidor) listarDistribuicoes(w http.ResponseWriter, r *http.Request) {
	var lista []financeiro.Distribuicao
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := existeEntidade(ctx, tx, r.PathValue("entidade")); err != nil {
			return err
		}
		var err error
		lista, err = financeiro.ListarDistribuicoes(ctx, tx, r.PathValue("entidade"), anoDaURL(r))
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

// distribuirLucro para quem está logado; simular=true só devolve o aviso de IRRF.
func (s *Servidor) distribuirLucro(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Data      string `json:"data"`
		Valor     int64  `json:"valor_centavos"`
		Descricao string `json:"descricao"`
		Simular   bool   `json:"simular"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	data, err := time.Parse("2006-01-02", c.Data)
	if err != nil || c.Valor <= 0 || len(c.Descricao) > 200 {
		falhar(w, r, invalido("informe a data e o valor"))
		return
	}
	var aviso financeiro.AvisoDistribuicao
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		ent := r.PathValue("entidade")
		if _, err := financeiro.CalcularPainelCNPJ(ctx, tx, ent, data); err != nil && !errors.Is(err, core.ErrSemRegraVigente) {
			return err // confere que é um CNPJ visível
		}
		var err error
		if c.Simular {
			aviso, err = financeiro.SimularDistribuicao(ctx, tx, ent, sessaoDe(r).UsuarioID, data, core.Centavos(c.Valor))
			return err
		}
		aviso, err = financeiro.RegistrarDistribuicao(ctx, tx, ent, sessaoDe(r).UsuarioID, data, core.Centavos(c.Valor), c.Descricao)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	status := http.StatusCreated
	if c.Simular {
		status = http.StatusOK
	}
	escreverJSON(w, status, aviso)
}
