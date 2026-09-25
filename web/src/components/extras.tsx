import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { type ReactNode, useEffect, useRef } from "react";
import { arvoreCategorias, nomeMes, somarMes } from "../lib/dados";
import { formatarMoeda } from "../lib/formato";
import type { Categoria } from "../lib/tipos";

/** Seletor de mês: ‹ Setembro de 2026 › */
export function SeletorMes({ mes, onChange }: { mes: string; onChange: (m: string) => void }) {
  return (
    <div className="flex items-center gap-1 rounded-xl border border-borda bg-superficie">
      <button
        type="button"
        aria-label="Mês anterior"
        onClick={() => onChange(somarMes(mes, -1))}
        className="inline-flex size-11 items-center justify-center text-texto-2 hover:text-texto"
      >
        <ChevronLeft className="size-5" />
      </button>
      <span className="min-w-36 text-center text-sm font-semibold" aria-live="polite">
        {nomeMes(mes)}
      </span>
      <button
        type="button"
        aria-label="Próximo mês"
        onClick={() => onChange(somarMes(mes, 1))}
        className="inline-flex size-11 items-center justify-center text-texto-2 hover:text-texto"
      >
        <ChevronRight className="size-5" />
      </button>
    </div>
  );
}

/** <select> de categorias agrupado por categoria mãe. */
export function SeletorCategoria({
  categorias,
  entidadeId,
  valor,
  onChange,
  rotulo = "Categoria",
  vazio = "Sem categoria",
  ocultarRotulo,
}: {
  categorias: Categoria[];
  entidadeId?: string;
  valor: string;
  onChange: (id: string) => void;
  rotulo?: string;
  vazio?: string;
  ocultarRotulo?: boolean;
}) {
  const arvore = arvoreCategorias(categorias, entidadeId);
  const grupos: [string, typeof arvore][] = [
    ["Gastos", arvore.filter((c) => c.tipo === "gasto")],
    ["Receitas", arvore.filter((c) => c.tipo === "receita")],
    ["Transferências", arvore.filter((c) => c.tipo === "transferencia")],
  ];
  return (
    <label className="flex flex-col gap-1.5">
      <span className={ocultarRotulo ? "sr-only" : "text-sm font-medium"}>{rotulo}</span>
      <select
        value={valor}
        onChange={(e) => onChange(e.target.value)}
        className="min-h-11 rounded-xl border border-borda bg-superficie-2 px-3 text-base text-texto focus:border-primaria focus:outline-none"
      >
        <option value="">{vazio}</option>
        {grupos.map(([nome, maes]) => (
          <optgroup key={nome} label={nome}>
            {maes.flatMap((m) => [
              <option key={m.id} value={m.id}>
                {m.nome}
              </option>,
              ...m.filhas.map((f) => (
                <option key={f.id} value={f.id}>
                  {"   "}
                  {m.nome} › {f.nome}
                </option>
              )),
            ])}
          </optgroup>
        ))}
      </select>
    </label>
  );
}

/** Valor com a cor do significado: entrada em verde, transferência apagada.
 *  `saldo`: posição de uma conta, sem sinal de entrada (negativo em vermelho). */
export function Valor({
  centavos,
  moeda = "BRL",
  tipo,
  saldo,
  className = "",
}: {
  centavos: number;
  moeda?: string;
  tipo?: string;
  saldo?: boolean;
  className?: string;
}) {
  const texto = formatarMoeda(centavos, moeda);
  if (saldo) {
    return (
      <span
        className={`valor num whitespace-nowrap ${centavos < 0 ? "text-saida" : "text-texto"} ${className}`}
      >
        {texto}
      </span>
    );
  }
  const cor = tipo === "transferencia" ? "text-texto-2" : centavos > 0 ? "text-entrada" : "text-texto";
  return (
    <span className={`valor num whitespace-nowrap ${cor} ${className}`}>
      {centavos > 0 && tipo !== "transferencia" ? `+${texto}` : texto}
    </span>
  );
}

/** Diálogo modal nativo (foco preso, Esc fecha). */
export function Dialogo({
  aberto,
  aoFechar,
  titulo,
  children,
}: {
  aberto: boolean;
  aoFechar: () => void;
  titulo: string;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (aberto && !d.open) d.showModal();
    if (!aberto && d.open) d.close();
  }, [aberto]);
  return (
    <dialog
      ref={ref}
      onClose={aoFechar}
      aria-label={titulo}
      className="m-auto max-h-[90dvh] w-[min(100vw-1.5rem,32rem)] overflow-y-auto rounded-cartao border border-borda bg-superficie p-5 text-texto shadow-cartao backdrop:bg-black/60"
    >
      <div className="mb-4 flex items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">{titulo}</h2>
        <button
          type="button"
          onClick={aoFechar}
          aria-label="Fechar"
          className="inline-flex size-11 items-center justify-center rounded-xl text-texto-2 hover:bg-superficie-2"
        >
          <X className="size-5" />
        </button>
      </div>
      {aberto ? children : null}
    </dialog>
  );
}

/** Barra de progresso; `marca` (0–1) desenha o ritmo ideal do mês. */
export function Barra({
  fracao,
  marca,
  cor = "var(--primaria)",
  rotulo,
}: {
  fracao: number;
  marca?: number;
  cor?: string;
  rotulo: string;
}) {
  const p = Math.min(Math.max(fracao, 0), 1);
  return (
    // biome-ignore lint/a11y/useSemanticElements: <meter> nativo não aceita o marcador do ritmo
    <span
      role="meter"
      aria-label={rotulo}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(fracao * 100)}
      className="relative mt-1.5 block h-2.5 rounded-full bg-superficie-2"
    >
      <span className="block h-2.5 rounded-full" style={{ width: `${p * 100}%`, backgroundColor: cor }} />
      {marca !== undefined && marca > 0 && marca < 1 ? (
        <span
          className="absolute -top-1 block h-4.5 w-0.5 rounded bg-texto"
          style={{ left: `${marca * 100}%` }}
          title="Ritmo ideal para hoje"
          aria-hidden
        />
      ) : null}
    </span>
  );
}

/** <select> de entidade quando há mais de uma editável. */
export function SeletorEntidade({
  entidades,
  valor,
  onChange,
}: {
  entidades: { id: string; nome: string }[];
  valor: string;
  onChange: (id: string) => void;
}) {
  if (entidades.length <= 1) return null;
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-sm font-medium">De quem é</span>
      <select
        value={valor}
        onChange={(e) => onChange(e.target.value)}
        className="min-h-11 rounded-xl border border-borda bg-superficie-2 px-3 text-base text-texto focus:border-primaria focus:outline-none"
      >
        {entidades.map((e) => (
          <option key={e.id} value={e.id}>
            {e.nome}
          </option>
        ))}
      </select>
    </label>
  );
}

/** Aviso fixo das telas que simulam (SPEC: não substitui contador nem assessor). */
export function AvisoSimulacao() {
  return (
    <p className="text-center text-xs text-texto-2">
      O painel organiza e simula; não substitui contador nem assessor de investimentos.
    </p>
  );
}
