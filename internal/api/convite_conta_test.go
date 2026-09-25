package api_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

// Convite de conta: o amigo cria a própria conta sem entrar na casa de quem convidou,
// não vê nada dela, e o link vale uma vez só.
func TestConviteDeConta(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa da Ana"}, http.StatusCreated).json(t, &casa)
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf, "nome": "CONTA-PRIVADA-DA-ANA", "tipo": "corrente",
		"visibilidade": "compartilhada", "casa_id": casa.ID}, http.StatusCreated)

	var conv struct{ Link string }
	a.exigir("POST", "/api/convites-conta", nil, http.StatusCreated).json(t, &conv)
	token := conv.Link[strings.LastIndex(conv.Link, "/")+1:]

	var info struct {
		Tipo         string `json:"tipo"`
		ConvidadoPor string `json:"convidado_por"`
		Casa         string `json:"casa"`
		Valido       bool   `json:"valido"`
	}
	amb.novoCliente().exigir("GET", "/api/convites/"+token, nil, http.StatusOK).json(t, &info)
	if info.Tipo != "conta" || info.ConvidadoPor != "Ana" || info.Casa != "" || !info.Valido {
		t.Fatalf("convite: %+v", info)
	}

	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", token)
	var casas []struct{ ID string }
	b.exigir("GET", "/api/casas", nil, http.StatusOK).json(t, &casas)
	if len(casas) != 0 {
		t.Fatalf("o amigo não deveria estar em casa nenhuma: %+v", casas)
	}
	if r := b.exigir("GET", "/api/contas", nil, http.StatusOK); bytes.Contains(r.corpo, []byte("CONTA-PRIVADA-DA-ANA")) {
		t.Fatalf("o amigo viu a conta da casa da Ana: %s", r.corpo)
	}

	// uso único; e só quem criou vê e cancela
	amb.novoCliente().exigir("GET", "/api/convites/"+token, nil, http.StatusOK).json(t, &info)
	if info.Valido {
		t.Fatal("convite usado continua válido")
	}
	c := amb.novoCliente()
	c.exigir("POST", "/api/auth/cadastro", map[string]string{"nome": "Carla", "email": "carla@teste.com",
		"senha": "senha comprida de teste", "convite": token}, http.StatusGone)
	var lista []struct {
		ID        string  `json:"id"`
		AceitoPor *string `json:"aceito_por"`
	}
	a.exigir("GET", "/api/convites-conta", nil, http.StatusOK).json(t, &lista)
	if len(lista) != 1 || lista[0].AceitoPor == nil || *lista[0].AceitoPor != "Bruno" {
		t.Fatalf("convites da Ana: %+v", lista)
	}
	if r := b.exigir("GET", "/api/convites-conta", nil, http.StatusOK); bytes.Contains(r.corpo, []byte(lista[0].ID)) {
		t.Fatal("Bruno viu o convite da Ana")
	}
	a.exigir("POST", "/api/convites-conta", nil, http.StatusCreated).json(t, &conv)
	a.exigir("GET", "/api/convites-conta", nil, http.StatusOK).json(t, &lista)
	var pendente string
	for _, x := range lista {
		if x.AceitoPor == nil {
			pendente = x.ID
		}
	}
	b.exigir("DELETE", "/api/convites-conta/"+pendente, nil, http.StatusNotFound)
	a.exigir("DELETE", "/api/convites-conta/"+pendente, nil, http.StatusNoContent)
	a.exigir("DELETE", "/api/convites-conta/"+lista[0].ID, nil, http.StatusNotFound) // aceito não se apaga
}
