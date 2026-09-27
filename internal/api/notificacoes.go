package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/notificar"
)

type estadoNotificacoes struct {
	Preferencias     notificar.Preferencias `json:"preferencias"`
	Tipos            []string               `json:"tipos"`
	PushDisponivel   bool                   `json:"push_disponivel"`
	ChavePublica     string                 `json:"chave_publica"`
	Aparelhos        []notificar.Aparelho   `json:"aparelhos"`
	TelegramDisponiv bool                   `json:"telegram_disponivel"`
	TelegramLigado   bool                   `json:"telegram_conectado"`
}

// GET /api/notificacoes: preferências, aparelhos e Telegram da pessoa.
func (s *Servidor) notificacoes(w http.ResponseWriter, r *http.Request) {
	e := estadoNotificacoes{Tipos: notificar.Tipos}
	d := s.Notificar
	if d != nil && d.Push != nil {
		e.PushDisponivel, e.ChavePublica = true, d.Push.VAPID.Publica
	}
	e.TelegramDisponiv = d != nil && d.Telegram != nil
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if e.Preferencias, err = notificar.LerPreferencias(ctx, tx); err != nil {
			return err
		}
		if e.Aparelhos, err = notificar.Aparelhos(ctx, tx); err != nil {
			return err
		}
		e.TelegramLigado, err = notificar.TelegramConectado(ctx, tx)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, e)
}

// PUT /api/notificacoes: tipos por canal e se mostra valores.
func (s *Servidor) salvarNotificacoes(w http.ResponseWriter, r *http.Request) {
	var p notificar.Preferencias
	if !lerJSON(w, r, &p) {
		return
	}
	if err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return notificar.SalvarPreferencias(ctx, tx, p)
	}); err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/notificacoes/push: inscreve este aparelho.
func (s *Servidor) inscreverPush(w http.ResponseWriter, r *http.Request) {
	var c struct {
		notificar.Inscricao
		Aparelho string `json:"aparelho"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if s.Notificar == nil || s.Notificar.Push == nil {
		erroJSON(w, http.StatusServiceUnavailable, "indisponivel", "notificações no aparelho indisponíveis")
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return s.Notificar.SalvarInscricao(ctx, tx, c.Inscricao, c.Aparelho)
	})
	switch {
	case errors.Is(err, notificar.ErrEndpoint) || errors.Is(err, notificar.ErrMuitosAparelhos) || errors.Is(err, notificar.ErrChaves):
		erroJSON(w, http.StatusBadRequest, "validacao", err.Error())
	case err != nil:
		falhar(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// DELETE /api/notificacoes/push/{id}
func (s *Servidor) removerPush(w http.ResponseWriter, r *http.Request) {
	if err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return notificar.RemoverAparelho(ctx, tx, r.PathValue("id"))
	}); err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/notificacoes/telegram: link de uso único para ligar o chat. Pede a senha de
// novo: com ele, os avisos passam a sair do painel para outro lugar.
func (s *Servidor) conectarTelegram(w http.ResponseWriter, r *http.Request) {
	if !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	bot := ""
	if s.Notificar != nil {
		bot = s.Notificar.UsuarioBot(r.Context())
	}
	if bot == "" {
		erroJSON(w, http.StatusServiceUnavailable, "indisponivel", "bot do Telegram indisponível")
		return
	}
	var codigo string
	if err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		codigo, err = notificar.NovoCodigoTelegram(ctx, tx)
		if err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "telegram_codigo", "", nil)
	}); err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]string{
		"link": "https://t.me/" + url.PathEscape(bot) + "?start=" + codigo, "expira_em": time.Now().Add(15 * time.Minute).Format(time.RFC3339),
	})
}

// DELETE /api/notificacoes/telegram
func (s *Servidor) desconectarTelegram(w http.ResponseWriter, r *http.Request) {
	if err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := notificar.DesconectarTelegram(ctx, tx); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "telegram_desconectado", "", nil)
	}); err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/notificacoes/teste: manda um aviso de teste para todos os canais.
func (s *Servidor) testarNotificacao(w http.ResponseWriter, r *http.Request) {
	if s.Notificar == nil {
		erroJSON(w, http.StatusServiceUnavailable, "indisponivel", "notificações indisponíveis")
		return
	}
	ctx, cancelar := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancelar()
	n, err := s.Notificar.Enviar(ctx, sessaoDe(r).UsuarioID, "", func(notificar.Preferencias) notificar.Aviso {
		return notificar.Aviso{Titulo: "Teste do painel Finanças", Texto: "As notificações estão funcionando.", Link: "/notificacoes"}
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]int{"enviados": n})
}
