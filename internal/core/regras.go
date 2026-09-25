package core

import (
	"errors"
	"time"
)

// Regra é uma linha de regras_fiscais: um valor com período de vigência.
type Regra struct {
	Chave      string
	Valor      []byte // JSON
	VigenteDe  time.Time
	VigenteAte *time.Time // nil = sem fim
	Fonte      string
}

// ErrSemRegraVigente indica que nenhuma regra cobre a data pedida.
var ErrSemRegraVigente = errors.New("nenhuma regra vigente na data")

// ErrRegrasSobrepostas indica duas regras da mesma chave valendo no mesmo dia.
var ErrRegrasSobrepostas = errors.New("regras com vigência sobreposta")

// RegraVigente escolhe, entre as regras de uma chave, a que vale na data.
// As datas comparadas são só o dia (o horário é ignorado).
func RegraVigente(regras []Regra, chave string, data time.Time) (Regra, error) {
	dia := truncarDia(data)
	var achou *Regra
	for i := range regras {
		r := &regras[i]
		if r.Chave != chave || truncarDia(r.VigenteDe).After(dia) {
			continue
		}
		if r.VigenteAte != nil && truncarDia(*r.VigenteAte).Before(dia) {
			continue
		}
		if achou != nil {
			return Regra{}, ErrRegrasSobrepostas
		}
		achou = r
	}
	if achou == nil {
		return Regra{}, ErrSemRegraVigente
	}
	return *achou, nil
}

func truncarDia(t time.Time) time.Time {
	a, m, d := t.Date()
	return time.Date(a, m, d, 0, 0, 0, 0, time.UTC)
}
