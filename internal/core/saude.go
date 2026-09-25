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
