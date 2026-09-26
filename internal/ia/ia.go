// Package ia conversa com o Claude (docs/SPEC.md §6): categorização em lote com um
// modelo pequeno e o chat "Pergunte às suas finanças" com ferramentas fechadas, que o
// servidor executa com as permissões do usuário. Nada de SQL livre: a IA só vê o que
// as ferramentas devolvem, e os textos saem sem CPF, CNPJ nem números de conta.
package ia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Modelos: pequeno e barato para categorizar; maior para o chat (SPEC §6, custos).
const (
	ModeloCategorizar = "claude-haiku-4-5"
	ModeloChat        = "claude-opus-5"
)

// precos em micro-dólares por milhão de tokens (entrada, saída); cache: escrita 1,25×,
// leitura 0,1× da entrada.
var precos = map[string][2]int64{
	ModeloCategorizar: {1_000_000, 5_000_000},
	ModeloChat:        {5_000_000, 25_000_000},
}

// ErrDesligada: sem ANTHROPIC_API_KEY no servidor.
var ErrDesligada = errors.New("IA não configurada no servidor")

// Cliente do Claude.
type Cliente struct {
	api anthropic.Client
}

// Novo cliente; sem chave devolve nil (a IA fica desligada). base vazio = API oficial.
func Novo(chave, base string) *Cliente {
	if strings.TrimSpace(chave) == "" {
		return nil
	}
	opcoes := []option.RequestOption{option.WithAPIKey(chave), option.WithMaxRetries(2)}
	if base != "" {
		opcoes = append(opcoes, option.WithBaseURL(base))
	}
	return &Cliente{api: anthropic.NewClient(opcoes...)}
}

// Uso de uma chamada (ou de várias somadas).
type Uso struct {
	Chamadas   int64 `json:"chamadas"`
	Entrada    int64 `json:"tokens_entrada"`
	Saida      int64 `json:"tokens_saida"`
	CustoMicro int64 `json:"custo_microdolares"`
}

func (u *Uso) somar(modelo string, x anthropic.Usage) {
	p := precos[modelo]
	u.Chamadas++
	u.Entrada += x.InputTokens + x.CacheCreationInputTokens + x.CacheReadInputTokens
	u.Saida += x.OutputTokens
	// custo = tokens × preço por milhão ÷ 1e6, com cache de escrita a 1,25× e leitura a 0,1×
	micro := x.InputTokens*p[0] + x.CacheCreationInputTokens*p[0]*5/4 + x.CacheReadInputTokens*p[0]/10 + x.OutputTokens*p[1]
	u.CustoMicro += (micro + 999_999) / 1_000_000
}

// Somar outro uso a este.
func (u *Uso) Somar(o Uso) {
	u.Chamadas += o.Chamadas
	u.Entrada += o.Entrada
	u.Saida += o.Saida
	u.CustoMicro += o.CustoMicro
}

var (
	reCNPJ   = regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`)
	reCPF    = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)
	reEmail  = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]+`)
	reDigito = regexp.MustCompile(`\d{5,}`) // agência, conta, cartão, protocolo
)

// Anonimizar tira CPF, CNPJ, e-mail e sequências longas de dígitos antes de enviar.
// Nomes próprios de terceiros em Pix ("PIX ENVIADO FULANO") viram "PIX ... [pessoa]"
// só quando o banco escreve o padrão conhecido; o resto do texto segue igual.
func Anonimizar(s string) string {
	s = reCNPJ.ReplaceAllString(s, "[cnpj]")
	s = reCPF.ReplaceAllString(s, "[cpf]")
	s = reEmail.ReplaceAllString(s, "[email]")
	s = reDigito.ReplaceAllString(s, "#")
	s = rePix.ReplaceAllString(s, "$1 [pessoa]")
	return strings.TrimSpace(s)
}

// rePix: "PIX ENVIADO/RECEBIDO/TRANSF ... NOME" — o nome da pessoa não sai do Pi.
var rePix = regexp.MustCompile(`(?i)^((?:pix|transf(?:erencia|erência)?|ted|doc)\s+(?:enviad[oa]|recebid[oa]|enviada|recebida)?)\s*(?:-\s*)?[A-Za-zÀ-ÿ][A-Za-zÀ-ÿ .]+$`)

// ---------------------------------------------------------------------------
// Categorização em lote
// ---------------------------------------------------------------------------

// ItemCategorizar: uma transação sem categoria.
type ItemCategorizar struct {
	ID        string
	Descricao string
	Valor     int64 // centavos, negativo = saída
}

// CategoriaOpcao que a IA pode escolher.
type CategoriaOpcao struct {
	ID   string
	Nome string // "Mercado" ou "Casa › Aluguel"
	Tipo string // gasto, receita, transferencia
}

const promptCategorizar = `Você classifica transações bancárias de uma pessoa no Brasil nas categorias dela.
Para cada transação, escolha o id de UMA categoria da lista, do mesmo tipo do valor (saída negativa é gasto;
entrada positiva é receita; pagamento de fatura ou transferência entre contas próprias é transferencia).
Se não tiver certeza razoável, use "nenhuma". Responda só chamando a ferramenta classificar.`

// Categorizar pede ao modelo pequeno uma categoria para cada item. Devolve id da
// transação → id da categoria (só as reconhecidas, que existem na lista).
func (c *Cliente) Categorizar(ctx context.Context, itens []ItemCategorizar, cats []CategoriaOpcao) (map[string]string, Uso, error) {
	var uso Uso
	out := map[string]string{}
	if c == nil {
		return out, uso, ErrDesligada
	}
	if len(itens) == 0 || len(cats) == 0 {
		return out, uso, nil
	}
	validas := map[string]bool{}
	var lista strings.Builder
	for _, k := range cats {
		validas[k.ID] = true
		fmt.Fprintf(&lista, "%s | %s | %s\n", k.ID, k.Tipo, k.Nome)
	}
	// ids curtos (t1, t2...) em vez dos uuids: menos tokens e nada de identificador interno
	curto := map[string]string{}
	var trans strings.Builder
	for i, it := range itens {
		k := fmt.Sprintf("t%d", i+1)
		curto[k] = it.ID
		fmt.Fprintf(&trans, "%s | %s | %.2f\n", k, Anonimizar(it.Descricao), float64(it.Valor)/100)
	}
	ferramenta := anthropic.ToolParam{
		Name:        "classificar",
		Description: anthropic.String("Registra a categoria escolhida para cada transação."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"itens": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"t":         map[string]any{"type": "string", "description": "id da transação (t1, t2...)"},
							"categoria": map[string]any{"type": "string", "description": "id da categoria ou \"nenhuma\""},
						},
						"required":             []string{"t", "categoria"},
						"additionalProperties": false,
					},
				},
			},
			Required: []string{"itens"},
		},
	}
	resp, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     ModeloCategorizar,
		MaxTokens: 8192,
		System: []anthropic.TextBlockParam{{
			Text: promptCategorizar + "\n\nCategorias (id | tipo | nome):\n" + lista.String(),
			// a lista de categorias se repete em cada lote: vale o cache
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Tools:      []anthropic.ToolUnionParam{{OfTool: &ferramenta}},
		ToolChoice: anthropic.ToolChoiceParamOfTool("classificar"),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Transações (id | descrição | valor em R$):\n" + trans.String())),
		},
	})
	if err != nil {
		return out, uso, err
	}
	uso.somar(ModeloCategorizar, resp.Usage)
	for _, bloco := range resp.Content {
		t, ok := bloco.AsAny().(anthropic.ToolUseBlock)
		if !ok || t.Name != "classificar" {
			continue
		}
		var in struct {
			Itens []struct {
				T         string `json:"t"`
				Categoria string `json:"categoria"`
			} `json:"itens"`
		}
		if err := json.Unmarshal([]byte(t.JSON.Input.Raw()), &in); err != nil {
			return out, uso, fmt.Errorf("resposta da IA inválida: %w", err)
		}
		for _, x := range in.Itens {
			if id, ok := curto[x.T]; ok && validas[x.Categoria] {
				out[id] = x.Categoria
			}
		}
	}
	return out, uso, nil
}

// ---------------------------------------------------------------------------
// Chat com ferramentas
// ---------------------------------------------------------------------------

// Ferramenta que o chat pode chamar: esquema JSON e a função que roda no servidor.
type Ferramenta struct {
	Nome       string
	Descricao  string
	Parametros map[string]any // properties do JSON Schema
	Requeridos []string
	Rodar      func(ctx context.Context, entrada json.RawMessage) (any, error)
}

// Mensagem do histórico do chat (só texto; o histórico fica no navegador).
type Mensagem struct {
	Papel string `json:"papel"` // usuario, assistente
	Texto string `json:"texto"`
}

// Resposta do chat.
type Resposta struct {
	Texto        string   `json:"texto"`
	Ferramentas  []string `json:"ferramentas"` // chamadas feitas, na ordem
	Uso          Uso      `json:"uso"`
	Interrompida bool     `json:"interrompida"` // passou do limite de passos
}

const promptChat = `Você é o assistente do painel de finanças "Finanças", respondendo em português do Brasil,
curto e direto, para quem está logado. Use SEMPRE as ferramentas para qualquer número: nunca invente, estime de
cabeça nem faça contas que a ferramenta já faz. Valores em reais no formato R$ 1.234,56. Diga o período a que o
número se refere. Quando citar transações, mostre data, descrição e valor. Se a pergunta pedir algo que as
ferramentas não cobrem, diga isso. Você só lê: para criar metas, regras ou orçamentos, explique como fazer na tela.
Em perguntas sobre impostos ou investimentos, termine lembrando que é uma estimativa do painel e não substitui
contador nem assessor. A data de hoje vem na primeira mensagem.`

// MaxPassos: chamadas ao modelo por pergunta (cada rodada de ferramentas conta uma).
const MaxPassos = 8

// Conversar roda o laço do chat: o modelo pede ferramentas, o servidor executa com as
// permissões do usuário e devolve os resultados, até a resposta final.
func (c *Cliente) Conversar(ctx context.Context, hoje string, historico []Mensagem, ferramentas []Ferramenta) (Resposta, error) {
	var r Resposta
	if c == nil {
		return r, ErrDesligada
	}
	porNome := map[string]Ferramenta{}
	var tools []anthropic.ToolUnionParam
	for i, f := range ferramentas {
		porNome[f.Nome] = f
		t := anthropic.ToolParam{
			Name:        f.Nome,
			Description: anthropic.String(f.Descricao),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: f.Parametros, Required: f.Requeridos},
		}
		if i == len(ferramentas)-1 {
			t.CacheControl = anthropic.NewCacheControlEphemeralParam() // ferramentas + sistema em cache
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &t})
	}
	var msgs []anthropic.MessageParam
	for i, m := range historico {
		texto := m.Texto
		if i == 0 {
			texto = "(hoje é " + hoje + ")\n" + texto
		}
		if m.Papel == "assistente" {
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(texto)))
		} else {
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(texto)))
		}
	}
	adaptativo := anthropic.ThinkingConfigAdaptiveParam{}
	for passo := 0; passo < MaxPassos; passo++ {
		resp, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     ModeloChat,
			MaxTokens: 16000,
			Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptativo},
			System:    []anthropic.TextBlockParam{{Text: promptChat}},
			Tools:     tools,
			Messages:  msgs,
		})
		if err != nil {
			return r, err
		}
		r.Uso.somar(ModeloChat, resp.Usage)
		if resp.StopReason == anthropic.StopReasonRefusal {
			r.Texto = "Não posso responder isso."
			return r, nil
		}
		msgs = append(msgs, resp.ToParam())
		var resultados []anthropic.ContentBlockParamUnion
		var texto strings.Builder
		for _, bloco := range resp.Content {
			switch b := bloco.AsAny().(type) {
			case anthropic.TextBlock:
				texto.WriteString(b.Text)
			case anthropic.ToolUseBlock:
				r.Ferramentas = append(r.Ferramentas, b.Name)
				resultados = append(resultados, rodarFerramenta(ctx, porNome, b))
			}
		}
		if resp.StopReason != anthropic.StopReasonToolUse || len(resultados) == 0 {
			r.Texto = strings.TrimSpace(texto.String())
			return r, nil
		}
		msgs = append(msgs, anthropic.NewUserMessage(resultados...))
	}
	r.Interrompida = true
	r.Texto = "A pergunta precisou de passos demais. Tente algo mais específico."
	return r, nil
}

func rodarFerramenta(ctx context.Context, porNome map[string]Ferramenta, b anthropic.ToolUseBlock) anthropic.ContentBlockParamUnion {
	f, ok := porNome[b.Name]
	if !ok {
		return anthropic.NewToolResultBlock(b.ID, "ferramenta desconhecida", true)
	}
	res, err := f.Rodar(ctx, json.RawMessage(b.JSON.Input.Raw()))
	if err != nil {
		return anthropic.NewToolResultBlock(b.ID, "erro: "+err.Error(), true)
	}
	j, err := json.Marshal(res)
	if err != nil {
		return anthropic.NewToolResultBlock(b.ID, "erro ao montar o resultado", true)
	}
	return anthropic.NewToolResultBlock(b.ID, string(j), false)
}
