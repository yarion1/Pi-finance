package core

import "time"

// Limites do /api/health (docs/SPEC.md §11).
const (
	LimiteSinalWorker = 3 * time.Minute
	LimiteBackup      = 8 * time.Hour // backup a cada 6 h + folga
)

// EstadoWorker: "up" se o worker deu sinal de vida nos últimos 3 minutos.
func EstadoWorker(vistoEm *time.Time, agora time.Time) string {
	if vistoEm == nil || agora.Sub(*vistoEm) > LimiteSinalWorker {
		return "down"
	}
	return "up"
}

// EstadoBackup: "ok", "atrasado" ou "pendente" (nunca rodou).
func EstadoBackup(ultimo *time.Time, agora time.Time) string {
	switch {
	case ultimo == nil:
		return "pendente"
	case agora.Sub(*ultimo) > LimiteBackup:
		return "atrasado"
	default:
		return "ok"
	}
}

// EstadoPluggy: "ok" sem conexões ou com todas em dia; "erro" se alguma conexão ou
// banco falhou; "atrasada" se a última sincronização passou de 36 h (a diária falhou).
// Não derruba o /api/health: é informação para o monitor.
func EstadoPluggy(conexoes, comErro int, ultima *time.Time, agora time.Time) string {
	switch {
	case conexoes == 0:
		return "ok"
	case comErro > 0:
		return "erro"
	case ultima == nil || agora.Sub(*ultima) > 36*time.Hour:
		return "atrasada"
	}
	return "ok"
}
