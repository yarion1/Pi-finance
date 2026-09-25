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
