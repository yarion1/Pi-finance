package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/ia"
	"github.com/yarion1/pi-finance/internal/importadores"
)

var tiposDocumento = map[string]bool{ia.DocFatura: true, ia.DocNota: true, ia.DocComprovante: true, ia.DocHolerite: true}

// limiteImagem: a API do Claude aceita imagens de até 5 MB.
const limiteImagem = 5 << 20

// midiaDoArquivo pelo conteúdo (nunca pelo nome ou pelo que o navegador diz).
func midiaDoArquivo(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("%PDF-")):
		return "application/pdf"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(b) > 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}

// previaDocumento: o extraído convertido no que vai ser lançado.
type previaDocumento struct {
	Tipo      string                    `json:"tipo"`
	Extraido  json.RawMessage           `json:"extraido"`
	Linhas    []importadores.Linha      `json:"linhas,omitempty"`
	Operacoes []importadores.OperacaoB3 `json:"operacoes,omitempty"`
	Holerite  *importadores.Holerite    `json:"holerite,omitempty"`
	Avisos    []string                  `json:"avisos"`
}

func converterDocumento(tipo string, extraido json.RawMessage) (previaDocumento, error) {
	p := previaDocumento{Tipo: tipo, Extraido: extraido, Avisos: []string{}}
	switch tipo {
	case ia.DocFatura, ia.DocComprovante:
		conv := importadores.LinhasDaFatura
		if tipo == ia.DocComprovante {
			conv = importadores.LinhaDoComprovante
		}
		r, err := conv(extraido)
		if err != nil {
			return p, invalido(err.Error())
		}
		p.Linhas = r.Linhas
		p.Avisos = append(p.Avisos, r.Avisos...)
	case ia.DocNota:
		r, err := importadores.OperacoesDaNota(extraido)
		if err != nil {
			return p, invalido(err.Error())
		}
		p.Operacoes = r.Linhas
		p.Avisos = append(p.Avisos, r.Avisos...)
	case ia.DocHolerite:
		h, err := importadores.HoleriteDoDocumento(extraido)
		if err != nil {
			return p, invalido(err.Error())
		}
		p.Holerite = &h
	default:
		return p, invalido("tipo: fatura, nota_corretagem, comprovante ou holerite")
	}
	return p, nil
}

// POST /api/documentos/ler: manda o PDF ou a foto para a IA e devolve a prévia. Nada é
// gravado (nem o arquivo): a pessoa revisa e lança em /api/documentos/lancar.
func (s *Servidor) lerDocumento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Tipo            string `json:"tipo"`
		Conteudo        string `json:"conteudo"`
		EnvioConfirmado bool   `json:"envio_confirmado"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if !c.EnvioConfirmado {
		falhar(w, r, invalido("confirme que o documento, com os dados pessoais dele, vai para a API da Anthropic"))
		return
	}
	if !tiposDocumento[c.Tipo] {
		falhar(w, r, invalido("tipo: fatura, nota_corretagem, comprovante ou holerite"))
		return
	}
	arquivo, err := decodificarArquivo(c.Conteudo)
	if err != nil {
		falhar(w, r, err)
		return
	}
	midia := midiaDoArquivo(arquivo)
	if midia == "" {
		falhar(w, r, invalido("envie um PDF ou uma foto (JPEG, PNG ou WebP)"))
		return
	}
	if midia != "application/pdf" && len(arquivo) > limiteImagem {
		falhar(w, r, invalido("foto maior que 5 MB"))
		return
	}
	if !s.exigirIA(w, r) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(3 * time.Minute))
	ctx, cancelar := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancelar()
	extraido, uso, errIA := s.IA.LerDocumento(ctx, c.Tipo, midia, arquivo)
	if err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.RegistrarUsoIA(ctx, tx, financeiro.Hoje(), uso)
	}); err != nil {
		falhar(w, r, err)
		return
	}
	if errIA != nil {
		if errors.Is(errIA, ia.ErrDocumento) {
			falhar(w, r, invalido("documento inválido"))
			return
		}
		erroJSON(w, http.StatusBadGateway, "ia", "a IA não conseguiu ler o documento; tente outro arquivo ou mais tarde")
		return
	}
	p, err := converterDocumento(c.Tipo, extraido)
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, p)
}

// POST /api/documentos/lancar: o extraído (revisado) vira transações, operações ou holerite.
func (s *Servidor) lancarDocumento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Tipo       string          `json:"tipo"`
		Extraido   json.RawMessage `json:"extraido"`
		ContaID    string          `json:"conta_id"`
		EntidadeID string          `json:"entidade_id"`
		Simular    bool            `json:"simular"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	p, err := converterDocumento(c.Tipo, c.Extraido)
	if err != nil {
		falhar(w, r, err)
		return
	}
	var resp any
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		switch c.Tipo {
		case ia.DocFatura, ia.DocComprovante:
			res, err := financeiro.Importar(ctx, tx, sessaoDe(r).UsuarioID, c.ContaID, "documento lido pela IA",
				importadores.Resultado{Formato: importadores.FormatoDocumento, Moeda: "BRL", Linhas: p.Linhas, Avisos: p.Avisos},
				c.Simular)
			resp = res
			if err != nil || c.Simular {
				return err
			}
			return s.auditar(ctx, tx, r, "importacao", res.ImportacaoID, map[string]any{"novas": res.Novas, "origem": "documento"})
		case ia.DocNota:
			res, err := financeiro.ImportarB3(ctx, tx, c.EntidadeID,
				importadores.ResultadoB3{Formato: importadores.FormatoNotaCorretagem, Linhas: p.Operacoes, Avisos: p.Avisos}, c.Simular)
			resp = res
			return err
		default: // holerite
			if c.Simular {
				resp = p.Holerite
				return nil
			}
			id, err := financeiro.SalvarHolerite(ctx, tx, c.EntidadeID, *p.Holerite)
			resp = map[string]string{"id": id}
			return err
		}
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

// GET /api/holerites?ano=2026
func (s *Servidor) listarHolerites(w http.ResponseWriter, r *http.Request) {
	ano := financeiro.Hoje().Year()
	if v := r.URL.Query().Get("ano"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2000 || n > 2100 {
			falhar(w, r, invalido("ano inválido"))
			return
		}
		ano = n
	}
	var out financeiro.HoleritesDoAno
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		out, err = financeiro.ListarHolerites(ctx, tx, ents, ano)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, out)
}

func (s *Servidor) apagarHolerite(w http.ResponseWriter, r *http.Request) {
	s.apagarDaTabela(w, r, "holerites")
}
