// Formatos devolvidos pela API (internal/api).

export type Entidade = {
  id: string;
  tipo: "PF" | "PJ";
  nome: string;
  regime: "MEI" | "SIMPLES_ME" | "SIMPLES_EPP" | null;
  anexo: "III" | "V" | null;
  cnae: string | null;
  municipio: string | null;
  data_abertura: string | null;
  atividade: "servicos" | "comercio" | "ambos" | null;
  papel: "dono" | "membro" | "leitor" | "contador";
  documento: string | null;
};

export type Acesso = {
  usuario_id: string;
  nome: string;
  email: string;
  papel: "membro" | "leitor" | "contador";
  criado_em: string;
};

export type Casa = { id: string; nome: string; moeda_base: string; papel: "dono" | "membro" | "leitor" };

export type Membro = {
  usuario_id: string;
  nome: string;
  email: string;
  papel: Casa["papel"];
  entrou_em: string;
};

export type ConvitePendente = {
  id: string;
  papel: "membro" | "leitor";
  criado_em: string;
  expira_em: string;
};

export type Alerta = {
  id: string;
  tipo: string;
  dados: Record<string, unknown>;
  criado_em: string;
  lido_em: string | null;
};

export type Conta = {
  id: string;
  entidade_id: string;
  entidade_nome: string | null;
  dono: string | null;
  instituicao_id: string | null;
  instituicao: string | null;
  nome: string;
  tipo: "corrente" | "poupanca" | "carteira" | "dinheiro" | "beneficio" | "investimento" | "cartao";
  moeda: string;
  visibilidade: "privada" | "saldo" | "compartilhada";
  casa_id: string | null;
  saldo_inicial_centavos: number;
  saldo_centavos: number | null;
  fechamento: number | null;
  vencimento: number | null;
  limite_centavos: number | null;
  arquivada: boolean;
  pode_editar: boolean;
  saldo_banco_centavos: number | null;
  saldo_banco_em: string | null;
  open_finance: boolean;
  limite_usado_banco_centavos: number | null;
  numero_final: string | null;
};

export type Instituicao = { id: string; nome: string; codigo_compe: string | null };

export type Transacao = {
  id: string;
  entidade_id: string;
  conta_id: string;
  conta: string;
  data: string;
  descricao: string;
  descricao_original: string;
  valor_centavos: number;
  moeda: string;
  tipo: "gasto" | "receita" | "transferencia" | "estorno";
  categoria_id: string | null;
  categoria: string | null;
  categoria_pai: string | null;
  cor: string | null;
  transferencia_par_id: string | null;
  origem: string;
  notas: string | null;
  importacao_id: string | null;
};

export type Categoria = {
  id: string;
  entidade_id: string | null;
  pai_id: string | null;
  nome: string;
  icone: string | null;
  cor: string | null;
  tipo: "gasto" | "receita" | "transferencia";
  ordem: number;
};

export type Regra = {
  id: string;
  entidade_id: string;
  texto: string;
  conta_id: string | null;
  categoria_id: string;
  categoria: string;
  prioridade: number;
};

export type Importacao = {
  id: string;
  entidade_id: string;
  conta_id: string;
  conta: string;
  fonte: string;
  arquivo: string;
  linhas: number;
  novas: number;
  duplicadas: number;
  transferencias: number;
  criada_em: string;
  desfeita_em: string | null;
};

export type ResultadoImportacao = {
  importacao_id?: string;
  formato: string;
  linhas: number;
  novas: number;
  duplicadas: number;
  transferencias: number;
  categorizadas: number;
  avisos?: string[];
  previa?: {
    data: string;
    descricao: string;
    valor_centavos: number;
    duplicada: boolean;
    categoria_id: string | null;
    origem_categoria?: string;
  }[];
  saldo_arquivo_centavos?: number;
  saldo_sistema_centavos?: number;
};

export type Mapeamento = {
  separador?: string;
  coluna_data: string;
  colunas_descricao: string[];
  coluna_valor: string;
  coluna_id?: string;
  formato_data: string;
  decimal: "," | ".";
  inverter_sinal: boolean;
};

export type Resumo = {
  mes: string;
  ate: string;
  saldo_total_centavos: number;
  contas: Conta[];
  receitas_centavos: number;
  gastos_centavos: number;
  gastos_mes_anterior_mesmo_periodo_centavos: number;
  gastos_por_categoria: {
    categoria_id: string | null;
    nome: string;
    cor: string | null;
    total_centavos: number;
  }[];
  sem_categoria: number;
  ultimas: Transacao[];
};

export const nomesTipoConta: Record<Conta["tipo"], string> = {
  corrente: "Conta corrente",
  poupanca: "Poupança",
  carteira: "Carteira digital",
  dinheiro: "Dinheiro",
  beneficio: "Vale-refeição/alimentação",
  investimento: "Investimentos",
  cartao: "Cartão de crédito",
};

export const nomesVisibilidade: Record<Conta["visibilidade"], string> = {
  privada: "Privada",
  saldo: "Só saldo para a casa",
  compartilhada: "Compartilhada com a casa",
};

export const nomesRegime: Record<string, string> = {
  MEI: "MEI",
  SIMPLES_ME: "Simples — ME",
  SIMPLES_EPP: "Simples — EPP",
};

export const nomesPapel: Record<string, string> = {
  dono: "Dono",
  membro: "Membro",
  leitor: "Leitor",
  contador: "Contador",
};

// ---- Fase 2: planejamento ----

export type Fatura = {
  vencimento: string;
  fechamento: string;
  status: "aberta" | "futura" | "fechada" | "anterior";
  compras_centavos: number;
  projetado_centavos: number;
  pagamentos_centavos: number;
  transacoes: number;
  paga: boolean;
};

export type ParcelaFutura = {
  conta_id: string;
  conta: string;
  descricao: string;
  parcela: number;
  total: number;
  valor_centavos: number;
  mes: string;
  data: string;
  fatura_em?: string;
  projetada: boolean;
};

export type RespostaFaturas = { faturas: Fatura[]; parcelas_futuras: ParcelaFutura[] };

export type PontoPatrimonio = {
  data: string;
  ativos_centavos: number;
  passivos_centavos: number;
  liquido_centavos: number;
};

export type Patrimonio = {
  hoje: PontoPatrimonio;
  variacao_mes_centavos: number;
  variacao_ano_centavos: number;
  serie: PontoPatrimonio[] | null;
  contas_centavos: number;
  investimentos_centavos: number;
  bens_centavos: number;
  cartoes_centavos: number;
  contas_negativas_centavos: number;
  dividas_centavos: number;
};

export type CustoDeVida = {
  medio_3_meses_centavos: number;
  medio_6_meses_centavos: number;
  medio_12_meses_centavos: number;
};

export type MesComprometido = {
  mes: string;
  parcelas_centavos: number;
  dividas_centavos: number;
  recorrentes_centavos: number;
  compromissos_centavos: number;
  total_centavos: number;
};

export type SaldoDia = { data: string; saldo_centavos: number };

export type Indicadores = {
  patrimonio: Patrimonio;
  custo_de_vida: CustoDeVida;
  comprometido: MesComprometido[];
  saldo_projetado_30_dias_centavos: number;
  menor_saldo_30_dias: SaldoDia;
  reserva_centavos: number;
  meses_de_reserva: number | null;
};

export type ItemAgenda = {
  data: string;
  descricao: string;
  valor_centavos: number;
  origem: "recorrencia" | "fatura" | "divida" | "compromisso" | "lancamento";
  id: string;
  conta_id: string | null;
  conta: string | null;
  pago: boolean;
  atrasado: boolean;
  no_cartao: boolean;
};

export type Agenda = {
  itens: ItemAgenda[] | null;
  saldo_inicial_centavos: number;
  saldos: SaldoDia[] | null;
  saldo_final_centavos: number;
  menor_saldo: SaldoDia;
};

export type Recorrencia = {
  id: string;
  entidade_id: string;
  conta_id: string | null;
  conta: string | null;
  conta_cartao: boolean;
  descricao: string;
  categoria_id: string | null;
  categoria: string | null;
  valor_centavos: number;
  valor_anterior_centavos: number | null;
  frequencia: "mensal" | "anual" | "semanal";
  dia: number;
  ultima_data: string | null;
  proxima: string;
  atrasada: boolean;
  tipo: "assinatura" | "conta_fixa" | "receita";
  ativa: boolean;
  detectada: boolean;
  subiu: boolean;
};

export type ItemOrcamento = {
  categoria_id: string;
  nome: string;
  pai: string | null;
  cor: string | null;
  limite_centavos: number;
  sobra_anterior_centavos: number;
  disponivel_centavos: number;
  gasto_centavos: number;
  ritmo_ideal_centavos: number;
  situacao: "ok" | "acima_do_ritmo" | "estourado" | "";
  media_3_meses_centavos: number;
};

export type Grupo503020 = { alvo_centavos: number; valor_centavos: number };

export type Orcamento = {
  entidade_id: string;
  mes: string;
  modo: "categoria" | "envelopes" | "50_30_20";
  grupos: Record<string, "necessidades" | "desejos"> | null;
  dias_no_mes: number;
  dia: number;
  itens: ItemOrcamento[] | null;
  sem_orcamento: ItemOrcamento[] | null;
  totais: {
    limite_centavos: number;
    disponivel_centavos: number;
    gasto_centavos: number;
    ritmo_ideal_centavos: number;
  };
  regra_50_30_20: {
    renda_centavos: number;
    necessidades: Grupo503020;
    desejos: Grupo503020;
    poupanca: Grupo503020;
  } | null;
};

export type Meta = {
  id: string;
  entidade_id: string;
  nome: string;
  tipo: "reserva" | "viagem" | "compra" | "aposentadoria" | "outra";
  alvo_centavos: number;
  data_alvo: string | null;
  contas_vinculadas: string[];
  valor_manual_centavos: number;
  aporte_planejado_centavos: number | null;
  atual_centavos: number;
  percentual: number;
  aporte_necessario_centavos: number;
  meses_restantes: number;
  data_prevista: string | null;
  meses_de_custo_de_vida: number | null;
  concluida: boolean;
};

export type Bem = {
  id: string;
  entidade_id: string;
  nome: string;
  tipo: "imovel" | "veiculo" | "outro";
  valor_centavos: number;
  atualizado_em: string;
  fipe_codigo: string | null;
  notas: string | null;
};

export type ParcelaDivida = {
  numero: number;
  vencimento: string;
  prestacao_centavos: number;
  juros_centavos: number;
  amortizacao_centavos: number;
  saldo_devedor_centavos: number;
};

export type Divida = {
  id: string;
  entidade_id: string;
  nome: string;
  tipo: "financiamento" | "emprestimo" | "outro";
  sistema: "price" | "sac";
  principal_centavos: number;
  taxa_mensal: string;
  prazo_meses: number;
  primeiro_vencimento: string;
  conta_id: string | null;
  bem_id: string | null;
  saldo_devedor_centavos: number;
  proxima_parcela: ParcelaDivida | null;
  parcelas_restantes: number;
};

export type RespostaPatrimonio = Patrimonio & { bens: Bem[]; dividas: Divida[] };

export type Compromisso = {
  id: string;
  entidade_id: string;
  conta_id: string | null;
  descricao: string;
  valor_centavos: number;
  vencimento: string;
  categoria_id: string | null;
  pago_em: string | null;
};

export const nomesTipoMeta: Record<Meta["tipo"], string> = {
  reserva: "Reserva de emergência",
  viagem: "Viagem",
  compra: "Compra",
  aposentadoria: "Aposentadoria",
  outra: "Outra",
};

export const nomesTipoRecorrencia: Record<Recorrencia["tipo"], string> = {
  assinatura: "Assinatura",
  conta_fixa: "Conta fixa",
  receita: "Receita",
};

export const nomesFrequencia: Record<Recorrencia["frequencia"], string> = {
  mensal: "Mensal",
  anual: "Anual",
  semanal: "Semanal",
};

// ---- Fase 3: Open Finance (Meu Pluggy) ----

export type ContaPluggy = {
  id: string;
  nome: string;
  tipo: "BANK" | "CREDIT";
  numero: string | null;
  moeda: string;
  saldo_centavos: number | null;
  limite_centavos: number | null;
  disponivel_centavos: number | null;
  saldo_em: string | null;
  conta_id: string | null;
  conta: string | null;
  ignorada: boolean;
  ultima_sync: string | null;
};

export type ItemPluggy = {
  id: string;
  item_id: string;
  entidade_id: string;
  entidade: string | null;
  instituicao: string | null;
  status: string | null;
  atualizado_em: string | null;
  ultima_sync: string | null;
  ultimo_erro: string | null;
  contas: ContaPluggy[];
};

export type OpenFinance = {
  conexao: {
    client_id_fim: string;
    criada_em: string;
    ultima_sync: string | null;
    ultimo_erro: string | null;
  } | null;
  itens: ItemPluggy[];
};

export type RelatorioSincronizacao = {
  itens: number;
  contas: number;
  contas_sem_vinculo: number;
  novas: number;
  casadas: number;
  divergencias: number;
  transferencias: number;
  erros?: string[];
};

// ---- Fase 4: investimentos ----

export type AtivoCarteira = {
  id: string;
  entidade_id: string;
  codigo: string;
  nome: string;
  classe: string;
  subtipo: string | null;
  instituicao: string | null;
  emissor: string | null;
  vencimento: string | null;
  origem: "manual" | "b3" | "pluggy";
  quantidade: string;
  preco_medio: string;
  cotacao: string | null;
  cotacao_em: string | null;
  custo_centavos: number;
  valor_centavos: number;
  rendimento_centavos: number;
  proventos_centavos: number;
  percentual: number;
  encerrado: boolean;
  isento_ir: boolean;
  rentabilidade_aa: number | null;
  na_curva: boolean;
};

export type Carteira = {
  total_centavos: number;
  custo_centavos: number;
  classes: { classe: string; valor_centavos: number; ativos: number; percentual: number }[];
  ativos: AtivoCarteira[];
  encerrados: number;
  valor_em: string;
  rentabilidade_aa: number | null;
  rentabilidade_desde: string | null;
  cdi_aa: number | null;
  ipca_aa: number | null;
};

export const nomesClasse: Record<string, string> = {
  acao: "Ações",
  fii: "Fundos imobiliários",
  etf: "ETFs",
  bdr: "BDRs",
  renda_fixa: "Renda fixa",
  tesouro: "Tesouro Direto",
  fundo: "Fundos",
  cripto: "Cripto",
  exterior: "Exterior",
  previdencia: "Previdência",
  outro: "Outros",
};

export type ResultadoB3 = {
  formato: "b3_negociacao" | "b3_movimentacao";
  novas: number;
  duplicadas: number;
  ativos_novos: string[];
  avisos: string[];
};

export type OperacaoAtivo = {
  id: string;
  data: string;
  tipo: string;
  quantidade: string;
  preco: string;
  valor_centavos: number;
  taxas_centavos: number;
  ir_retido_centavos: number;
  descricao: string | null;
  origem: string;
  evento: boolean;
};

export type GrupoIR = {
  grupo: "comum" | "day_trade" | "fii";
  vendas_centavos: number;
  ganho_centavos: number;
  isento_centavos: number;
  prejuizo_anterior_centavos: number;
  base_centavos: number;
  imposto_centavos: number;
  prejuizo_acumulado_centavos: number;
  aliquota: string;
};

export type MesIR = {
  mes: string;
  grupos: GrupoIR[];
  vendas_acoes_centavos: number;
  isento_acoes: boolean;
  imposto_centavos: number;
  ir_retido_centavos: number;
  darf_centavos: number;
  darf_acumulado_centavos: number;
  vencimento: string;
  codigo_darf: string;
};

export type ApuracaoIR = {
  meses: MesIR[];
  vendas: {
    mes: string;
    data: string;
    ativo: string;
    classe: string;
    valor_centavos: number;
    custo_centavos: number;
    ganho_centavos: number;
    day_trade: boolean;
  }[];
  fonte: string;
};

export const nomesOperacao: Record<string, string> = {
  compra: "Compra",
  venda: "Venda",
  provento: "Provento",
  juros: "Juros",
  amortizacao: "Amortização",
  ajuste: "Ajuste de quantidade",
  desdobramento: "Desdobramento",
  grupamento: "Grupamento",
  bonificacao: "Bonificação",
};

export const nomesGrupoIR: Record<GrupoIR["grupo"], string> = {
  comum: "Ações, ETFs e BDRs",
  day_trade: "Day trade",
  fii: "Fundos imobiliários",
};

export type ReceitaMes = {
  mes: string;
  receita_centavos: number;
  exportacao_centavos: number;
  servicos_centavos: number;
  comercio_centavos: number;
};

export type DASMes = {
  competencia: string;
  vencimento: string;
  valor_centavos: number;
  pago_em: string | null;
  atrasado: boolean;
};

export type PainelCNPJ = {
  entidade: {
    id: string;
    nome: string;
    regime: "MEI" | "SIMPLES_ME" | "SIMPLES_EPP";
    atividade: string;
    data_abertura: string | null;
    pode_editar: boolean;
  };
  hoje: string;
  meses: ReceitaMes[];
  receita_ano_centavos: number;
  receita_mes_centavos: number;
  despesas_ano_centavos: number;
  clientes: { id: string | null; nome: string; pais: string; receita_centavos: number; percentual: number }[];
  alertas: string[];
  lucro_distribuivel_mes_centavos: number;
  mei?: {
    projecao: {
      faturado_centavos: number;
      teto_centavos: number;
      falta_centavos: number;
      media_maxima_centavos: number;
      projecao_ano_centavos: number;
      data_teto: string | null;
      percentual: number;
      meses_restantes: number;
    };
    situacao: "normal" | "atencao" | "planejar" | "vira_me" | "desenquadramento";
    receita_12m_centavos: number;
    teto_com_tolerancia_centavos: number;
    das_centavos: number;
    das_meses: DASMes[];
    lucro: {
      receita_centavos: number;
      despesas_centavos: number;
      lucro_centavos: number;
      presumido_centavos: number;
      isento_centavos: number;
      tributavel_centavos: number;
    };
    dasn_ano: number;
    dasn_receita_centavos: number;
    dasn_prazo: string;
    exportacao_ano_centavos: number;
  };
  simples?: {
    rbt12_centavos: number;
    folha_12m_centavos: number;
    fator_r: string;
    fator_r_minimo: string;
    anexo: "III" | "V";
    das: {
      faixa: number;
      aliquota_nominal: string;
      aliquota_efetiva: string;
      das_centavos: number;
      das_sem_exportacao_centavos: number;
      exportacao_centavos: number;
      tributos: { tributo: string; aliquota: string; valor_centavos: number }[];
    };
    prolabore_para_fator_r_centavos: number;
    acima_sublimite: boolean;
    das_meses: DASMes[];
  };
};

export type NotaCNPJ = {
  id: string;
  cliente: string | null;
  pais: string | null;
  numero: string | null;
  data_emissao: string;
  descricao: string | null;
  atividade: string;
  valor_centavos: number;
  moeda: string;
  valor_moeda_centavos: number | null;
  taxa_cambio: string | null;
  exportacao: boolean;
  cancelada: boolean;
  origem: string;
};

export type FolhaMes = {
  competencia: string;
  prolabore_centavos: number;
  salarios_centavos: number;
  inss_centavos: number;
  irrf_centavos: number;
};

export type CenarioCNPJ = {
  nome: string;
  possivel: boolean;
  anexo?: string;
  aliquota_efetiva?: string;
  impostos_centavos: number;
  prolabore_mensal_centavos: number;
  inss_anual_centavos: number;
  irrf_anual_centavos: number;
  contador_anual_centavos: number;
  total_anual_centavos: number;
  total_mensal_centavos: number;
  observacao: string;
};

export type SimulacaoCNPJ = {
  receita_mensal_centavos: number;
  receita_anual_centavos: number;
  teto_mei_centavos: number;
  receita_mensal_maxima_mei_centavos: number;
  cenarios: CenarioCNPJ[];
  melhor: string;
};

export type AvisoDistribuicao = {
  ja_no_mes_centavos: number;
  folga_centavos: number;
  irrf_centavos: number;
  limite_centavos: number;
};

export const nomesSituacaoMEI: Record<string, string> = {
  normal: "Dentro do teto",
  atencao: "Atenção: passou de 70 %",
  planejar: "Planeje a migração (90 %)",
  vira_me: "Passou do teto: vira ME em janeiro",
  desenquadramento: "Mais de 20 % acima: desenquadramento",
};

export type UsoIA = {
  chamadas: number;
  tokens_entrada: number;
  tokens_saida: number;
  custo_microdolares: number;
};

export type ConfigIA = {
  ativa: boolean;
  teto_mensal_microdolares: number;
  uso_mes: UsoIA;
  servidor: boolean;
};

export type RespostaChat = {
  texto: string;
  ferramentas: string[];
  uso: UsoIA;
  interrompida: boolean;
};

export const nomesFerramentaIA: Record<string, string> = {
  gastos_por_categoria: "gastos por categoria",
  resumo_periodo: "receitas e gastos",
  buscar_transacoes: "busca de transações",
  saldos: "saldos",
  proximas_contas: "próximas contas",
  carteira: "carteira",
  patrimonio: "patrimônio",
  cnpj_resumo: "CNPJ",
};
