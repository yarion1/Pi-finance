// Package pluggyfalsa é uma API da Pluggy de mentira, com o mesmo formato da real,
// para os testes e o e2e (cmd/pluggy-falsa). Nunca entra no binário de produção.
package pluggyfalsa

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yarion1/pi-finance/internal/pluggy"
)

// Item falso: de quem é (client id) e as contas com as transações.
type Item struct {
	Dono   string
	Item   pluggy.Item
	Contas []*Conta
}

// Conta falsa com as transações dela.
type Conta struct {
	Conta      pluggy.Conta
	Transacoes []pluggy.Transacao
}

// Falsa guarda o estado; mexa nele com os métodos (são seguros entre goroutines).
type Falsa struct {
	mu       sync.Mutex
	clientes map[string]string // client id → secret
	itens    map[string]*Item
	// PorPagina: quantas transações por página (testa o cursor).
	PorPagina int
	Chamadas  int
}

// Nova API falsa vazia.
func Nova() *Falsa {
	return &Falsa{clientes: map[string]string{}, itens: map[string]*Item{}, PorPagina: 2}
}

// Cliente registra credenciais válidas.
func (f *Falsa) Cliente(id, secret string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clientes[id] = secret
}

// Item acrescenta um banco conectado para o client id.
func (f *Falsa) Item(dono, id, banco string, contas ...*Conta) {
	f.mu.Lock()
	defer f.mu.Unlock()
	it := &Item{Dono: dono, Contas: contas}
	it.Item.ID, it.Item.Status, it.Item.LastUpdatedAt = id, "UPDATED", "2026-09-25T09:00:00.000Z"
	it.Item.Connector.Name = banco
	f.itens[id] = it
}

// Status muda o estado do item (ex.: LOGIN_ERROR).
func (f *Falsa) Status(item, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.itens[item].Item.Status = status
}

// Saldo muda o saldo de uma conta.
func (f *Falsa) Saldo(conta, valor string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.conta(conta); c != nil {
		c.Conta.Balance = json.Number(valor)
	}
}

// Transacao acrescenta uma transação a uma conta.
func (f *Falsa) Transacao(conta string, t pluggy.Transacao) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c := f.conta(conta); c != nil {
		c.Transacoes = append(c.Transacoes, t)
	}
}

func (f *Falsa) conta(id string) *Conta {
	for _, it := range f.itens {
		for _, c := range it.Contas {
			if c.Conta.ID == id {
				return c
			}
		}
	}
	return nil
}

func escrever(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// dono da chave de API ("chave-<client id>"), vazio se inválida.
func (f *Falsa) dono(r *http.Request) string {
	id, ok := strings.CutPrefix(r.Header.Get("X-API-KEY"), "chave-")
	if !ok {
		return ""
	}
	if _, existe := f.clientes[id]; !existe {
		return ""
	}
	return id
}

// ServeHTTP implementa /auth, /items/{id}, /accounts e /v2/transactions.
func (f *Falsa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Chamadas++

	if r.Method == http.MethodPost && r.URL.Path == "/auth" {
		var c struct{ ClientID, ClientSecret string }
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil || c.ClientID == "" || f.clientes[c.ClientID] != c.ClientSecret {
			escrever(w, http.StatusUnauthorized, map[string]string{"message": "invalid credentials"})
			return
		}
		escrever(w, http.StatusOK, map[string]string{"apiKey": "chave-" + c.ClientID})
		return
	}
	dono := f.dono(r)
	if dono == "" {
		escrever(w, http.StatusForbidden, map[string]string{"message": "forbidden"})
		return
	}
	item := func(id string) *Item {
		if it := f.itens[id]; it != nil && it.Dono == dono {
			return it
		}
		return nil
	}

	switch {
	case strings.HasPrefix(r.URL.Path, "/items/"):
		it := item(strings.TrimPrefix(r.URL.Path, "/items/"))
		if it == nil {
			escrever(w, http.StatusNotFound, map[string]string{"message": "not found"})
			return
		}
		escrever(w, http.StatusOK, it.Item)
	case r.URL.Path == "/accounts":
		it := item(r.URL.Query().Get("itemId"))
		if it == nil {
			escrever(w, http.StatusNotFound, map[string]string{"message": "not found"})
			return
		}
		contas := []pluggy.Conta{}
		for _, c := range it.Contas {
			contas = append(contas, c.Conta)
		}
		escrever(w, http.StatusOK, map[string]any{"results": contas})
	case r.URL.Path == "/v2/transactions":
		q := r.URL.Query()
		var c *Conta
		for _, it := range f.itens {
			if it.Dono != dono {
				continue
			}
			for _, x := range it.Contas {
				if x.Conta.ID == q.Get("accountId") {
					c = x
				}
			}
		}
		if c == nil {
			escrever(w, http.StatusNotFound, map[string]string{"message": "not found"})
			return
		}
		lista := []pluggy.Transacao{}
		for _, t := range c.Transacoes {
			d, err := pluggy.DataSP(t.Date)
			if err == nil && d.Format("2006-01-02") >= q.Get("dateFrom") {
				lista = append(lista, t)
			}
		}
		sort.SliceStable(lista, func(i, j int) bool { return lista[i].Date < lista[j].Date })
		inicio, _ := strconv.Atoi(q.Get("cursor"))
		fim := min(inicio+f.PorPagina, len(lista))
		resp := map[string]any{"results": lista[min(inicio, fim):fim]}
		if fim < len(lista) {
			prox := q
			prox.Set("cursor", strconv.Itoa(fim))
			resp["next"] = "?" + prox.Encode()
		}
		escrever(w, http.StatusOK, resp)
	default:
		escrever(w, http.StatusNotFound, map[string]string{"message": "not found"})
	}
}
