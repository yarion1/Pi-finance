package notificar

import (
	"strings"
	"testing"
)

func TestPreferencias(t *testing.T) {
	var p Preferencias
	if !p.Quer(TipoAlertas, false) || !p.Quer(TipoAgenda, true) {
		t.Fatal("sem configuração, tudo ligado")
	}
	p.Tipos = map[string]Canal{TipoAlertas: {Push: true}}
	if !p.Quer(TipoAlertas, false) || p.Quer(TipoAlertas, true) || !p.Quer(TipoRelatorios, true) {
		t.Fatalf("por canal: %+v", p)
	}
	for tipo, grupo := range map[string]string{"login_aparelho_novo": TipoSeguranca, "relatorio_pronto": TipoRelatorios,
		"cobranca_duplicada": TipoAlertas, "qualquer": TipoAlertas} {
		if TipoDoAlerta(tipo) != grupo {
			t.Errorf("%s → %s", tipo, TipoDoAlerta(tipo))
		}
	}
}

func TestTextoDoAlerta(t *testing.T) {
	d := map[string]any{"descricao": "UBER *TRIP", "valor_centavos": float64(-12345), "referencia_centavos": float64(-3000),
		"data": "2026-09-20", "conta": "Nubank", "categoria": "Transporte", "de_centavos": float64(3990), "para_centavos": float64(4490),
		"banco_centavos": float64(100), "calculado_centavos": float64(200), "ip": "1.2.3.4"}
	casos := []struct {
		tipo, titulo, texto, link string
	}{
		{"gasto_fora_do_padrao", "Gasto fora do padrão em Transporte: UBER *TRIP", "R$ 123,45 em 20/09; o normal é perto de R$ 30,00", "/gastos?busca=UBER+%2ATRIP"},
		{"cobranca_duplicada", "Possível cobrança duplicada: UBER *TRIP", "R$ 123,45 duas vezes até 20/09 · Nubank", "/gastos?busca=UBER+%2ATRIP"},
		{"tarifa_bancaria", "Tarifa bancária: UBER *TRIP", "R$ 123,45 em 20/09 · Nubank", "/gastos?busca=UBER+%2ATRIP"},
		{"juros_iof", "Juros ou IOF: UBER *TRIP", "R$ 123,45 em 20/09 · Nubank", "/gastos?busca=UBER+%2ATRIP"},
		{"recorrencia_subiu", "UBER *TRIP ficou mais cara", "de R$ 39,90 para R$ 44,90", "/recorrencias"},
		{"assinatura_detectada", "Nova cobrança recorrente: UBER *TRIP", "R$ 123,45", "/recorrencias"},
		{"saldo_divergente", "Saldo de Nubank diferente do banco", "banco R$ 1,00, painel R$ 2,00", "/contas"},
		{"login_aparelho_novo", "Login em aparelho novo", "Se não foi você, troque a senha em Segurança. 1.2.3.4", "/seguranca"},
		{"outro", "Novo alerta no painel", "outro", "/"},
	}
	for _, c := range casos {
		a := TextoDoAlerta(c.tipo, d, true)
		if a.Titulo != c.titulo || a.Texto != c.texto || a.Link != c.link {
			t.Errorf("%s: %+v", c.tipo, a)
		}
	}
	// sem valores: nada de R$ na tela bloqueada
	for _, c := range casos {
		if a := TextoDoAlerta(c.tipo, d, false); strings.Contains(a.Titulo+a.Texto, "R$") {
			t.Errorf("%s mostra valor: %+v", c.tipo, a)
		}
	}
	if a := TextoDoAlerta("gasto_fora_do_padrao", map[string]any{"valor_centavos": "x"}, true); a.Link != "/gastos" || a.Titulo != "Gasto fora do padrão: " {
		t.Errorf("sem descrição: %+v", a)
	}
	if a := TextoDoAlerta("cobranca_duplicada", map[string]any{"data": "ontem"}, true); !strings.Contains(a.Texto, "até ontem") {
		t.Errorf("data livre: %+v", a)
	}
	mes := TextoDoAlerta("relatorio_pronto", map[string]any{"tipo": "mes", "periodo": "2026-08", "relatorio_id": "r1"}, false)
	if mes.Titulo != "Relatório de agosto pronto" || mes.Link != "/relatorios/r1" {
		t.Errorf("mês: %+v", mes)
	}
	sem := TextoDoAlerta("relatorio_pronto", map[string]any{"tipo": "semana", "periodo": "2026-09-21"}, false)
	if sem.Titulo != "Resumo da semana de 21/09" || sem.Link != "/relatorios" {
		t.Errorf("semana: %+v", sem)
	}
}

func TestTextoAgenda(t *testing.T) {
	a := TextoAgenda([]string{"Luz"}, 12345, true)
	if a.Titulo != "1 conta para hoje e amanhã (R$ 123,45)" || a.Texto != "Luz" || a.Link != "/agenda" {
		t.Errorf("uma: %+v", a)
	}
	a = TextoAgenda([]string{"a", "b", "c", "d", "e", "f"}, 1, false)
	if a.Titulo != "6 contas para hoje e amanhã" || a.Texto != "a, b, c, d e mais 2" {
		t.Errorf("várias: %+v", a)
	}
}
