package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/openfinance"
	"github.com/yarion1/pi-finance/internal/pluggy"
)

// errAguarde: sincronização pedida logo depois da anterior.
var errAguarde = errors.New("a última sincronização foi agora há pouco; aguarde alguns segundos")

// falharOpenFinance traduz os erros da Pluggy; o resto segue para falhar.
func falharOpenFinance(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pluggy.ErrCredenciais):
		erroJSON(w, http.StatusBadRequest, "validacao", "o Meu Pluggy recusou o Client ID e o Client Secret")
	case errors.Is(err, pluggy.ErrItemNaoEncontrado):
		erroJSON(w, http.StatusBadRequest, "validacao", "item não encontrado no Meu Pluggy: confira o id copiado")
	case errors.Is(err, pluggy.ErrRecusada):
		erroJSON(w, http.StatusBadGateway, "pluggy", err.Error())
	case errors.Is(err, pluggy.ErrIndisponivel):
		erroJSON(w, http.StatusBadGateway, "pluggy", "o Meu Pluggy não respondeu; tente de novo em alguns minutos")
	case errors.Is(err, openfinance.ErrSemConexao):
		erroJSON(w, http.StatusBadRequest, "validacao", err.Error())
	case errors.Is(err, openfinance.ErrItemRepetido):
		erroJSON(w, http.StatusConflict, "duplicado", err.Error())
	case errors.Is(err, errAguarde):
		erroJSON(w, http.StatusTooManyRequests, "aguarde", err.Error())
	default:
		falhar(w, r, err)
	}
}

type contaOpenFinance struct {
	ID         string     `json:"id"`
	Nome       string     `json:"nome"`
	Tipo       string     `json:"tipo"`
	Numero     *string    `json:"numero"`
	Moeda      string     `json:"moeda"`
	Saldo      *int64     `json:"saldo_centavos"`
	Limite     *int64     `json:"limite_centavos"`
	SaldoEm    *time.Time `json:"saldo_em"`
	ContaID    *string    `json:"conta_id"`
	Conta      *string    `json:"conta"`
	Ignorada   bool       `json:"ignorada"`
	UltimaSync *time.Time `json:"ultima_sync"`
}

type itemOpenFinance struct {
	ID           string             `json:"id"`
	ItemID       string             `json:"item_id"`
	EntidadeID   string             `json:"entidade_id"`
	Entidade     *string            `json:"entidade"`
	Instituicao  *string            `json:"instituicao"`
	Status       *string            `json:"status"`
	AtualizadoEm *time.Time         `json:"atualizado_em"`
	UltimaSync   *time.Time         `json:"ultima_sync"`
	UltimoErro   *string            `json:"ultimo_erro"`
	Contas       []contaOpenFinance `json:"contas"`
}

// openFinance: as credenciais (só o final do Client ID, nunca o segredo), os bancos e
// as contas que cada um devolveu. Tudo só da própria pessoa.
func (s *Servidor) openFinance(w http.ResponseWriter, r *http.Request) {
	var resp struct {
		Conexao *struct {
			ClientIDFim string     `json:"client_id_fim"`
			CriadaEm    time.Time  `json:"criada_em"`
			UltimaSync  *time.Time `json:"ultima_sync"`
			UltimoErro  *string    `json:"ultimo_erro"`
		} `json:"conexao"`
		Itens []itemOpenFinance `json:"itens"`
	}
	resp.Itens = []itemOpenFinance{}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var c struct {
			ClientIDFim string     `json:"client_id_fim"`
			CriadaEm    time.Time  `json:"criada_em"`
			UltimaSync  *time.Time `json:"ultima_sync"`
			UltimoErro  *string    `json:"ultimo_erro"`
		}
		err := tx.QueryRow(ctx, `select client_id_fim, criada_em, ultima_sync, ultimo_erro from conexoes_pluggy
			where usuario_id = app_usuario_id()`).Scan(&c.ClientIDFim, &c.CriadaEm, &c.UltimaSync, &c.UltimoErro)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		resp.Conexao = &c
		linhas, err := tx.Query(ctx, `select i.id, i.item_id, i.entidade_id, e.nome, i.instituicao, i.status, i.atualizado_em,
				i.ultima_sync, i.ultimo_erro
			from itens_pluggy i left join entidades e on e.id = i.entidade_id order by i.criado_em`)
		if err != nil {
			return err
		}
		for linhas.Next() {
			var it itemOpenFinance
			if err := linhas.Scan(&it.ID, &it.ItemID, &it.EntidadeID, &it.Entidade, &it.Instituicao, &it.Status,
				&it.AtualizadoEm, &it.UltimaSync, &it.UltimoErro); err != nil {
				linhas.Close()
				return err
			}
			it.Contas = []contaOpenFinance{}
			resp.Itens = append(resp.Itens, it)
		}
		linhas.Close()
		for i := range resp.Itens {
			linhas, err := tx.Query(ctx, `select cp.id, cp.nome, cp.tipo, cp.numero, cp.moeda, cp.saldo_centavos, cp.limite_centavos,
					cp.saldo_em, cp.conta_id, c.nome, cp.ignorada, cp.ultima_sync
				from contas_pluggy cp left join contas c on c.id = cp.conta_id
				where cp.item_id = $1 order by cp.tipo, cp.nome`, resp.Itens[i].ID)
			if err != nil {
				return err
			}
			contas, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[contaOpenFinance])
			if err != nil {
				return err
			}
			resp.Itens[i].Contas = append(resp.Itens[i].Contas, contas...)
		}
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

// salvarCredenciais: cadastrar ou trocar as credenciais pede reautenticação (quem
// troca as credenciais decide de onde vêm os dados).
func (s *Servidor) salvarCredenciais(w http.ResponseWriter, r *http.Request) {
	if !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	var c struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	u := sessaoDe(r).UsuarioID
	if err := s.OpenFinance.Conectar(r.Context(), u, c.ClientID, c.ClientSecret); err != nil {
		falharOpenFinance(w, r, err)
		return
	}
	_ = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return s.auditar(ctx, tx, r, "open_finance_credenciais", u, nil)
	})
	w.WriteHeader(http.StatusNoContent)
}

// apagarConexao remove credenciais, bancos e vínculos; as transações ficam.
func (s *Servidor) apagarConexao(w http.ResponseWriter, r *http.Request) {
	if !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if err := exigirUma(tx.Exec(ctx, "delete from conexoes_pluggy where usuario_id = app_usuario_id()")); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "open_finance_desconectado", sessaoDe(r).UsuarioID, nil)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) adicionarItem(w http.ResponseWriter, r *http.Request) {
	var c struct {
		ItemID     string `json:"item_id"`
		EntidadeID string `json:"entidade_id"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	id, err := s.OpenFinance.AdicionarItem(r.Context(), sessaoDe(r).UsuarioID, c.ItemID, c.EntidadeID)
	if err != nil {
		falharOpenFinance(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) apagarItem(w http.ResponseWriter, r *http.Request) {
	s.apagarDaTabela(w, r, "itens_pluggy")
}

// vincularConta decide o destino de uma conta da Pluggy: ligar a uma conta do
// painel, criar uma nova, ignorar ou desfazer o vínculo.
func (s *Servidor) vincularConta(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Acao    string  `json:"acao"` // vincular, criar, ignorar, desvincular
		ContaID *string `json:"conta_id"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	id := r.PathValue("id")
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		switch c.Acao {
		case "criar":
			_, err := openfinance.CriarConta(ctx, tx, id)
			return err
		case "vincular":
			if c.ContaID == nil || *c.ContaID == "" {
				return invalido("escolha a conta")
			}
			// a conta precisa ser da mesma entidade do banco conectado (a política confere a edição)
			return exigirUma(tx.Exec(ctx, `update contas_pluggy cp set conta_id = $2, ignorada = false, acertar_saldo = false,
					ultima_sync = null
				from itens_pluggy i, contas c
				where cp.id = $1 and i.id = cp.item_id and c.id = $2 and c.entidade_id = i.entidade_id`, id, *c.ContaID))
		case "ignorar":
			return exigirUma(tx.Exec(ctx, "update contas_pluggy set conta_id = null, ignorada = true where id = $1", id))
		case "desvincular":
			return exigirUma(tx.Exec(ctx, "update contas_pluggy set conta_id = null, ignorada = false, ultima_sync = null where id = $1", id))
		}
		return invalido("ação: vincular, criar, ignorar ou desvincular")
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sincronizarAgora roda a sincronização na hora (o worker faz a diária).
func (s *Servidor) sincronizarAgora(w http.ResponseWriter, r *http.Request) {
	u := sessaoDe(r).UsuarioID
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var recente bool
		err := tx.QueryRow(ctx, `select coalesce(ultima_sync > now() - interval '10 seconds', false) from conexoes_pluggy
			where usuario_id = app_usuario_id()`).Scan(&recente)
		if errors.Is(err, pgx.ErrNoRows) {
			return openfinance.ErrSemConexao
		}
		if err == nil && recente {
			return errAguarde
		}
		return err
	})
	if err != nil {
		falharOpenFinance(w, r, err)
		return
	}
	// um ano de extrato pode passar do limite de escrita padrão do servidor
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Minute))
	ctx, cancelar := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancelar()
	rel, err := s.OpenFinance.Sincronizar(ctx, u)
	if err != nil {
		falharOpenFinance(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, rel)
}

// acertarSaldo muda o saldo inicial para o calculado bater com o último saldo do
// banco (o histórico importado começou no meio do caminho).
func (s *Servidor) acertarSaldo(w http.ResponseWriter, r *http.Request) {
	var ajuste int64
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var banco, calculado int64
		err := tx.QueryRow(ctx, `select x.saldo_banco_centavos, app_saldo_conta(x.conta_id, x.data)
			from conciliacoes x join contas c on c.id = x.conta_id
			where x.conta_id = $1 and app_pode_editar_entidade(c.entidade_id)
			order by x.data desc limit 1`, r.PathValue("id")).Scan(&banco, &calculado)
		if err != nil {
			return err
		}
		ajuste = banco - calculado
		if _, err := tx.Exec(ctx, "update contas set saldo_inicial_centavos = saldo_inicial_centavos + $2 where id = $1",
			r.PathValue("id"), ajuste); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `update conciliacoes set saldo_calculado_centavos = saldo_banco_centavos
			where conta_id = $1 and data = (select max(data) from conciliacoes where conta_id = $1)`, r.PathValue("id")); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "saldo_acertado", r.PathValue("id"), map[string]any{"ajuste_centavos": ajuste})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]int64{"ajuste_centavos": ajuste})
}
