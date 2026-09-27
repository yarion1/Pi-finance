package notificar

import (
	"fmt"
	"net/url"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
)

// Tipos de notificação que a pessoa liga e desliga (por canal).
const (
	TipoAlertas    = "alertas"    // gasto fora do padrão, cobrança duplicada, tarifa, saldo diferente...
	TipoRelatorios = "relatorios" // relatório do mês e resumo da semana prontos
	TipoAgenda     = "agenda"     // contas que vencem hoje e amanhã
	TipoSeguranca  = "seguranca"  // login em aparelho novo
)

// Tipos na ordem da tela.
var Tipos = []string{TipoAlertas, TipoAgenda, TipoRelatorios, TipoSeguranca}

// Canal: onde a notificação vai.
type Canal struct {
	Push     bool `json:"push"`
	Telegram bool `json:"telegram"`
}

// Preferencias de notificação da pessoa (sem linha: tudo ligado, sem valores).
type Preferencias struct {
	Tipos          map[string]Canal `json:"tipos"`
	MostrarValores bool             `json:"mostrar_valores"`
}

// Quer: o tipo vai por esse canal? O que não foi configurado vem ligado.
func (p Preferencias) Quer(tipo string, telegram bool) bool {
	c, ok := p.Tipos[tipo]
	if !ok {
		return true
	}
	if telegram {
		return c.Telegram
	}
	return c.Push
}

// Aviso: o que aparece na notificação.
type Aviso struct {
	Titulo string `json:"titulo"`
	Texto  string `json:"texto"`
	Link   string `json:"url"` // caminho no painel
}

// TipoDoAlerta: a que grupo de preferência o alerta pertence.
func TipoDoAlerta(tipo string) string {
	switch tipo {
	case "login_aparelho_novo":
		return TipoSeguranca
	case "relatorio_pronto":
		return TipoRelatorios
	}
	return TipoAlertas
}

var meses = []string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro",
	"outubro", "novembro", "dezembro"}

// TextoDoAlerta monta o aviso como o início do painel mostra. Sem mostrar valores, os
// valores viram "•••" (a notificação aparece na tela bloqueada).
func TextoDoAlerta(tipo string, d map[string]any, mostrarValores bool) Aviso {
	texto := func(k string) string {
		s, _ := d[k].(string)
		return s
	}
	valor := func(k string) string {
		if !mostrarValores {
			return "•••"
		}
		f, ok := d[k].(float64)
		if !ok {
			return ""
		}
		if f < 0 {
			f = -f
		}
		return core.FormatarBRL(core.Centavos(int64(f)))
	}
	data := texto("data")
	if t, err := time.Parse("2006-01-02", data); err == nil {
		data = t.Format("02/01")
	}
	conta := ""
	if c := texto("conta"); c != "" {
		conta = " · " + c
	}
	desc := texto("descricao")
	busca := "/gastos"
	if desc != "" {
		busca = "/gastos?busca=" + url.QueryEscape(desc)
	}
	switch tipo {
	case "login_aparelho_novo":
		return Aviso{Titulo: "Login em aparelho novo", Texto: "Se não foi você, troque a senha em Segurança. " + texto("ip"),
			Link: "/seguranca"}
	case "recorrencia_subiu":
		return Aviso{Titulo: desc + " ficou mais cara", Texto: fmt.Sprintf("de %s para %s", valor("de_centavos"), valor("para_centavos")),
			Link: "/recorrencias"}
	case "saldo_divergente":
		return Aviso{Titulo: "Saldo de " + texto("conta") + " diferente do banco",
			Texto: fmt.Sprintf("banco %s, painel %s", valor("banco_centavos"), valor("calculado_centavos")), Link: "/contas"}
	case "assinatura_detectada":
		return Aviso{Titulo: "Nova cobrança recorrente: " + desc, Texto: valor("valor_centavos"), Link: "/recorrencias"}
	case "gasto_fora_do_padrao":
		cat := ""
		if c := texto("categoria"); c != "" {
			cat = " em " + c
		}
		return Aviso{Titulo: "Gasto fora do padrão" + cat + ": " + desc,
			Texto: fmt.Sprintf("%s em %s; o normal é perto de %s", valor("valor_centavos"), data, valor("referencia_centavos")), Link: busca}
	case "cobranca_duplicada":
		return Aviso{Titulo: "Possível cobrança duplicada: " + desc,
			Texto: fmt.Sprintf("%s duas vezes até %s%s", valor("valor_centavos"), data, conta), Link: busca}
	case "tarifa_bancaria":
		return Aviso{Titulo: "Tarifa bancária: " + desc, Texto: fmt.Sprintf("%s em %s%s", valor("valor_centavos"), data, conta), Link: busca}
	case "juros_iof":
		return Aviso{Titulo: "Juros ou IOF: " + desc, Texto: fmt.Sprintf("%s em %s%s", valor("valor_centavos"), data, conta), Link: busca}
	case "relatorio_pronto":
		link := "/relatorios"
		if id := texto("relatorio_id"); id != "" {
			link += "/" + id
		}
		p := texto("periodo")
		if texto("tipo") == "mes" {
			if t, err := time.Parse("2006-01", p); err == nil {
				p = meses[t.Month()-1]
			}
			return Aviso{Titulo: "Relatório de " + p + " pronto", Texto: "O que mudou no mês e o que fazer agora.", Link: link}
		}
		if t, err := time.Parse("2006-01-02", p); err == nil {
			p = t.Format("02/01")
		}
		return Aviso{Titulo: "Resumo da semana de " + p, Texto: "Quanto gastou e o que vence nos próximos dias.", Link: link}
	}
	return Aviso{Titulo: "Novo alerta no painel", Texto: tipo, Link: "/"}
}

// TextoAgenda: as contas de hoje e amanhã numa notificação só.
func TextoAgenda(itens []string, total core.Centavos, mostrarValores bool) Aviso {
	a := Aviso{Titulo: fmt.Sprintf("%d %s para hoje e amanhã", len(itens), map[bool]string{true: "conta", false: "contas"}[len(itens) == 1]),
		Link: "/agenda"}
	if mostrarValores {
		a.Titulo += " (" + core.FormatarBRL(total) + ")"
	}
	a.Texto = juntar(itens, 4)
	return a
}

func juntar(itens []string, max int) string {
	s := ""
	for i, it := range itens {
		if i == max {
			return s + fmt.Sprintf(" e mais %d", len(itens)-max)
		}
		if i > 0 {
			s += ", "
		}
		s += it
	}
	return s
}
