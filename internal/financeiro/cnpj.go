package financeiro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// ---------------------------------------------------------------------------
// Regras fiscais do CNPJ, vigentes numa data
// ---------------------------------------------------------------------------

// regrasFiscais carregadas uma vez por requisição.
type regrasFiscais struct{ todas []core.Regra }

func carregarRegrasFiscais(ctx context.Context, tx pgx.Tx) (*regrasFiscais, error) {
	linhas, err := tx.Query(ctx, `select chave, valor_json::text, vigente_de, vigente_ate, fonte from regras_fiscais`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	r := &regrasFiscais{}
	for linhas.Next() {
		var x core.Regra
		var v string
		if err := linhas.Scan(&x.Chave, &v, &x.VigenteDe, &x.VigenteAte, &x.Fonte); err != nil {
			return nil, err
		}
		x.Valor = []byte(v)
		r.todas = append(r.todas, x)
	}
	return r, linhas.Err()
}

// ler a regra vigente na data para dentro de destino.
func (r *regrasFiscais) ler(chave string, data time.Time, destino any) error {
	rg, err := core.RegraVigente(r.todas, chave, data)
	if err != nil {
		return fmt.Errorf("regra %s em %s: %w", chave, data.Format(formatoData), err)
	}
	return json.Unmarshal(rg.Valor, destino)
}

// RegrasCNPJ usadas nos cálculos de uma data.
type RegrasCNPJ struct {
	TetoMEI struct {
		Anual      core.Centavos `json:"anual_centavos"`
		Mensal     core.Centavos `json:"mensal_proporcional_centavos"`
		Tolerancia string        `json:"tolerancia"`
	}
	DASMEI       core.RegrasDASMEI
	LucroMEI     struct{ Servicos, Comercio string }
	FaixasIII    []core.FaixaSimples
	FaixasV      []core.FaixaSimples
	PartilhaIII  []map[string]string
	PartilhaV    []map[string]string
	ISSMaximo    string
	FatorRMinimo string
	TetoSimples  core.Centavos
	Sublimite    core.Centavos
	INSS         core.RegrasINSS
	IRRF         core.TabelaIRRF
	SalarioMin   core.Centavos
	Dividendos   core.LimiteDividendos
}

func (r *regrasFiscais) cnpj(data time.Time) (RegrasCNPJ, error) {
	var x RegrasCNPJ
	var iss, fator struct {
		Percentual string `json:"percentual"`
	}
	var teto, sub, sm struct {
		Anual    core.Centavos `json:"anual_centavos"`
		Centavos core.Centavos `json:"centavos"`
	}
	var lucro struct {
		Servicos string `json:"servicos"`
		Comercio string `json:"comercio"`
	}
	for chave, destino := range map[string]any{
		"teto_mei": &x.TetoMEI, "das_mei": &x.DASMEI, "lucro_presumido_mei": &lucro,
		"faixas_anexo_iii": &x.FaixasIII, "faixas_anexo_v": &x.FaixasV,
		"partilha_anexo_iii": &x.PartilhaIII, "partilha_anexo_v": &x.PartilhaV,
		"iss_maximo_simples": &iss, "fator_r_minimo": &fator, "teto_simples": &teto, "sublimite_iss": &sub,
		"inss_prolabore": &x.INSS, "tabela_irrf": &x.IRRF, "salario_minimo": &sm, "limite_dividendos_mes": &x.Dividendos,
	} {
		if err := r.ler(chave, data, destino); err != nil {
			return x, err
		}
	}
	x.LucroMEI.Servicos, x.LucroMEI.Comercio = lucro.Servicos, lucro.Comercio
	x.ISSMaximo, x.FatorRMinimo = iss.Percentual, fator.Percentual
	x.TetoSimples, x.Sublimite, x.SalarioMin = teto.Anual, sub.Anual, sm.Centavos
	return x, nil
}

// ---------------------------------------------------------------------------
// Painel do CNPJ
// ---------------------------------------------------------------------------

// ErrNaoEPJ: a entidade pedida é pessoa física.
var ErrNaoEPJ = errors.New("esta entidade não é um CNPJ")

// EntidadePJ com o que os cálculos precisam.
type EntidadePJ struct {
	ID           string     `json:"id"`
	Nome         string     `json:"nome"`
	Regime       string     `json:"regime"` // MEI, SIMPLES_ME, SIMPLES_EPP
	Atividade    string     `json:"atividade"`
	DataAbertura *time.Time `json:"data_abertura"`
	PodeEditar   bool       `json:"pode_editar"`
}

func lerEntidadePJ(ctx context.Context, tx pgx.Tx, id string) (EntidadePJ, error) {
	var e EntidadePJ
	var tipo string
	var regime, atividade *string
	err := tx.QueryRow(ctx, `select id, nome, tipo::text, regime::text, atividade, data_abertura, app_pode_editar_entidade(id)
		from entidades where id = $1`, id).Scan(&e.ID, &e.Nome, &tipo, &regime, &atividade, &e.DataAbertura, &e.PodeEditar)
	if err != nil {
		return e, err
	}
	if tipo != "PJ" {
		return e, ErrNaoEPJ
	}
	e.Regime, e.Atividade = "MEI", "servicos"
	if regime != nil {
		e.Regime = *regime
	}
	if atividade != nil {
		e.Atividade = *atividade
	}
	return e, nil
}

// ReceitaMes: soma das receitas (notas não canceladas) do mês.
type ReceitaMes struct {
	Mes        string        `json:"mes"` // AAAA-MM
	Receita    core.Centavos `json:"receita_centavos"`
	Exportacao core.Centavos `json:"exportacao_centavos"`
	Servicos   core.Centavos `json:"servicos_centavos"`
	Comercio   core.Centavos `json:"comercio_centavos"`
}

// receitasPorMes de [de, ate), mês a mês (os meses sem receita vêm com zero).
func receitasPorMes(ctx context.Context, tx pgx.Tx, entidadeID string, de, ate time.Time) ([]ReceitaMes, error) {
	linhas, err := tx.Query(ctx, `select to_char(data_emissao, 'YYYY-MM'),
			coalesce(sum(valor_centavos), 0),
			coalesce(sum(valor_centavos) filter (where exportacao), 0),
			coalesce(sum(valor_centavos) filter (where atividade = 'servicos'), 0),
			coalesce(sum(valor_centavos) filter (where atividade = 'comercio'), 0)
		from notas_fiscais where entidade_id = $1 and not cancelada and data_emissao >= $2 and data_emissao < $3
		group by 1`, entidadeID, de, ate)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	porMes := map[string]ReceitaMes{}
	for linhas.Next() {
		var m ReceitaMes
		if err := linhas.Scan(&m.Mes, &m.Receita, &m.Exportacao, &m.Servicos, &m.Comercio); err != nil {
			return nil, err
		}
		porMes[m.Mes] = m
	}
	if err := linhas.Err(); err != nil {
		return nil, err
	}
	var out []ReceitaMes
	for m := inicioMes(de); m.Before(ate); m = m.AddDate(0, 1, 0) {
		k := m.Format("2006-01")
		x, ok := porMes[k]
		if !ok {
			x = ReceitaMes{Mes: k}
		}
		out = append(out, x)
	}
	return out, nil
}

func inicioMes(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC) }

func somaReceita(ms []ReceitaMes) (total core.Centavos) {
	for _, m := range ms {
		total += m.Receita
	}
	return total
}

// ClienteConcentracao: participação de um cliente na receita dos 12 meses.
type ClienteConcentracao struct {
	ID         *string       `json:"id"`
	Nome       string        `json:"nome"`
	Pais       string        `json:"pais"`
	Receita    core.Centavos `json:"receita_centavos"`
	Percentual float64       `json:"percentual"`
}

// DASMes: um DAS de competência (pago ou não).
type DASMes struct {
	Competencia string        `json:"competencia"` // AAAA-MM
	Vencimento  string        `json:"vencimento"`
	Valor       core.Centavos `json:"valor_centavos"`
	PagoEm      *string       `json:"pago_em"`
	Atrasado    bool          `json:"atrasado"`
}

// PainelMEI do ano corrente.
type PainelMEI struct {
	Projecao    core.ProjecaoMEI `json:"projecao"`
	Situacao    string           `json:"situacao"`
	Receita12m  core.Centavos    `json:"receita_12m_centavos"`
	Tolerancia  core.Centavos    `json:"teto_com_tolerancia_centavos"`
	DAS         core.Centavos    `json:"das_centavos"`
	DASMeses    []DASMes         `json:"das_meses"`
	Lucro       core.LucroMEI    `json:"lucro"`
	DASNAno     int              `json:"dasn_ano"`
	DASNReceita core.Centavos    `json:"dasn_receita_centavos"`
	DASNPrazo   string           `json:"dasn_prazo"`
	Exportacao  core.Centavos    `json:"exportacao_ano_centavos"`
}

// PainelSimples do mês corrente.
type PainelSimples struct {
	RBT12         core.Centavos     `json:"rbt12_centavos"`
	Folha12       core.Centavos     `json:"folha_12m_centavos"`
	FatorR        string            `json:"fator_r"`
	FatorRMinimo  string            `json:"fator_r_minimo"`
	Anexo         string            `json:"anexo"`
	DAS           core.ResultadoDAS `json:"das"`
	ProlaboreMin  core.Centavos     `json:"prolabore_para_fator_r_centavos"`
	AcimaSublimte bool              `json:"acima_sublimite"`
	DASMeses      []DASMes          `json:"das_meses"`
}

// PainelCNPJ: o que a tela do CNPJ mostra.
type PainelCNPJ struct {
	Entidade     EntidadePJ            `json:"entidade"`
	Hoje         string                `json:"hoje"`
	Meses        []ReceitaMes          `json:"meses"` // do ano corrente
	ReceitaAno   core.Centavos         `json:"receita_ano_centavos"`
	ReceitaMes   core.Centavos         `json:"receita_mes_centavos"`
	Despesas     core.Centavos         `json:"despesas_ano_centavos"`
	Clientes     []ClienteConcentracao `json:"clientes"`
	MEI          *PainelMEI            `json:"mei,omitempty"`
	Simples      *PainelSimples        `json:"simples,omitempty"`
	Alertas      []string              `json:"alertas"`
	Fonte        string                `json:"fonte"`
	LucroMesPrev core.Centavos         `json:"lucro_distribuivel_mes_centavos"`
}

// despesasPJ: gastos das contas da entidade no período (positivo).
func despesasPJ(ctx context.Context, tx pgx.Tx, entidadeID string, de, ate time.Time) (core.Centavos, error) {
	var v int64
	err := tx.QueryRow(ctx, `select coalesce(-sum(valor_centavos), 0) from transacoes
		where entidade_id = $1 and tipo = 'gasto' and data >= $2 and data < $3`, entidadeID, de, ate).Scan(&v)
	return core.Centavos(v), err
}

func dasMeses(ctx context.Context, tx pgx.Tx, entidadeID string, ano int, hoje time.Time, valorPadrao func(mes time.Time) core.Centavos, diaVenc int) ([]DASMes, error) {
	linhas, err := tx.Query(ctx, `select to_char(competencia, 'YYYY-MM'), das_centavos, to_char(pago_em, 'YYYY-MM-DD')
		from apuracoes_simples where entidade_id = $1 and extract(year from competencia) = $2`, entidadeID, ano)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	gravados := map[string]DASMes{}
	for linhas.Next() {
		var d DASMes
		if err := linhas.Scan(&d.Competencia, &d.Valor, &d.PagoEm); err != nil {
			return nil, err
		}
		gravados[d.Competencia] = d
	}
	if err := linhas.Err(); err != nil {
		return nil, err
	}
	var out []DASMes
	for m := time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC); m.Year() == ano && !m.After(inicioMes(hoje)); m = m.AddDate(0, 1, 0) {
		k := m.Format("2006-01")
		d, ok := gravados[k]
		if !ok {
			d = DASMes{Competencia: k, Valor: valorPadrao(m)}
		}
		venc := time.Date(m.Year(), m.Month()+1, diaVenc, 0, 0, 0, 0, time.UTC)
		d.Vencimento = venc.Format(formatoData)
		d.Atrasado = d.PagoEm == nil && venc.Before(dia(hoje))
		out = append(out, d)
	}
	return out, nil
}

// folha12 dos 12 meses antes do mês (pró-labore + salários).
func folha12(ctx context.Context, tx pgx.Tx, entidadeID string, mes time.Time) (core.Centavos, error) {
	var v int64
	err := tx.QueryRow(ctx, `select coalesce(sum(prolabore_centavos + salarios_centavos), 0) from folha
		where entidade_id = $1 and competencia >= $2 and competencia < $3`,
		entidadeID, mes.AddDate(-1, 0, 0), mes).Scan(&v)
	return core.Centavos(v), err
}

// CalcularPainelCNPJ na data.
func CalcularPainelCNPJ(ctx context.Context, tx pgx.Tx, entidadeID string, hoje time.Time) (PainelCNPJ, error) {
	hoje = dia(hoje)
	p := PainelCNPJ{Hoje: hoje.Format(formatoData), Alertas: []string{}, Clientes: []ClienteConcentracao{}}
	e, err := lerEntidadePJ(ctx, tx, entidadeID)
	if err != nil {
		return p, err
	}
	p.Entidade = e
	rf, err := carregarRegrasFiscais(ctx, tx)
	if err != nil {
		return p, err
	}
	rg, err := rf.cnpj(hoje)
	if err != nil {
		return p, err
	}
	p.Fonte = "regras_fiscais vigentes em " + p.Hoje

	inicioAno := time.Date(hoje.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	fimMes := inicioMes(hoje).AddDate(0, 1, 0)
	if p.Meses, err = receitasPorMes(ctx, tx, entidadeID, inicioAno, fimMes); err != nil {
		return p, err
	}
	p.ReceitaAno = somaReceita(p.Meses)
	p.ReceitaMes = p.Meses[len(p.Meses)-1].Receita
	if p.Despesas, err = despesasPJ(ctx, tx, entidadeID, inicioAno, hoje.AddDate(0, 0, 1)); err != nil {
		return p, err
	}
	doze, err := receitasPorMes(ctx, tx, entidadeID, fimMes.AddDate(-1, 0, 0), fimMes)
	if err != nil {
		return p, err
	}
	receita12 := somaReceita(doze)

	// concentração de clientes nos 12 meses
	linhas, err := tx.Query(ctx, `select c.id, coalesce(c.nome, 'Sem cliente'), coalesce(c.pais, 'BR'), sum(n.valor_centavos)
		from notas_fiscais n left join clientes c on c.id = n.cliente_id
		where n.entidade_id = $1 and not n.cancelada and n.data_emissao >= $2 and n.data_emissao < $3
		group by 1, 2, 3 order by 4 desc limit 10`, entidadeID, fimMes.AddDate(-1, 0, 0), fimMes)
	if err != nil {
		return p, err
	}
	for linhas.Next() {
		var c ClienteConcentracao
		if err := linhas.Scan(&c.ID, &c.Nome, &c.Pais, &c.Receita); err != nil {
			linhas.Close()
			return p, err
		}
		if receita12 > 0 {
			c.Percentual = float64(c.Receita) / float64(receita12)
		}
		p.Clientes = append(p.Clientes, c)
	}
	linhas.Close()
	if len(p.Clientes) > 0 && p.Clientes[0].ID != nil && p.Clientes[0].Percentual > 0.7 {
		p.Alertas = append(p.Alertas, fmt.Sprintf("%s concentra %.0f %% da receita dos últimos 12 meses.",
			p.Clientes[0].Nome, p.Clientes[0].Percentual*100))
	}

	if e.Regime == "MEI" {
		m := &PainelMEI{Receita12m: receita12}
		teto := core.TetoMEI(e.DataAbertura, hoje.Year(), rg.TetoMEI.Anual, rg.TetoMEI.Mensal)
		m.Projecao = core.ProjetarMEI(p.ReceitaAno, teto, hoje, e.DataAbertura)
		m.Situacao = core.SituacaoMEI(p.ReceitaAno, teto, rg.TetoMEI.Tolerancia)
		m.Tolerancia = core.Centavos(float64(teto) * 1.2)
		if tol, ok := parseFracao(rg.TetoMEI.Tolerancia); ok {
			m.Tolerancia = teto + core.Centavos(float64(teto)*tol+0.5)
		}
		m.DAS = core.DASMEI(rg.DASMEI, e.Atividade)
		dia := rg.DASMEI.VencimentoDia
		if dia == 0 {
			dia = 20
		}
		desde := inicioAno
		if e.DataAbertura != nil && e.DataAbertura.After(desde) {
			desde = inicioMes(*e.DataAbertura)
		}
		if m.DASMeses, err = dasMeses(ctx, tx, entidadeID, hoje.Year(), hoje, func(mes time.Time) core.Centavos {
			if mes.Before(desde) {
				return 0
			}
			return m.DAS
		}, dia); err != nil {
			return p, err
		}
		var serv, com core.Centavos
		for _, x := range p.Meses {
			serv += x.Servicos
			com += x.Comercio
			m.Exportacao += x.Exportacao
		}
		m.Lucro = core.CalcularLucroMEI(serv, com, p.Despesas, rg.LucroMEI.Servicos, rg.LucroMEI.Comercio)
		m.DASNAno = hoje.Year() - 1
		anterior, err := receitasPorMes(ctx, tx, entidadeID, time.Date(m.DASNAno, 1, 1, 0, 0, 0, 0, time.UTC), inicioAno)
		if err != nil {
			return p, err
		}
		m.DASNReceita = somaReceita(anterior)
		m.DASNPrazo = fmt.Sprintf("%d-05-31", hoje.Year())
		switch m.Situacao {
		case core.MEIAtencao:
			p.Alertas = append(p.Alertas, "Faturamento passou de 70 % do teto do MEI.")
		case core.MEIPlanejar:
			p.Alertas = append(p.Alertas, "Faturamento passou de 90 % do teto: hora de planejar a migração para ME.")
		case core.MEIViraME:
			p.Alertas = append(p.Alertas, "Passou do teto do MEI (até 20 %): o CNPJ vira ME em janeiro. Fale com o contador.")
		case core.MEIDesenquadrado:
			p.Alertas = append(p.Alertas, "Passou mais de 20 % do teto: desenquadramento retroativo. Fale com o contador já.")
		}
		if m.Projecao.DataTeto != nil && m.Situacao != core.MEIViraME && m.Situacao != core.MEIDesenquadrado {
			p.Alertas = append(p.Alertas, "No ritmo atual, o faturamento bate o teto do MEI em "+
				m.Projecao.DataTeto.Format("02/01/2006")+".")
		}
		atrasados := 0
		for _, d := range m.DASMeses {
			if d.Atrasado && d.Valor > 0 {
				atrasados++
			}
		}
		if atrasados > 0 {
			p.Alertas = append(p.Alertas, fmt.Sprintf("%d DAS-MEI em atraso.", atrasados))
		}
		p.MEI = m
		p.LucroMesPrev = p.ReceitaMes - m.DAS
	} else {
		s := &PainelSimples{FatorRMinimo: rg.FatorRMinimo}
		mes := inicioMes(hoje)
		anteriores, err := receitasPorMes(ctx, tx, entidadeID, mes.AddDate(-1, 0, 0), mes)
		if err != nil {
			return p, err
		}
		// meses anteriores de atividade (no início de atividade, RBT12 proporcional)
		var valores []core.Centavos
		for _, a := range anteriores {
			m0, _ := time.Parse("2006-01", a.Mes)
			if e.DataAbertura != nil && m0.Before(inicioMes(*e.DataAbertura)) {
				continue
			}
			valores = append(valores, a.Receita)
		}
		s.RBT12 = core.RBT12(valores, p.ReceitaMes)
		if s.Folha12, err = folha12(ctx, tx, entidadeID, mes); err != nil {
			return p, err
		}
		s.FatorR = core.FatorR(s.Folha12, s.RBT12)
		s.Anexo = core.AnexoPeloFatorR(s.Folha12, s.RBT12, rg.FatorRMinimo)
		s.DAS, err = dasDoMes(rg, s.Anexo, s.RBT12, p.Meses[len(p.Meses)-1])
		if err != nil && !errors.Is(err, core.ErrAcimaDoSimples) {
			return p, err
		}
		if errors.Is(err, core.ErrAcimaDoSimples) {
			p.Alertas = append(p.Alertas, "Receita dos últimos 12 meses acima do teto do Simples (R$ 4,8 milhões).")
		}
		s.AcimaSublimte = s.RBT12 > rg.Sublimite
		if s.AcimaSublimte {
			p.Alertas = append(p.Alertas, "Acima do sublimite de R$ 3,6 milhões: ISS e ICMS saem do DAS.")
		}
		s.ProlaboreMin = core.ProlaboreParaFatorR(s.RBT12, 0, rg.SalarioMin, rg.FatorRMinimo)
		if s.Anexo == core.AnexoV {
			p.Alertas = append(p.Alertas, "Fator R abaixo de 28 %: o DAS sai pelo Anexo V. Veja o simulador de pró-labore.")
		}
		if s.DASMeses, err = dasMeses(ctx, tx, entidadeID, hoje.Year(), hoje, func(time.Time) core.Centavos { return 0 }, 20); err != nil {
			return p, err
		}
		p.Simples = s
		p.LucroMesPrev = p.ReceitaMes - s.DAS.DAS
	}
	return p, nil
}

func parseFracao(s string) (float64, bool) {
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil {
		return 0, false
	}
	return f, true
}

func dasDoMes(rg RegrasCNPJ, anexo string, rbt12 core.Centavos, m ReceitaMes) (core.ResultadoDAS, error) {
	faixas, partilha := rg.FaixasV, rg.PartilhaV
	if anexo == core.AnexoIII {
		faixas, partilha = rg.FaixasIII, rg.PartilhaIII
	}
	return core.CalcularDAS(core.ParametrosDAS{Anexo: anexo, Faixas: faixas, Partilha: partilha, ISSMaximo: rg.ISSMaximo,
		RBT12: rbt12, Receita: m.Receita, Exportacao: m.Exportacao})
}

// ---------------------------------------------------------------------------
// Simulador: continuar MEI ou migrar para ME
// ---------------------------------------------------------------------------

// CenarioCNPJ de custo anual.
type CenarioCNPJ struct {
	Nome            string        `json:"nome"`
	Possivel        bool          `json:"possivel"`
	Anexo           string        `json:"anexo,omitempty"`
	AliquotaEfetiva string        `json:"aliquota_efetiva,omitempty"`
	Impostos        core.Centavos `json:"impostos_centavos"` // DAS dos 12 meses
	Prolabore       core.Centavos `json:"prolabore_mensal_centavos"`
	INSS            core.Centavos `json:"inss_anual_centavos"`
	IRRF            core.Centavos `json:"irrf_anual_centavos"`
	Contador        core.Centavos `json:"contador_anual_centavos"`
	Total           core.Centavos `json:"total_anual_centavos"`
	Mensal          core.Centavos `json:"total_mensal_centavos"`
	Observacao      string        `json:"observacao"`
}

// Simulacao dos próximos 12 meses com uma receita mensal constante.
type Simulacao struct {
	ReceitaMensal core.Centavos `json:"receita_mensal_centavos"`
	ReceitaAnual  core.Centavos `json:"receita_anual_centavos"`
	TetoMEI       core.Centavos `json:"teto_mei_centavos"`
	MaximoMEI     core.Centavos `json:"receita_mensal_maxima_mei_centavos"`
	Cenarios      []CenarioCNPJ `json:"cenarios"`
	Melhor        string        `json:"melhor"`
}

// Simular: MEI (DAS fixo) × ME no Anexo V com pró-labore de um salário mínimo × ME no
// Anexo III com o pró-labore que leva o Fator R a 28 %. Custos de ME: DAS, INSS do
// sócio, IRRF do pró-labore e o contador. O RBT12 é a receita anual projetada.
func Simular(ctx context.Context, tx pgx.Tx, atividade string, receitaMensal, contadorMensal core.Centavos, hoje time.Time) (Simulacao, error) {
	s := Simulacao{ReceitaMensal: receitaMensal, ReceitaAnual: receitaMensal * 12}
	rf, err := carregarRegrasFiscais(ctx, tx)
	if err != nil {
		return s, err
	}
	rg, err := rf.cnpj(hoje)
	if err != nil {
		return s, err
	}
	s.TetoMEI, s.MaximoMEI = rg.TetoMEI.Anual, rg.TetoMEI.Mensal

	das := core.DASMEI(rg.DASMEI, atividade)
	mei := CenarioCNPJ{Nome: "Continuar MEI", Possivel: s.ReceitaAnual <= rg.TetoMEI.Anual, Impostos: das * 12}
	mei.Total, mei.Mensal = mei.Impostos, das
	if !mei.Possivel {
		mei.Observacao = "A receita projetada passa do teto do MEI."
	}
	s.Cenarios = append(s.Cenarios, mei)

	me := func(nome, anexo string, prolabore core.Centavos) CenarioCNPJ {
		c := CenarioCNPJ{Nome: nome, Anexo: anexo, Possivel: true, Prolabore: prolabore, Contador: contadorMensal * 12}
		r, err := dasDoMes(rg, anexo, s.ReceitaAnual, ReceitaMes{Receita: receitaMensal})
		if err != nil {
			c.Possivel, c.Observacao = false, "Receita acima do teto do Simples."
			return c
		}
		c.AliquotaEfetiva, c.Impostos = r.AliquotaEfetiva, r.DAS*12
		inss := core.INSSProlabore(prolabore, rg.INSS)
		c.INSS = inss * 12
		c.IRRF = core.IRRF(prolabore, inss, 0, rg.IRRF) * 12
		c.Total = c.Impostos + c.INSS + c.IRRF + c.Contador
		c.Mensal = core.Centavos((int64(c.Total) + 6) / 12)
		return c
	}
	v := me("ME no Anexo V (pró-labore de 1 salário mínimo)", core.AnexoV, rg.SalarioMin)
	pl := core.ProlaboreParaFatorR(s.ReceitaAnual, 0, rg.SalarioMin, rg.FatorRMinimo)
	iii := me("ME no Anexo III (Fator R de 28 %)", core.AnexoIII, pl)
	iii.Observacao = fmt.Sprintf("Pró-labore de %s por mês para o Fator R.", core.FormatarBRL(pl))
	if atividade == "comercio" {
		v.Observacao = "Comércio usa o Anexo I, que o painel ainda não calcula: compare com o contador."
		iii.Observacao = v.Observacao
	}
	s.Cenarios = append(s.Cenarios, v, iii)

	melhor := -1
	for i, c := range s.Cenarios {
		if c.Possivel && (melhor < 0 || c.Total < s.Cenarios[melhor].Total) {
			melhor = i
		}
	}
	if melhor >= 0 {
		s.Melhor = s.Cenarios[melhor].Nome
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Receitas (notas), folha, DAS pago e lucros
// ---------------------------------------------------------------------------

// Nota é uma receita do CNPJ.
type Nota struct {
	ID              string         `json:"id"`
	EntidadeID      string         `json:"entidade_id"`
	ClienteID       *string        `json:"cliente_id"`
	Cliente         *string        `json:"cliente"`
	Pais            *string        `json:"pais"`
	Numero          *string        `json:"numero"`
	DataEmissao     string         `json:"data_emissao"`
	DataRecebimento *string        `json:"data_recebimento"`
	Descricao       *string        `json:"descricao"`
	Atividade       string         `json:"atividade"`
	Valor           core.Centavos  `json:"valor_centavos"`
	Moeda           string         `json:"moeda"`
	ValorMoeda      *core.Centavos `json:"valor_moeda_centavos"`
	TaxaCambio      *string        `json:"taxa_cambio"`
	Exportacao      bool           `json:"exportacao"`
	Cancelada       bool           `json:"cancelada"`
	Origem          string         `json:"origem"`
}

// ListarNotas da entidade no ano (mais recentes primeiro).
func ListarNotas(ctx context.Context, tx pgx.Tx, entidadeID string, ano int) ([]Nota, error) {
	linhas, err := tx.Query(ctx, `select n.id, n.entidade_id, n.cliente_id, c.nome, c.pais, n.numero,
			to_char(n.data_emissao, 'YYYY-MM-DD'), to_char(n.data_recebimento, 'YYYY-MM-DD'), n.descricao, n.atividade,
			n.valor_centavos, n.moeda, n.valor_moeda_centavos, n.taxa_cambio::text, n.exportacao, n.cancelada, n.origem
		from notas_fiscais n left join clientes c on c.id = n.cliente_id
		where n.entidade_id = $1 and extract(year from n.data_emissao) = $2
		order by n.data_emissao desc, n.criada_em desc`, entidadeID, ano)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	lista := []Nota{}
	for linhas.Next() {
		var n Nota
		if err := linhas.Scan(&n.ID, &n.EntidadeID, &n.ClienteID, &n.Cliente, &n.Pais, &n.Numero, &n.DataEmissao,
			&n.DataRecebimento, &n.Descricao, &n.Atividade, &n.Valor, &n.Moeda, &n.ValorMoeda, &n.TaxaCambio,
			&n.Exportacao, &n.Cancelada, &n.Origem); err != nil {
			return nil, err
		}
		if n.TaxaCambio != nil {
			t := limparDec(*n.TaxaCambio)
			n.TaxaCambio = &t
		}
		lista = append(lista, n)
	}
	return lista, linhas.Err()
}

// DadosNota para lançar uma receita. Em moeda estrangeira, informe valor_moeda e o
// câmbio (ou deixe o câmbio vazio para usar a PTAX do recebimento).
type DadosNota struct {
	EntidadeID       string  `json:"entidade_id"`
	Cliente          string  `json:"cliente"`
	Pais             string  `json:"pais"`
	Numero           string  `json:"numero"`
	ChaveAcesso      string  `json:"-"`
	DataEmissao      string  `json:"data_emissao"`
	DataRecebimento  string  `json:"data_recebimento"`
	Descricao        string  `json:"descricao"`
	Atividade        string  `json:"atividade"`
	Valor            int64   `json:"valor_centavos"`
	Moeda            string  `json:"moeda"`
	ValorMoeda       int64   `json:"valor_moeda_centavos"`
	TaxaCambio       string  `json:"taxa_cambio"`
	Exportacao       bool    `json:"exportacao"`
	Origem           string  `json:"-"`
	DocumentoCliente *string `json:"-"` // já cifrado
}

// ErrSemCambio: moeda estrangeira sem câmbio informado nem PTAX do dia.
var ErrSemCambio = ErrCampo{"informe o câmbio: não há PTAX guardada para a data do recebimento"}

// CriarNota grava a receita (e o cliente, se novo). Devolve o id; com chave de acesso
// repetida, devolve "" e nenhum erro (já importada).
func CriarNota(ctx context.Context, tx pgx.Tx, d DadosNota) (string, error) {
	e, err := lerEntidadePJ(ctx, tx, d.EntidadeID)
	if err != nil {
		return "", err
	}
	if !e.PodeEditar {
		return "", ErrSemPermissao
	}
	emissao, err := time.Parse(formatoData, d.DataEmissao)
	if err != nil {
		return "", ErrCampo{"data de emissão inválida"}
	}
	var recebimento *time.Time
	if d.DataRecebimento != "" {
		r, err := time.Parse(formatoData, d.DataRecebimento)
		if err != nil {
			return "", ErrCampo{"data de recebimento inválida"}
		}
		recebimento = &r
	}
	if d.Atividade == "" {
		d.Atividade = "servicos"
		if e.Atividade == "comercio" {
			d.Atividade = "comercio"
		}
	}
	if d.Atividade != "servicos" && d.Atividade != "comercio" {
		return "", ErrCampo{"atividade: servicos ou comercio"}
	}
	d.Moeda = strings.ToUpper(strings.TrimSpace(d.Moeda))
	if d.Moeda == "" {
		d.Moeda = "BRL"
	}
	if len(d.Moeda) != 3 {
		return "", ErrCampo{"moeda inválida"}
	}
	d.Pais = strings.ToUpper(strings.TrimSpace(d.Pais))
	if d.Pais == "" {
		d.Pais = "BR"
	}
	if len(d.Pais) != 2 {
		return "", ErrCampo{"país: código de 2 letras (BR, US...)"}
	}
	if len(d.Descricao) > 500 || len(d.Numero) > 60 || len(d.Cliente) > 200 {
		return "", ErrCampo{"texto longo demais"}
	}
	var valorMoeda *int64
	var taxa *string
	if d.Moeda != "BRL" {
		if d.ValorMoeda <= 0 {
			return "", ErrCampo{"informe o valor na moeda da nota"}
		}
		if d.TaxaCambio == "" {
			// PTAX do recebimento (ou da emissão), do último dia útil até a data
			base := emissao
			if recebimento != nil {
				base = *recebimento
			}
			if d.Moeda != "USD" {
				return "", ErrCampo{"informe o câmbio para moedas diferentes do dólar"}
			}
			err := tx.QueryRow(ctx, `select valor::text from indices where serie = 'usd_brl' and data <= $1
				and data > $1::date - 7 order by data desc limit 1`, base).Scan(&d.TaxaCambio)
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrSemCambio
			}
			if err != nil {
				return "", err
			}
		}
		c, err := core.ParseDec8(strings.ReplaceAll(strings.TrimSpace(d.TaxaCambio), ",", "."))
		if err != nil || c <= 0 {
			return "", ErrCampo{"câmbio inválido"}
		}
		d.Valor = int64(core.Valor(core.Dec8(d.ValorMoeda)*core.Um/100, c))
		vm, t := d.ValorMoeda, c.String()
		valorMoeda, taxa = &vm, &t
	}
	if d.Valor <= 0 {
		return "", ErrCampo{"informe o valor da receita"}
	}
	var clienteID *string
	if nome := strings.TrimSpace(d.Cliente); nome != "" {
		var id string
		err := tx.QueryRow(ctx, `insert into clientes (entidade_id, nome, pais, moeda, documento_cifrado) values ($1, $2, $3, $4, $5)
			on conflict (entidade_id, nome) do update set documento_cifrado = coalesce(clientes.documento_cifrado, excluded.documento_cifrado)
			returning id`, d.EntidadeID, nome, d.Pais, d.Moeda, d.DocumentoCliente).Scan(&id)
		if err != nil {
			return "", err
		}
		clienteID = &id
	}
	origem := d.Origem
	if origem == "" {
		origem = "manual"
	}
	var id string
	err = tx.QueryRow(ctx, `insert into notas_fiscais (entidade_id, cliente_id, numero, chave_acesso, data_emissao,
			data_recebimento, descricao, atividade, valor_centavos, moeda, valor_moeda_centavos, taxa_cambio, exportacao, origem)
		values ($1, $2, nullif($3, ''), nullif($4, ''), $5, $6, nullif($7, ''), $8, $9, $10, $11, $12::numeric, $13, $14)
		on conflict (entidade_id, chave_acesso) where chave_acesso is not null do nothing
		returning id`, d.EntidadeID, clienteID, strings.TrimSpace(d.Numero), d.ChaveAcesso, emissao, recebimento,
		strings.TrimSpace(d.Descricao), d.Atividade, d.Valor, d.Moeda, valorMoeda, taxa, d.Exportacao, origem).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// CancelarNota (ou reativa): cancelada não conta para teto, DAS nem lucro.
func CancelarNota(ctx context.Context, tx pgx.Tx, id string, cancelada bool) error {
	var ent string
	if err := tx.QueryRow(ctx, "select entidade_id from notas_fiscais where id = $1", id).Scan(&ent); err != nil {
		return err
	}
	if err := PodeEditarEntidade(ctx, tx, ent); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "update notas_fiscais set cancelada = $2 where id = $1", id, cancelada)
	return err
}

// ApagarNota da entidade.
func ApagarNota(ctx context.Context, tx pgx.Tx, id string) error {
	var ent string
	if err := tx.QueryRow(ctx, "select entidade_id from notas_fiscais where id = $1", id).Scan(&ent); err != nil {
		return err
	}
	if err := PodeEditarEntidade(ctx, tx, ent); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "delete from notas_fiscais where id = $1", id)
	return err
}

// MarcarDAS pago (ou desmarcar) numa competência AAAA-MM.
func MarcarDAS(ctx context.Context, tx pgx.Tx, entidadeID, competencia string, valor core.Centavos, pagoEm *string, hoje time.Time) error {
	e, err := lerEntidadePJ(ctx, tx, entidadeID)
	if err != nil {
		return err
	}
	if !e.PodeEditar {
		return ErrSemPermissao
	}
	mes, err := time.Parse("2006-01", competencia)
	if err != nil {
		return ErrCampo{"competência: AAAA-MM"}
	}
	var pago *time.Time
	if pagoEm != nil && *pagoEm != "" {
		p, err := time.Parse(formatoData, *pagoEm)
		if err != nil {
			return ErrCampo{"data de pagamento inválida"}
		}
		pago = &p
	}
	if valor < 0 {
		return ErrCampo{"valor inválido"}
	}
	anexo := "MEI"
	if e.Regime != "MEI" {
		anexo = core.AnexoV
		var rbt12 core.Centavos
		if p, err := CalcularPainelCNPJ(ctx, tx, entidadeID, mes.AddDate(0, 1, -1)); err == nil && p.Simples != nil {
			anexo, rbt12 = p.Simples.Anexo, p.Simples.RBT12
			if valor == 0 {
				valor = p.Simples.DAS.DAS
			}
		}
		_, err = tx.Exec(ctx, `insert into apuracoes_simples (entidade_id, competencia, rbt12_centavos, anexo, das_centavos, pago_em)
			values ($1, $2, $3, $4, $5, $6)
			on conflict (entidade_id, competencia) do update set das_centavos = excluded.das_centavos, pago_em = excluded.pago_em,
				rbt12_centavos = excluded.rbt12_centavos, anexo = excluded.anexo`, entidadeID, mes, rbt12, anexo, valor, pago)
		return err
	}
	if valor == 0 {
		rf, err := carregarRegrasFiscais(ctx, tx)
		if err != nil {
			return err
		}
		var r core.RegrasDASMEI
		if err := rf.ler("das_mei", mes, &r); err != nil {
			return err
		}
		valor = core.DASMEI(r, e.Atividade)
	}
	_, err = tx.Exec(ctx, `insert into apuracoes_simples (entidade_id, competencia, anexo, das_centavos, pago_em)
		values ($1, $2, 'MEI', $3, $4)
		on conflict (entidade_id, competencia) do update set das_centavos = excluded.das_centavos, pago_em = excluded.pago_em`,
		entidadeID, mes, valor, pago)
	return err
}

// Folha de uma competência.
type Folha struct {
	Competencia string        `json:"competencia"` // AAAA-MM
	Prolabore   core.Centavos `json:"prolabore_centavos"`
	Salarios    core.Centavos `json:"salarios_centavos"`
	INSS        core.Centavos `json:"inss_centavos"`
	IRRF        core.Centavos `json:"irrf_centavos"`
}

// ListarFolha dos últimos 12 meses.
func ListarFolha(ctx context.Context, tx pgx.Tx, entidadeID string, hoje time.Time) ([]Folha, error) {
	linhas, err := tx.Query(ctx, `select to_char(competencia, 'YYYY-MM'), prolabore_centavos, salarios_centavos, inss_centavos, irrf_centavos
		from folha where entidade_id = $1 and competencia >= $2 order by competencia desc`, entidadeID, inicioMes(hoje).AddDate(-1, 0, 0))
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	out := []Folha{}
	for linhas.Next() {
		var f Folha
		if err := linhas.Scan(&f.Competencia, &f.Prolabore, &f.Salarios, &f.INSS, &f.IRRF); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, linhas.Err()
}

// SalvarFolha da competência; INSS e IRRF do pró-labore são calculados pelas regras
// vigentes quando não vêm.
func SalvarFolha(ctx context.Context, tx pgx.Tx, entidadeID string, f Folha) (Folha, error) {
	if err := PodeEditarEntidade(ctx, tx, entidadeID); err != nil {
		return f, err
	}
	mes, err := time.Parse("2006-01", f.Competencia)
	if err != nil {
		return f, ErrCampo{"competência: AAAA-MM"}
	}
	if f.Prolabore < 0 || f.Salarios < 0 || f.INSS < 0 || f.IRRF < 0 {
		return f, ErrCampo{"valores não podem ser negativos"}
	}
	if f.Prolabore > 0 && f.INSS == 0 && f.IRRF == 0 {
		rf, err := carregarRegrasFiscais(ctx, tx)
		if err != nil {
			return f, err
		}
		var inss core.RegrasINSS
		var irrf core.TabelaIRRF
		if err := rf.ler("inss_prolabore", mes, &inss); err == nil {
			f.INSS = core.INSSProlabore(f.Prolabore, inss)
		}
		if err := rf.ler("tabela_irrf", mes, &irrf); err == nil {
			f.IRRF = core.IRRF(f.Prolabore, f.INSS, 0, irrf)
		}
	}
	_, err = tx.Exec(ctx, `insert into folha (entidade_id, competencia, prolabore_centavos, salarios_centavos, inss_centavos, irrf_centavos)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (entidade_id, competencia) do update set prolabore_centavos = excluded.prolabore_centavos,
			salarios_centavos = excluded.salarios_centavos, inss_centavos = excluded.inss_centavos, irrf_centavos = excluded.irrf_centavos`,
		entidadeID, mes, f.Prolabore, f.Salarios, f.INSS, f.IRRF)
	return f, err
}

// Distribuicao de lucro para uma pessoa.
type Distribuicao struct {
	ID    string        `json:"id"`
	Data  string        `json:"data"`
	Valor core.Centavos `json:"valor_centavos"`
	IRRF  core.Centavos `json:"irrf_centavos"`
	Desc  *string       `json:"descricao"`
}

// AvisoDistribuicao antes de um Pix de lucro.
type AvisoDistribuicao struct {
	JaNoMes core.Centavos `json:"ja_no_mes_centavos"`
	Folga   core.Centavos `json:"folga_centavos"`
	IRRF    core.Centavos `json:"irrf_centavos"`
	Limite  core.Centavos `json:"limite_centavos"`
}

// SimularDistribuicao: quanto ainda cabe no mês sem IRRF (Lei 15.270/2025) e o IRRF se
// passar. Considera o que a entidade já distribuiu para a mesma pessoa no mês.
func SimularDistribuicao(ctx context.Context, tx pgx.Tx, entidadeID, beneficiario string, data time.Time, valor core.Centavos) (AvisoDistribuicao, error) {
	var a AvisoDistribuicao
	rf, err := carregarRegrasFiscais(ctx, tx)
	if err != nil {
		return a, err
	}
	var l core.LimiteDividendos
	if err := rf.ler("limite_dividendos_mes", data, &l); err != nil {
		return a, err
	}
	var ja int64
	if err := tx.QueryRow(ctx, `select coalesce(sum(valor_centavos), 0) from distribuicoes_lucro
		where entidade_id = $1 and beneficiario_id = $2 and date_trunc('month', data) = date_trunc('month', $3::date)`,
		entidadeID, beneficiario, data).Scan(&ja); err != nil {
		return a, err
	}
	a.JaNoMes, a.Limite = core.Centavos(ja), l.Centavos
	a.IRRF, a.Folga = core.IRRFDividendos(a.JaNoMes, valor, l)
	return a, nil
}

// RegistrarDistribuicao do usuário da sessão; o IRRF do mês é recalculado e o que
// faltava reter entra nesta.
func RegistrarDistribuicao(ctx context.Context, tx pgx.Tx, entidadeID, beneficiario string, data time.Time, valor core.Centavos, descricao string) (AvisoDistribuicao, error) {
	if err := PodeEditarEntidade(ctx, tx, entidadeID); err != nil {
		return AvisoDistribuicao{}, err
	}
	if valor <= 0 {
		return AvisoDistribuicao{}, ErrCampo{"informe o valor"}
	}
	a, err := SimularDistribuicao(ctx, tx, entidadeID, beneficiario, data, valor)
	if err != nil {
		return a, err
	}
	var retido int64
	if err := tx.QueryRow(ctx, `select coalesce(sum(irrf_centavos), 0) from distribuicoes_lucro
		where entidade_id = $1 and beneficiario_id = $2 and date_trunc('month', data) = date_trunc('month', $3::date)`,
		entidadeID, beneficiario, data).Scan(&retido); err != nil {
		return a, err
	}
	irrf := max(a.IRRF-core.Centavos(retido), 0)
	_, err = tx.Exec(ctx, `insert into distribuicoes_lucro (entidade_id, beneficiario_id, data, valor_centavos, irrf_centavos, descricao)
		values ($1, $2, $3, $4, $5, nullif($6, ''))`, entidadeID, beneficiario, data, valor, irrf, strings.TrimSpace(descricao))
	return a, err
}

// ListarDistribuicoes do ano.
func ListarDistribuicoes(ctx context.Context, tx pgx.Tx, entidadeID string, ano int) ([]Distribuicao, error) {
	linhas, err := tx.Query(ctx, `select id, to_char(data, 'YYYY-MM-DD'), valor_centavos, irrf_centavos, descricao
		from distribuicoes_lucro where entidade_id = $1 and extract(year from data) = $2 order by data desc`, entidadeID, ano)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	out := []Distribuicao{}
	for linhas.Next() {
		var d Distribuicao
		if err := linhas.Scan(&d.ID, &d.Data, &d.Valor, &d.IRRF, &d.Desc); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, linhas.Err()
}

// ---------------------------------------------------------------------------
// Calendário fiscal na agenda
// ---------------------------------------------------------------------------

// itensFiscais dos CNPJs: DAS do mês (dia 20), DASN-SIMEI (31/05, MEI) e DEFIS (31/03,
// Simples), entre hoje e `ate`.
func itensFiscais(ctx context.Context, tx pgx.Tx, ents []string, hoje, ate time.Time) ([]ItemAgenda, error) {
	linhas, err := tx.Query(ctx, `select id from entidades where id = any($1) and tipo = 'PJ'`, ents)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	var out []ItemAgenda
	for _, id := range ids {
		p, err := CalcularPainelCNPJ(ctx, tx, id, hoje)
		if err != nil {
			if errors.Is(err, core.ErrSemRegraVigente) {
				continue
			}
			return nil, err
		}
		nome := p.Entidade.Nome
		das := p.Simples
		var meses []DASMes
		var valorFuturo core.Centavos
		rotulo := "DAS"
		if p.MEI != nil {
			meses, valorFuturo, rotulo = p.MEI.DASMeses, p.MEI.DAS, "DAS-MEI"
		} else if das != nil {
			meses, valorFuturo = das.DASMeses, das.DAS.DAS
		}
		vistos := map[string]bool{}
		for _, d := range meses {
			vistos[d.Competencia] = true
			if d.PagoEm != nil || d.Valor == 0 && p.MEI != nil {
				continue
			}
			venc, _ := time.Parse(formatoData, d.Vencimento)
			if venc.After(ate) {
				continue
			}
			it := ItemAgenda{Data: d.Vencimento, Descricao: fmt.Sprintf("%s %s (%s)", rotulo, d.Competencia, nome),
				Valor: -int64(d.Valor), Origem: "fiscal", ID: id + ":das:" + d.Competencia, Atrasado: d.Atrasado}
			if d.Valor == 0 {
				it.Valor = -int64(valorFuturo)
			}
			if it.Atrasado {
				it.Data = hoje.Format(formatoData)
			}
			out = append(out, it)
		}
		// competências futuras dentro da janela
		for m := inicioMes(hoje).AddDate(0, 1, 0); !time.Date(m.Year(), m.Month()+1, 20, 0, 0, 0, 0, time.UTC).After(ate); m = m.AddDate(0, 1, 0) {
			k := m.Format("2006-01")
			if vistos[k] {
				continue
			}
			venc := time.Date(m.Year(), m.Month()+1, 20, 0, 0, 0, 0, time.UTC)
			out = append(out, ItemAgenda{Data: venc.Format(formatoData), Descricao: fmt.Sprintf("%s %s (%s)", rotulo, k, nome),
				Valor: -int64(valorFuturo), Origem: "fiscal", ID: id + ":das:" + k})
		}
		for ano := hoje.Year(); ano <= ate.Year(); ano++ {
			d, desc := time.Date(ano, 5, 31, 0, 0, 0, 0, time.UTC), "DASN-SIMEI "+fmt.Sprint(ano-1)+" ("+nome+")"
			if p.MEI == nil {
				d, desc = time.Date(ano, 3, 31, 0, 0, 0, 0, time.UTC), "DEFIS "+fmt.Sprint(ano-1)+" ("+nome+")"
			}
			if !d.Before(hoje) && !d.After(ate) {
				out = append(out, ItemAgenda{Data: d.Format(formatoData), Descricao: desc, Origem: "fiscal", ID: id + ":declaracao:" + fmt.Sprint(ano)})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Data < out[j].Data })
	return out, nil
}
