package notificar

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/cripto"
	"github.com/yarion1/pi-finance/internal/db"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

// Contextos de cifra (nome da coluna).
const (
	ctxChat      = "notificacoes_config.telegram_chat"
	ctxInscricao = "push_inscricoes.inscricao"
)

// Despachante: lê o que notificar no banco e manda por push e Telegram.
type Despachante struct {
	Pool       *pgxpool.Pool
	Cifrador   *cripto.Cifrador
	Push       *Push     // nil: push indisponível
	Telegram   *Telegram // nil: sem bot configurado
	URLPublica string    // para os links do Telegram

	mu  sync.Mutex
	bot string // @ do bot (cache do getMe)
}

// UsuarioBot: o @ do bot, para o link t.me (vazio sem bot ou se o Telegram não respondeu).
func (d *Despachante) UsuarioBot(ctx context.Context) string {
	if d.Telegram == nil {
		return ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bot == "" {
		u, err := d.Telegram.Usuario(ctx)
		if err != nil {
			slog.Warn("telegram: getMe", "erro", err)
			return ""
		}
		d.bot = u
	}
	return d.bot
}

// Aparelho que recebe push.
type Aparelho struct {
	ID       string    `json:"id"`
	Aparelho string    `json:"aparelho"`
	CriadaEm time.Time `json:"criada_em"`
}

// Aparelhos do usuário da sessão.
func Aparelhos(ctx context.Context, tx pgx.Tx) ([]Aparelho, error) {
	linhas, err := tx.Query(ctx, "select id, aparelho, criada_em from push_inscricoes order by criada_em")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, pgx.RowToStructByPos[Aparelho])
}

// TelegramConectado: o usuário da sessão tem chat vinculado?
func TelegramConectado(ctx context.Context, tx pgx.Tx) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `select exists (select 1 from notificacoes_config
		where usuario_id = app_usuario_id() and telegram_chat_cifrado is not null)`).Scan(&ok)
	return ok, err
}

// DesconectarTelegram do usuário da sessão.
func DesconectarTelegram(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `update notificacoes_config set telegram_chat_cifrado = null, telegram_codigo_hash = null,
		telegram_codigo_expira = null, atualizada_em = now() where usuario_id = app_usuario_id()`)
	return err
}

// RemoverAparelho do usuário da sessão.
func RemoverAparelho(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx, "delete from push_inscricoes where id = $1", id)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

// LerPreferencias do usuário da sessão.
func LerPreferencias(ctx context.Context, tx pgx.Tx) (Preferencias, error) {
	p := Preferencias{Tipos: map[string]Canal{}}
	var tipos []byte
	err := tx.QueryRow(ctx, "select tipos, mostrar_valores from notificacoes_config where usuario_id = app_usuario_id()").
		Scan(&tipos, &p.MostrarValores)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(tipos, &p.Tipos)
	return p, nil
}

// SalvarPreferencias do usuário da sessão (só os tipos conhecidos).
func SalvarPreferencias(ctx context.Context, tx pgx.Tx, p Preferencias) error {
	tipos := map[string]Canal{}
	for _, t := range Tipos {
		if c, ok := p.Tipos[t]; ok {
			tipos[t] = c
		}
	}
	j, _ := json.Marshal(tipos)
	_, err := tx.Exec(ctx, `insert into notificacoes_config (usuario_id, tipos, mostrar_valores) values (app_usuario_id(), $1, $2)
		on conflict (usuario_id) do update set tipos = excluded.tipos, mostrar_valores = excluded.mostrar_valores, atualizada_em = now()`,
		j, p.MostrarValores)
	return err
}

// MaxAparelhos por pessoa.
const MaxAparelhos = 10

// ErrMuitosAparelhos: passou do limite de aparelhos.
var ErrMuitosAparelhos = fmt.Errorf("no máximo %d aparelhos; remova um antes", MaxAparelhos)

// ErrChaves: as chaves que o navegador mandou não servem.
var ErrChaves = errors.New("chaves do navegador inválidas")

// SalvarInscricao do aparelho (o endereço precisa ser de um serviço de push conhecido).
func (d *Despachante) SalvarInscricao(ctx context.Context, tx pgx.Tx, i Inscricao, aparelho string) error {
	if err := ValidarEndpoint(i.Endpoint); err != nil {
		return err
	}
	if _, err := Cifrar([]byte("teste"), i, nil, nil); err != nil {
		return ErrChaves
	}
	j, _ := json.Marshal(i)
	cifrada, err := d.Cifrador.Cifrar(string(j), ctxInscricao)
	if err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(ctx, "select count(*) from push_inscricoes").Scan(&n); err != nil {
		return err
	}
	if n >= MaxAparelhos {
		return ErrMuitosAparelhos
	}
	h := sha256.Sum256([]byte(i.Endpoint))
	if len(aparelho) > 120 {
		aparelho = aparelho[:120]
	}
	// o mesmo aparelho reinscrito troca as chaves; de outra pessoa, não
	_, err = tx.Exec(ctx, `insert into push_inscricoes (usuario_id, endpoint_hash, inscricao_cifrada, aparelho)
		values (app_usuario_id(), $1, $2, $3)
		on conflict (endpoint_hash) do update set inscricao_cifrada = excluded.inscricao_cifrada, aparelho = excluded.aparelho
		where push_inscricoes.usuario_id = app_usuario_id()`, hex.EncodeToString(h[:]), cifrada, aparelho)
	return err
}

// NovoCodigoTelegram: código de uso único (15 min) para o link t.me/<bot>?start=<código>.
func NovoCodigoTelegram(ctx context.Context, tx pgx.Tx) (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	codigo := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	h := sha256.Sum256([]byte(codigo))
	_, err := tx.Exec(ctx, `insert into notificacoes_config (usuario_id, telegram_codigo_hash, telegram_codigo_expira)
		values (app_usuario_id(), $1, now() + interval '15 minutes')
		on conflict (usuario_id) do update set telegram_codigo_hash = excluded.telegram_codigo_hash,
			telegram_codigo_expira = excluded.telegram_codigo_expira`, hex.EncodeToString(h[:]))
	return codigo, err
}

// destinos do usuário da sessão: inscrições de push e o chat do Telegram.
func (d *Despachante) destinos(ctx context.Context, tx pgx.Tx) (insc map[string]Inscricao, chat int64, err error) {
	insc = map[string]Inscricao{}
	linhas, err := tx.Query(ctx, "select id, inscricao_cifrada from push_inscricoes")
	if err != nil {
		return nil, 0, err
	}
	for linhas.Next() {
		var id, cifrada string
		if err := linhas.Scan(&id, &cifrada); err != nil {
			linhas.Close()
			return nil, 0, err
		}
		j, err := d.Cifrador.Decifrar(cifrada, ctxInscricao)
		if err != nil {
			continue
		}
		var i Inscricao
		if json.Unmarshal([]byte(j), &i) == nil {
			insc[id] = i
		}
	}
	linhas.Close()
	var chatCifrado *string
	err = tx.QueryRow(ctx, "select telegram_chat_cifrado from notificacoes_config where usuario_id = app_usuario_id()").Scan(&chatCifrado)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, err
	}
	if chatCifrado != nil {
		if s, err := d.Cifrador.Decifrar(*chatCifrado, ctxChat); err == nil {
			chat, _ = strconv.ParseInt(s, 10, 64)
		}
	}
	return insc, chat, nil
}

// Enviar o aviso para os aparelhos e o Telegram do usuário, conforme as preferências
// do tipo (tipo vazio: manda em todos os canais, é o teste). Devolve quantos canais receberam.
func (d *Despachante) Enviar(ctx context.Context, usuarioID, tipo string, montar func(Preferencias) Aviso) (int, error) {
	var prefs Preferencias
	var insc map[string]Inscricao
	var chat int64
	if err := db.ComUsuario(ctx, d.Pool, usuarioID, func(tx pgx.Tx) error {
		var err error
		if prefs, err = LerPreferencias(ctx, tx); err != nil {
			return err
		}
		insc, chat, err = d.destinos(ctx, tx)
		return err
	}); err != nil {
		return 0, err
	}
	aviso := montar(prefs)
	enviados := 0
	var vencidas []string
	if d.Push != nil && (tipo == "" || prefs.Quer(tipo, false)) {
		conteudo, _ := json.Marshal(aviso)
		for id, i := range insc {
			err := d.Push.Enviar(ctx, i, conteudo)
			switch {
			case errors.Is(err, ErrInscricaoVencida) || errors.Is(err, ErrEndpoint):
				vencidas = append(vencidas, id)
			case err != nil:
				slog.Warn("push falhou", "usuario", usuarioID, "erro", err)
			default:
				enviados++
			}
		}
	}
	if d.Telegram != nil && chat != 0 && (tipo == "" || prefs.Quer(tipo, true)) {
		texto := aviso.Titulo
		if aviso.Texto != "" {
			texto += "\n" + aviso.Texto
		}
		if d.URLPublica != "" && aviso.Link != "" {
			texto += "\n" + strings.TrimRight(d.URLPublica, "/") + aviso.Link
		}
		if err := d.Telegram.Enviar(ctx, chat, texto); err != nil {
			slog.Warn("telegram falhou", "usuario", usuarioID, "erro", err)
		} else {
			enviados++
		}
	}
	if len(vencidas) > 0 {
		_ = db.ComUsuario(ctx, d.Pool, usuarioID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "delete from push_inscricoes where id = any($1)", vencidas)
			return err
		})
	}
	return enviados, nil
}

// Alertas: manda os alertas novos (dos últimos 2 dias, não lidos, ainda não notificados)
// e marca como notificados, mesmo quando a pessoa não quer aquele tipo.
func (d *Despachante) Alertas(ctx context.Context) (int, error) {
	linhas, err := d.Pool.Query(ctx, "select id, usuario_id from app_alertas_para_notificar()")
	if err != nil {
		return 0, err
	}
	type pendente struct{ id, usuario string }
	pendentes, err := pgx.CollectRows(linhas, func(r pgx.CollectableRow) (pendente, error) {
		var p pendente
		return p, r.Scan(&p.id, &p.usuario)
	})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, p := range pendentes {
		var tipo string
		var dados map[string]any
		if err := db.ComUsuario(ctx, d.Pool, p.usuario, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, "select tipo, dados from alertas where id = $1", p.id).Scan(&tipo, &dados); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "update alertas set notificado_em = now() where id = $1", p.id)
			return err
		}); err != nil {
			slog.Warn("notificação: alerta", "id", p.id, "erro", err)
			continue
		}
		n, err := d.Enviar(ctx, p.usuario, TipoDoAlerta(tipo), func(pr Preferencias) Aviso {
			return TextoDoAlerta(tipo, dados, pr.MostrarValores)
		})
		if err != nil {
			slog.Warn("notificação: envio", "id", p.id, "erro", err)
		}
		total += n
	}
	return total, nil
}

// HoraAgenda: a partir desta hora (São Paulo) sai o lembrete das contas do dia.
const HoraAgenda = 8

// Agenda: uma vez por dia, depois das 8 h, as contas que vencem hoje e amanhã.
func (d *Despachante) Agenda(ctx context.Context, agora time.Time) (int, error) {
	if agora.Hour() < HoraAgenda {
		return 0, nil
	}
	linhas, err := d.Pool.Query(ctx, "select * from app_usuarios_notificaveis()")
	if err != nil {
		return 0, err
	}
	usuarios, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	hoje := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.UTC)
	chave := "agenda:" + hoje.Format("2006-01-02")
	total := 0
	for _, u := range usuarios {
		var itens []string
		var soma core.Centavos
		var novo bool
		if err := db.ComUsuario(ctx, d.Pool, u, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `insert into notificacoes_enviadas (usuario_id, chave) values (app_usuario_id(), $1)
				on conflict do nothing`, chave)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			novo = true
			ents, err := financeiro.Entidades(ctx, tx, "")
			if err != nil || len(ents) == 0 {
				return err
			}
			ag, err := financeiro.CalcularAgenda(ctx, tx, ents, hoje, 1)
			if err != nil {
				return err
			}
			for _, it := range ag.Itens {
				if !it.Pago && !it.NoCartao && it.Valor < 0 {
					itens = append(itens, it.Descricao)
					soma -= core.Centavos(it.Valor)
				}
			}
			return nil
		}); err != nil {
			slog.Warn("notificação: agenda", "usuario", u, "erro", err)
			continue
		}
		if !novo || len(itens) == 0 {
			continue
		}
		n, err := d.Enviar(ctx, u, TipoAgenda, func(pr Preferencias) Aviso { return TextoAgenda(itens, soma, pr.MostrarValores) })
		if err != nil {
			slog.Warn("notificação: agenda", "usuario", u, "erro", err)
		}
		total += n
	}
	return total, nil
}

// VincularTelegram lê as mensagens novas do bot: "/start <código>" liga o chat à pessoa
// que pediu o código; o resto recebe uma explicação curta.
func (d *Despachante) VincularTelegram(ctx context.Context) error {
	if d.Telegram == nil {
		return nil
	}
	var desde int64
	var s string
	if err := d.Pool.QueryRow(ctx, "select valor from estado_servidor where chave = 'telegram_offset'").Scan(&s); err == nil {
		desde, _ = strconv.ParseInt(s, 10, 64)
	}
	ms, err := d.Telegram.Novas(ctx, desde)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m.ID >= desde {
			desde = m.ID + 1
		}
		if m.ChatID == 0 {
			continue
		}
		resposta := "Este é o bot do painel Finanças. Para receber avisos aqui, abra Mais › Notificações no painel e toque em Conectar Telegram."
		if codigo := CodigoDoStart(m.Texto); codigo != "" {
			h := sha256.Sum256([]byte(codigo))
			cifrado, err := d.Cifrador.Cifrar(ChatTexto(m.ChatID), ctxChat)
			if err != nil {
				return err
			}
			var usuario *string
			if err := d.Pool.QueryRow(ctx, "select app_vincular_telegram($1, $2)", hex.EncodeToString(h[:]), cifrado).Scan(&usuario); err != nil {
				return err
			}
			if usuario != nil {
				resposta = "Pronto: os avisos do painel Finanças vão chegar aqui. Dá para escolher quais em Mais › Notificações."
			} else {
				resposta = "Esse link venceu ou já foi usado. Gere outro em Mais › Notificações."
			}
		}
		if err := d.Telegram.Enviar(ctx, m.ChatID, resposta); err != nil {
			slog.Warn("telegram: resposta", "erro", err)
		}
	}
	_, err = d.Pool.Exec(ctx, `insert into estado_servidor (chave, valor) values ('telegram_offset', $1)
		on conflict (chave) do update set valor = excluded.valor`, fmt.Sprint(desde))
	return err
}
