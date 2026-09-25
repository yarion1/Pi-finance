// Consultas compartilhadas entre telas (mesma chave = mesmo cache).
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { obter } from "./api";
import type { Casa, Categoria, Conta, Entidade, Instituicao, ItemOrcamento } from "./tipos";

export const useEntidades = () =>
  useQuery({ queryKey: ["entidades"], queryFn: () => obter<Entidade[]>("/api/entidades") });

export const useContas = () =>
  useQuery({ queryKey: ["contas"], queryFn: () => obter<Conta[]>("/api/contas") });

export const useCategorias = () =>
  useQuery({
    queryKey: ["categorias"],
    queryFn: () => obter<Categoria[]>("/api/categorias"),
    staleTime: 60_000,
  });

export const useInstituicoes = () =>
  useQuery({
    queryKey: ["instituicoes"],
    queryFn: () => obter<Instituicao[]>("/api/instituicoes"),
    staleTime: 600_000,
  });

export const useCasas = () => useQuery({ queryKey: ["casas"], queryFn: () => obter<Casa[]>("/api/casas") });

/** Categorias visíveis para uma entidade (padrão + dela), em árvore. */
export function arvoreCategorias(todas: Categoria[], entidadeId?: string) {
  const visiveis = todas.filter((c) => c.entidade_id === null || !entidadeId || c.entidade_id === entidadeId);
  const maes = visiveis.filter((c) => !c.pai_id);
  return maes.map((m) => ({ ...m, filhas: visiveis.filter((c) => c.pai_id === m.id) }));
}

export function rotuloCategoria(todas: Categoria[], id: string | null | undefined): string {
  if (!id) return "Sem categoria";
  const c = todas.find((x) => x.id === id);
  if (!c) return "—";
  const pai = c.pai_id ? todas.find((x) => x.id === c.pai_id) : undefined;
  return pai ? `${pai.nome} › ${c.nome}` : c.nome;
}

/** "2026-09" do mês atual em São Paulo. */
export function mesAtual(): string {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "America/Sao_Paulo",
    year: "numeric",
    month: "2-digit",
  }).format(new Date());
}

export function somarMes(mes: string, n: number): string {
  const [a, m] = mes.split("-").map(Number);
  const d = new Date(Date.UTC(a ?? 2000, (m ?? 1) - 1 + n, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

export function nomeMes(mes: string): string {
  const [a, m] = mes.split("-").map(Number);
  const s = new Intl.DateTimeFormat("pt-BR", { month: "long", year: "numeric", timeZone: "UTC" }).format(
    new Date(Date.UTC(a ?? 2000, (m ?? 1) - 1, 1)),
  );
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** Primeiro e último dia do mês ("2026-09" → ["2026-09-01", "2026-09-30"]). */
export function limitesMes(mes: string): [string, string] {
  const [a, m] = mes.split("-").map(Number);
  const ultimo = new Date(Date.UTC(a ?? 2000, m ?? 1, 0)).getUTCDate();
  return [`${mes}-01`, `${mes}-${String(ultimo).padStart(2, "0")}`];
}

/** Lê um arquivo como base64 (sem o prefixo data:). */
export function lerBase64(arquivo: File): Promise<string> {
  return new Promise((ok, erro) => {
    const leitor = new FileReader();
    leitor.onload = () => ok(String(leitor.result).replace(/^data:[^,]*,/, ""));
    leitor.onerror = () => erro(leitor.error);
    leitor.readAsDataURL(arquivo);
  });
}

/** Entidades em que o usuário pode lançar (dono ou membro); PF primeiro. */
export function useEditaveis() {
  const entidades = useEntidades();
  const lista = (entidades.data ?? [])
    .filter((e) => e.papel === "dono" || e.papel === "membro")
    .sort((a, b) => (a.tipo === b.tipo ? 0 : a.tipo === "PF" ? -1 : 1));
  return { ...entidades, lista };
}

/** Tudo que muda quando um planejamento muda (agenda, indicadores, patrimônio...). */
export const chavesPlanejamento = [
  "indicadores",
  "agenda",
  "recorrencias",
  "orcamento",
  "metas",
  "patrimonio",
  "compromissos",
  "faturas",
  "contas",
  "resumo",
];

export function useInvalidarPlanejamento() {
  const cliente = useQueryClient();
  return () => Promise.all(chavesPlanejamento.map((k) => cliente.invalidateQueries({ queryKey: [k] })));
}

/** Gasto que conta contra o orçado: só categorias com limite, sem somar mãe e filha duas vezes. */
export function gastoOrcado(itens: ItemOrcamento[]): number {
  const maes = new Set(itens.filter((i) => !i.pai).map((i) => i.nome));
  return itens.filter((i) => !i.pai || !maes.has(i.pai)).reduce((s, i) => s + i.gasto_centavos, 0);
}
