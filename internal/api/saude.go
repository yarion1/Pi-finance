package api

import (
	"context"
	"net/http"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
)

// saude responde ao deploy e ao monitor do PiControl (docs/SPEC.md §11).
// 200 quando banco e worker estão de pé; 503 caso contrário.
func (s *Servidor) saude(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancelar()

	resp := map[string]any{
		"status": "ok", "version": s.Versao, "db": "up", "worker": "down",
		"pluggy": "ok", "backup": "pendente", "last_sync": nil,
	}
	agora := time.Now()
	var worker, backup *time.Time
	err := s.Pool.QueryRow(ctx, `select
		(select visto_em from sinais_vida where servico = 'worker'),
		(select visto_em from sinais_vida where servico = 'backup')`).Scan(&worker, &backup)
	if err != nil {
		resp["db"] = "down"
	} else {
		resp["worker"] = core.EstadoWorker(worker, agora)
		resp["backup"] = core.EstadoBackup(backup, agora)
	}

	status := http.StatusOK
	if resp["db"] != "up" || resp["worker"] != "up" {
		resp["status"] = "degradado"
		status = http.StatusServiceUnavailable
	}
	if s.Config.ForcarFalhaSaude {
		resp["status"] = "falha_forcada"
		status = http.StatusServiceUnavailable
	}
	escreverJSON(w, status, resp)
}
