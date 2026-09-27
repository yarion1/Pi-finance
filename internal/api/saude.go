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
		"pluggy": "ok", "backup": "pendente", "restore": "pendente", "last_sync": nil,
	}
	agora := time.Now()
	var worker, backup, restore *time.Time
	var restoreOK bool
	err := s.Pool.QueryRow(ctx, `select
		(select visto_em from sinais_vida where servico = 'worker'),
		(select visto_em from sinais_vida where servico = 'backup'),
		(select visto_em from sinais_vida where servico = 'restore_teste'),
		coalesce((select (dados->>'ok')::boolean from sinais_vida where servico = 'restore_teste'), false)`).
		Scan(&worker, &backup, &restore, &restoreOK)
	if err != nil {
		resp["db"] = "down"
	} else {
		resp["worker"] = core.EstadoWorker(worker, agora)
		resp["backup"] = core.EstadoBackup(backup, agora)
		resp["restore"] = core.EstadoRestore(restore, restoreOK, agora)
		var conexoes, comErro int
		var ultima *time.Time
		if err := s.Pool.QueryRow(ctx, "select conexoes, com_erro, ultima_sync from app_estado_pluggy()").
			Scan(&conexoes, &comErro, &ultima); err == nil {
			resp["pluggy"] = core.EstadoPluggy(conexoes, comErro, ultima, agora)
			if ultima != nil {
				resp["last_sync"] = ultima.UTC().Format(time.RFC3339)
			}
		}
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

// GET /api/sistema/backups: idade do último backup e o último teste de restore, para a
// tela Segurança. Só o estado: os totais conferidos e os erros ficam no banco e no log.
func (s *Servidor) backups(w http.ResponseWriter, r *http.Request) {
	type nuvem struct {
		Em *time.Time `json:"em"`
		OK bool       `json:"ok"`
	}
	var resp struct {
		Backup struct {
			Estado  string     `json:"estado"`
			Em      *time.Time `json:"em"`
			Cifrado bool       `json:"cifrado"`
			Nuvem   *nuvem     `json:"nuvem"`
		} `json:"backup"`
		Restore struct {
			Estado   string     `json:"estado"`
			Em       *time.Time `json:"em"`
			Segundos *int       `json:"segundos"`
		} `json:"restore"`
	}
	var restoreOK bool
	var nuvemEm *time.Time
	var nuvemOK *bool
	err := s.Pool.QueryRow(r.Context(), `select
		(select visto_em from sinais_vida where servico = 'backup'),
		coalesce((select (dados->>'cifrado')::boolean from sinais_vida where servico = 'backup'), false),
		(select (dados->'nuvem'->>'em')::timestamptz from sinais_vida where servico = 'backup'),
		(select (dados->'nuvem'->>'ok')::boolean from sinais_vida where servico = 'backup'),
		(select visto_em from sinais_vida where servico = 'restore_teste'),
		coalesce((select (dados->>'ok')::boolean from sinais_vida where servico = 'restore_teste'), false),
		(select (dados->>'segundos')::int from sinais_vida where servico = 'restore_teste')`).
		Scan(&resp.Backup.Em, &resp.Backup.Cifrado, &nuvemEm, &nuvemOK, &resp.Restore.Em, &restoreOK, &resp.Restore.Segundos)
	if err != nil {
		falhar(w, r, err)
		return
	}
	agora := time.Now()
	resp.Backup.Estado = core.EstadoBackup(resp.Backup.Em, agora)
	resp.Restore.Estado = core.EstadoRestore(resp.Restore.Em, restoreOK, agora)
	if nuvemOK != nil {
		resp.Backup.Nuvem = &nuvem{Em: nuvemEm, OK: *nuvemOK}
	}
	escreverJSON(w, http.StatusOK, resp)
}
