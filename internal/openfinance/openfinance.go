// Package openfinance liga o Meu Pluggy ao painel: guarda as credenciais de cada
// pessoa (cifradas, só ela vê), sincroniza contas e transações pela mesma fila dos
// arquivos (deduplicar, categorizar, pareamento) e concilia o saldo do banco com o
// calculado. As chamadas à Pluggy ficam fora das transações do banco de dados.
package openfinance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/cripto"
	"github.com/yarion1/pi-finance/internal/db"
	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/importadores"
	"github.com/yarion1/pi-finance/internal/pluggy"
)

var (
	// ErrSemConexao: a pessoa ainda não cadastrou as credenciais.
	ErrSemConexao = errors.New("cadastre o Client ID e o Secret do Meu Pluggy antes")
	// ErrItemRepetido: o item já está cadastrado.
	ErrItemRepetido = errors.New("esse banco já está conectado")
)

const (
	// janela de transações na primeira leitura de uma conta
	historicoInicial = 365
	// ao sincronizar de novo, relê alguns dias: lançamento pendente vira confirmado depois
	sobreposicao = 10
)

// Servico de Open Finance.
type Servico struct {
	Banco    db.Iniciador
	Cifrador *cripto.Cifrador
	Pluggy   *pluggy.Cliente
	// Hoje em São Paulo; os testes trocam.
	Hoje func() time.Time
	// Agora é o relógio de parede (ultima_sync).
	Agora func() time.Time
}

func (s *Servico) hoje() time.Time {
	if s.Hoje != nil {
		return s.Hoje()
	}
	return financeiro.Hoje()
}

func (s *Servico) agora() time.Time {
	if s.Agora != nil {
		return s.Agora()
	}
	return time.Now()
}

// Conectar valida as credenciais na Pluggy e guarda cifradas (uma por pessoa).
func (s *Servico) Conectar(ctx context.Context, usuarioID, clientID, clientSecret string) error {
	clientID, clientSecret = strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" || len(clientID) > 200 || len(clientSecret) > 200 {
		return fmt.Errorf("%w: preencha o Client ID e o Client Secret", pluggy.ErrCredenciais)
	}
	if _, err := s.Pluggy.Autenticar(ctx, clientID, clientSecret); err != nil {
		return err
	}
	idCifrado, err := s.Cifrador.Cifrar(clientID, "conexoes_pluggy.client_id")
	if err != nil {
		return err
	}
	secretCifrado, err := s.Cifrador.Cifrar(clientSecret, "conexoes_pluggy.client_secret")
	if err != nil {
		return err
	}
	fim := clientID[max(len(clientID)-4, 0):]
	return db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `insert into conexoes_pluggy (usuario_id, client_id, client_secret, client_id_fim)
			values (app_usuario_id(), $1, $2, $3)
			on conflict (usuario_id) do update set client_id = excluded.client_id, client_secret = excluded.client_secret,
				client_id_fim = excluded.client_id_fim, ultimo_erro = null`, idCifrado, secretCifrado, fim)
		return err
	})
}

// chave lê as credenciais da pessoa e pede uma chave de API.
func (s *Servico) chave(ctx context.Context, usuarioID string) (conexao, chave string, err error) {
	var idCifrado, secretCifrado string
	err = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select id, client_id, client_secret from conexoes_pluggy where usuario_id = app_usuario_id()").
			Scan(&conexao, &idCifrado, &secretCifrado)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrSemConexao
	}
	if err != nil {
		return "", "", err
	}
	clientID, err := s.Cifrador.Decifrar(idCifrado, "conexoes_pluggy.client_id")
	if err != nil {
		return "", "", err
	}
	secret, err := s.Cifrador.Decifrar(secretCifrado, "conexoes_pluggy.client_secret")
	if err != nil {
		return "", "", err
	}
	chave, err = s.Pluggy.Autenticar(ctx, clientID, secret)
	return conexao, chave, err
}

// AdicionarItem confere na Pluggy que o item é da pessoa e guarda para qual
// entidade vão as contas dele.
func (s *Servico) AdicionarItem(ctx context.Context, usuarioID, itemID, entidadeID string) (string, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" || len(itemID) > 100 {
		return "", pluggy.ErrItemNaoEncontrado
	}
	conexao, chave, err := s.chave(ctx, usuarioID)
	if err != nil {
		return "", err
	}
	item, err := s.Pluggy.Item(ctx, chave, itemID)
	if err != nil {
		return "", err
	}
	var id string
	err = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		var pode bool
		if err := tx.QueryRow(ctx, "select app_pode_editar_entidade($1)", entidadeID).Scan(&pode); err != nil || !pode {
			return financeiro.ErrContaNaoEditavel
		}
		err := tx.QueryRow(ctx, `insert into itens_pluggy (conexao_id, usuario_id, item_id, entidade_id, instituicao, status, atualizado_em)
			values ($1, app_usuario_id(), $2, $3, $4, $5, $6)
			on conflict (conexao_id, item_id) do nothing returning id`,
			conexao, itemID, entidadeID, item.Connector.Name, item.Status, item.AtualizadoEm()).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrItemRepetido
		}
		return err
	})
	return id, err
}

// Relatorio de uma sincronização.
type Relatorio struct {
	Itens         int      `json:"itens"`
	Contas        int      `json:"contas"`
	Pendentes     int      `json:"contas_sem_vinculo"`
	Novas         int      `json:"novas"`
	Casadas       int      `json:"casadas"`
	Divergencias  int      `json:"divergencias"`
	Transferencia int      `json:"transferencias"`
	Erros         []string `json:"erros,omitempty"`
}

type itemLocal struct {
	id, itemID, entidade string
	instituicao          *string
}

type contaLocal struct {
	id                string
	contaID           *string
	ignorada, acertar bool
	ultimaSync        *time.Time
	pluggy            pluggy.Conta
	saldo             *core.Centavos
}

// Sincronizar busca, para cada banco da pessoa, as contas e as transações novas e
// concilia os saldos. Erro de um banco não impede os outros; vai para o relatório
// e fica gravado no item.
func (s *Servico) Sincronizar(ctx context.Context, usuarioID string) (Relatorio, error) {
	var rel Relatorio
	conexao, chave, err := s.chave(ctx, usuarioID)
	if errors.Is(err, ErrSemConexao) {
		return rel, err
	}
	if err != nil {
		// credencial recusada ou Pluggy fora: fica registrado para a tela e o /api/health
		_ = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, "update conexoes_pluggy set ultima_sync = $2, ultimo_erro = $3 where id = $1",
				conexao, s.agora(), err.Error())
			return e
		})
		return rel, err
	}

	var itens []itemLocal
	err = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, "select id, item_id, entidade_id, instituicao from itens_pluggy where conexao_id = $1 order by criado_em", conexao)
		if err != nil {
			return err
		}
		defer linhas.Close()
		for linhas.Next() {
			var it itemLocal
			if err := linhas.Scan(&it.id, &it.itemID, &it.entidade, &it.instituicao); err != nil {
				return err
			}
			itens = append(itens, it)
		}
		return linhas.Err()
	})
	if err != nil {
		return rel, err
	}

	for _, it := range itens {
		rel.Itens++
		if err := s.sincronizarItem(ctx, usuarioID, chave, it, &rel); err != nil {
			nome := it.itemID
			if it.instituicao != nil && *it.instituicao != "" {
				nome = *it.instituicao
			}
			rel.Erros = append(rel.Erros, nome+": "+err.Error())
			_ = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
				_, e := tx.Exec(ctx, "update itens_pluggy set ultimo_erro = $2, ultima_sync = $3 where id = $1", it.id, err.Error(), s.agora())
				return e
			})
		}
	}
	err = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update conexoes_pluggy set ultima_sync = $2, ultimo_erro = null where id = $1", conexao, s.agora())
		return err
	})
	return rel, err
}

func (s *Servico) sincronizarItem(ctx context.Context, usuarioID, chave string, it itemLocal, rel *Relatorio) error {
	item, err := s.Pluggy.Item(ctx, chave, it.itemID)
	if err != nil {
		return err
	}
	contasPluggy, err := s.Pluggy.Contas(ctx, chave, it.itemID)
	if err != nil {
		return err
	}

	// 1) contas da Pluggy: grava saldo e dados; descobre quais estão vinculadas
	var contas []contaLocal
	err = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		var problema *string
		if p := item.Problema(); p != "" {
			problema = &p
		}
		if _, err := tx.Exec(ctx, `update itens_pluggy set instituicao = $2, status = $3, atualizado_em = $4, ultimo_erro = $5
			where id = $1`, it.id, item.Connector.Name, item.Status, item.AtualizadoEm(), problema); err != nil {
			return err
		}
		for _, pc := range contasPluggy {
			c := contaLocal{pluggy: pc}
			if v, err := pc.Saldo(); err == nil {
				c.saldo = &v
			}
			fech, venc := pc.DiasDoCartao()
			err := tx.QueryRow(ctx, `insert into contas_pluggy (item_id, usuario_id, pluggy_id, nome, tipo, subtipo, numero, moeda,
					saldo_centavos, limite_centavos, dia_fechamento, dia_vencimento, saldo_em, disponivel_centavos)
				values ($1, app_usuario_id(), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), $12)
				on conflict (item_id, pluggy_id) do update set nome = excluded.nome, tipo = excluded.tipo, subtipo = excluded.subtipo,
					numero = excluded.numero, moeda = excluded.moeda, saldo_centavos = excluded.saldo_centavos,
					limite_centavos = excluded.limite_centavos, dia_fechamento = excluded.dia_fechamento,
					dia_vencimento = excluded.dia_vencimento, saldo_em = now(), disponivel_centavos = excluded.disponivel_centavos
				returning id, conta_id, ignorada, acertar_saldo, ultima_sync`,
				it.id, pc.ID, pc.NomeExibido(), strings.ToUpper(pc.Type), pc.Subtype, pc.Number, pc.Moeda(),
				c.saldo, pc.Limite(), fech, venc, pc.Disponivel()).Scan(&c.id, &c.contaID, &c.ignorada, &c.acertar, &c.ultimaSync)
			if err != nil {
				return err
			}
			rel.Contas++
			if c.contaID == nil && !c.ignorada {
				rel.Pendentes++
			}
			contas = append(contas, c)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// investimentos do banco: viram ativos da entidade do item, com o saldo informado
	invs, err := s.sincronizarInvestimentos(ctx, usuarioID, chave, it, item.Connector.Name)
	if err != nil {
		return err
	}
	// faturas, empréstimos, identidade e movimentações dos investimentos (quando o banco oferece)
	s.sincronizarExtras(ctx, usuarioID, chave, it, contas, invs, rel)

	// 2) transações de cada conta vinculada, cada uma na sua transação do banco
	for _, c := range contas {
		if c.contaID == nil || c.ignorada {
			continue
		}
		desde := s.hoje().AddDate(0, 0, -historicoInicial)
		if c.ultimaSync != nil {
			desde = c.ultimaSync.In(time.UTC).AddDate(0, 0, -sobreposicao)
		}
		lista, err := s.Pluggy.Transacoes(ctx, chave, c.pluggy.ID, desde)
		if err != nil {
			return err
		}
		r := importadores.Resultado{Formato: importadores.FormatoPluggy, Moeda: c.pluggy.Moeda()}
		for _, t := range lista {
			l, ok, err := t.Linha(c.pluggy.Cartao())
			if err != nil {
				r.Avisos = append(r.Avisos, fmt.Sprintf("transação %s ignorada: %v", t.ID, err))
				continue
			}
			if ok {
				r.Linhas = append(r.Linhas, l)
			}
		}
		nomeArquivo := "Open Finance"
		if item.Connector.Name != "" {
			nomeArquivo += " · " + item.Connector.Name
		}
		err = db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
			// cartão: limite e dias vêm do banco antes de importar, para cada compra cair na fatura certa
			if c.pluggy.Cartao() {
				if err := acertarCartao(ctx, tx, *c.contaID, c.pluggy); err != nil {
					return err
				}
			}
			res, err := financeiro.Importar(ctx, tx, usuarioID, *c.contaID, nomeArquivo, r, false)
			if err != nil {
				return err
			}
			rel.Novas += res.Novas
			rel.Casadas += res.Casadas
			rel.Transferencia += res.Transferencias
			if res.Novas == 0 {
				// sincronização sem nada novo não polui o histórico de importações
				if _, err := tx.Exec(ctx, "delete from importacoes where id = $1", res.ImportacaoID); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, "update contas_pluggy set ultima_sync = $2, acertar_saldo = false where id = $1", c.id, s.agora()); err != nil {
				return err
			}
			divergiu, err := s.conciliar(ctx, tx, *c.contaID, c)
			if divergiu {
				rel.Divergencias++
			}
			return err
		})
		if err != nil {
			return err
		}
	}
	return db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update itens_pluggy set ultima_sync = $2 where id = $1", it.id, s.agora())
		return err
	})
}

// sincronizarInvestimentos grava cada investimento como ativo (chave: id da Pluggy).
// Os resgatados por inteiro ficam como encerrados, para o histórico.
func (s *Servico) sincronizarInvestimentos(ctx context.Context, usuarioID, chave string, it itemLocal, banco string) ([]pluggy.Investimento, error) {
	invs, err := s.Pluggy.Investimentos(ctx, chave, it.itemID)
	if err != nil {
		return nil, err
	}
	return invs, db.ComUsuario(ctx, s.Banco, usuarioID, func(tx pgx.Tx) error {
		for _, inv := range invs {
			saldo, _ := pluggy.Centavos(inv.Balance)
			var aplicado *core.Centavos
			if v, err := pluggy.Centavos(inv.AmountOriginal); err == nil && v > 0 {
				aplicado = &v
			}
			var venc *time.Time
			if d, err := pluggy.DataSP(inv.DueDate); err == nil {
				venc = &d
			}
			nome := strings.TrimSpace(inv.Name)
			if nome == "" {
				nome = inv.Code
			}
			// pelo conector MeuPluggy o nome do "banco" é o do conector: o emissor diz mais
			instituicao := banco
			if strings.Contains(strings.ToLower(strings.ReplaceAll(banco, " ", "")), "meupluggy") && inv.Issuer != "" {
				instituicao = inv.Issuer
			}
			moeda := strings.ToUpper(strings.TrimSpace(inv.CurrencyCode))
			if len(moeda) != 3 {
				moeda = "BRL"
			}
			if _, err := tx.Exec(ctx, `insert into ativos (entidade_id, codigo, nome, classe, emissor, vencimento, moeda, isento_ir,
					origem, pluggy_id, instituicao, subtipo, saldo_centavos, aplicado_centavos, saldo_em, encerrado)
				values ($1, $2, $3, $4::classe_ativo, $5, $6, $7, $8, 'pluggy', $9, $10, $11, $12, $13, now(), $14)
				on conflict (entidade_id, pluggy_id) where pluggy_id is not null do update set nome = excluded.nome,
					classe = excluded.classe, emissor = excluded.emissor, vencimento = excluded.vencimento,
					instituicao = excluded.instituicao, subtipo = excluded.subtipo, saldo_centavos = excluded.saldo_centavos,
					aplicado_centavos = excluded.aplicado_centavos, saldo_em = now(), encerrado = excluded.encerrado`,
				it.entidade, "pluggy:"+inv.ID, nome, inv.Classe(), nullSe(inv.Issuer), venc, moeda, inv.Isento(),
				inv.ID, nullSe(instituicao), nullSe(inv.Subtype), int64(saldo), aplicado, !inv.Ativo() || saldo == 0); err != nil {
				return err
			}
		}
		return nil
	})
}

func nullSe(s string) *string {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return &s
}

// acertarCartao copia do banco o limite (sempre) e os dias de fechamento e vencimento
// (quando a conta ainda não tem), e recalcula as faturas se os dias mudaram.
func acertarCartao(ctx context.Context, tx pgx.Tx, contaID string, pc pluggy.Conta) error {
	fech, venc := pc.DiasDoCartao()
	var mudouDias bool
	err := tx.QueryRow(ctx, `update contas c set limite_centavos = coalesce($2, c.limite_centavos),
			fechamento = coalesce(c.fechamento, $3), vencimento = coalesce(c.vencimento, $4)
		from contas antes where c.id = $1 and antes.id = c.id
		returning antes.fechamento is distinct from c.fechamento or antes.vencimento is distinct from c.vencimento`,
		contaID, pc.Limite(), fech, venc).Scan(&mudouDias)
	if err != nil || !mudouDias {
		return err
	}
	return financeiro.RecalcularFaturas(ctx, tx, contaID)
}

// conciliar compara o saldo do banco com o calculado até hoje. Conta criada pela
// Pluggy acerta o saldo inicial na primeira vez (o histórico dela começa no meio).
// Cartão fica de fora: o saldo da Pluggy e o nosso medem coisas diferentes
// (fatura x compras lançadas, com parcelas); DECISOES D22.
func (s *Servico) conciliar(ctx context.Context, tx pgx.Tx, contaID string, c contaLocal) (bool, error) {
	if c.saldo == nil || c.pluggy.Cartao() {
		return false, nil
	}
	hoje := s.hoje()
	var calculado int64
	var entidade, nome string
	if err := tx.QueryRow(ctx, "select app_saldo_conta($1, $2), entidade_id, nome from contas where id = $1", contaID, hoje).
		Scan(&calculado, &entidade, &nome); err != nil {
		return false, err
	}
	banco := int64(*c.saldo)
	if c.acertar && banco != calculado {
		if _, err := tx.Exec(ctx, "update contas set saldo_inicial_centavos = saldo_inicial_centavos + $2 where id = $1",
			contaID, banco-calculado); err != nil {
			return false, err
		}
		calculado = banco
	}
	if _, err := tx.Exec(ctx, `insert into conciliacoes (entidade_id, conta_id, data, saldo_banco_centavos, saldo_calculado_centavos)
		values ($1, $2, $3, $4, $5)
		on conflict (conta_id, data) do update set saldo_banco_centavos = excluded.saldo_banco_centavos,
			saldo_calculado_centavos = excluded.saldo_calculado_centavos, criada_em = now()`,
		entidade, contaID, hoje, banco, calculado); err != nil {
		return false, err
	}
	if banco == calculado {
		return false, nil
	}
	return true, financeiro.Alertar(ctx, tx, "saldo_divergente", map[string]any{
		"conta_id": contaID, "conta": nome, "data": hoje.Format("2006-01-02"),
		"banco_centavos": banco, "calculado_centavos": calculado, "diferenca_centavos": banco - calculado,
	}, "conta_id", "data", "diferenca_centavos")
}

// CriarConta cria no painel a conta que a Pluggy devolveu e vincula as duas. O saldo
// inicial é acertado com o do banco na próxima sincronização.
func CriarConta(ctx context.Context, tx pgx.Tx, contaPluggyID string) (string, error) {
	var entidade, nome, moeda string
	var pc pluggy.Conta
	var instituicao *string
	var limite *int64
	var fech, venc *int16
	err := tx.QueryRow(ctx, `select i.entidade_id, cp.nome, cp.tipo, coalesce(cp.subtipo, ''), cp.moeda, i.instituicao,
			cp.limite_centavos, cp.dia_fechamento, cp.dia_vencimento
		from contas_pluggy cp join itens_pluggy i on i.id = cp.item_id
		where cp.id = $1 and cp.conta_id is null`, contaPluggyID).
		Scan(&entidade, &nome, &pc.Type, &pc.Subtype, &moeda, &instituicao, &limite, &fech, &venc)
	if err != nil {
		return "", err
	}
	var instituicaoID *string
	if instituicao != nil && *instituicao != "" {
		var id string
		if err := tx.QueryRow(ctx, `select id from instituicoes
			where lower($1) = lower(nome) or lower($1) like '%' || lower(nome) || '%'
			order by length(nome) desc limit 1`, *instituicao).Scan(&id); err == nil {
			instituicaoID = &id
		}
	}
	if pc.TipoConta() != "cartao" {
		limite, fech, venc = nil, nil, nil
	}
	var contaID string
	if err := tx.QueryRow(ctx, `insert into contas (entidade_id, instituicao_id, nome, tipo, moeda, limite_centavos, fechamento, vencimento)
		values ($1, $2, $3, $4, $5, $6, $7, $8) returning id`,
		entidade, instituicaoID, nome, pc.TipoConta(), moeda, limite, fech, venc).Scan(&contaID); err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, "update contas_pluggy set conta_id = $2, acertar_saldo = true, ignorada = false where id = $1", contaPluggyID, contaID)
	return contaID, err
}
