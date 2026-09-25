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
  nome: string;
  tipo: string;
  moeda: string;
  visibilidade: "privada" | "saldo" | "compartilhada";
  casa_id: string | null;
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
