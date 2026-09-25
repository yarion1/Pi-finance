import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from "react";
import { useId } from "react";

const juntar = (...c: (string | false | undefined | null)[]) => c.filter(Boolean).join(" ");

type VarianteBotao = "primario" | "secundario" | "perigo" | "fantasma";

const estilosBotao: Record<VarianteBotao, string> = {
  primario: "bg-primaria text-primaria-texto hover:brightness-110",
  secundario: "bg-superficie-2 text-texto border border-borda hover:border-texto-2",
  perigo: "bg-transparent text-saida border border-saida/50 hover:bg-saida/10",
  fantasma: "bg-transparent text-texto-2 hover:text-texto hover:bg-superficie-2",
};

export function Botao({
  variante = "primario",
  carregando,
  className,
  children,
  disabled,
  ...resto
}: ButtonHTMLAttributes<HTMLButtonElement> & { variante?: VarianteBotao; carregando?: boolean }) {
  return (
    <button
      type="button"
      {...resto}
      disabled={disabled || carregando}
      aria-busy={carregando || undefined}
      className={juntar(
        "inline-flex min-h-11 items-center justify-center gap-2 rounded-xl px-4 text-sm font-semibold",
        "transition-[filter,background-color,border-color,color] duration-150 disabled:cursor-not-allowed disabled:opacity-50",
        estilosBotao[variante],
        className,
      )}
    >
      {carregando ? (
        <span className="size-4 animate-spin rounded-full border-2 border-current border-t-transparent" />
      ) : null}
      {children}
    </button>
  );
}

export function Campo({
  rotulo,
  dica,
  erro,
  className,
  ...resto
}: InputHTMLAttributes<HTMLInputElement> & { rotulo: string; dica?: ReactNode; erro?: string }) {
  const id = useId();
  return (
    <div className={juntar("flex flex-col gap-1.5", className)}>
      <label htmlFor={id} className="text-sm font-medium">
        {rotulo}
      </label>
      <input
        id={id}
        aria-invalid={erro ? true : undefined}
        aria-describedby={dica || erro ? `${id}-dica` : undefined}
        className={juntar(
          "min-h-11 rounded-xl border bg-superficie-2 px-3 text-base text-texto placeholder:text-texto-2/70",
          "transition-colors duration-150 focus:border-primaria focus:outline-none",
          erro ? "border-saida" : "border-borda",
        )}
        {...resto}
      />
      {erro || dica ? (
        <p id={`${id}-dica`} className={juntar("text-xs", erro ? "text-saida" : "text-texto-2")}>
          {erro ?? dica}
        </p>
      ) : null}
    </div>
  );
}

export function Selecao({
  rotulo,
  children,
  className,
  ...resto
}: SelectHTMLAttributes<HTMLSelectElement> & { rotulo: string }) {
  const id = useId();
  return (
    <div className={juntar("flex flex-col gap-1.5", className)}>
      <label htmlFor={id} className="text-sm font-medium">
        {rotulo}
      </label>
      <select
        id={id}
        className="min-h-11 rounded-xl border border-borda bg-superficie-2 px-3 text-base text-texto focus:border-primaria focus:outline-none"
        {...resto}
      >
        {children}
      </select>
    </div>
  );
}

export function Cartao({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <section
      className={juntar(
        "rounded-cartao border border-borda bg-superficie p-4 shadow-cartao sm:p-5",
        className,
      )}
    >
      {children}
    </section>
  );
}

export function TituloCartao({ children, acao }: { children: ReactNode; acao?: ReactNode }) {
  return (
    <div className="mb-3 flex items-center justify-between gap-3">
      <h2 className="text-base font-semibold">{children}</h2>
      {acao}
    </div>
  );
}

type TipoAviso = "erro" | "info" | "sucesso" | "alerta";

const estilosAviso: Record<TipoAviso, string> = {
  erro: "border-saida/40 bg-saida/10 text-saida",
  info: "border-primaria/40 bg-primaria/10 text-texto",
  sucesso: "border-entrada/40 bg-entrada/10 text-texto",
  alerta: "border-alerta/40 bg-alerta/10 text-texto",
};

export function Aviso({ tipo = "info", children }: { tipo?: TipoAviso; children: ReactNode }) {
  return (
    <div
      role={tipo === "erro" ? "alert" : "status"}
      className={juntar("rounded-xl border px-3 py-2.5 text-sm", estilosAviso[tipo])}
    >
      {children}
    </div>
  );
}

export function Etiqueta({
  children,
  cor = "texto-2",
}: {
  children: ReactNode;
  cor?: "texto-2" | "entrada" | "invest" | "imposto" | "alerta" | "primaria";
}) {
  const cores = {
    "texto-2": "text-texto-2 border-borda",
    entrada: "text-entrada border-entrada/40",
    invest: "text-invest border-invest/40",
    imposto: "text-imposto border-imposto/40",
    alerta: "text-alerta border-alerta/40",
    primaria: "text-primaria border-primaria/40",
  };
  return (
    <span
      className={juntar(
        "inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium",
        cores[cor],
      )}
    >
      {children}
    </span>
  );
}

export function Esqueleto({ className }: { className?: string }) {
  return <div className={juntar("esqueleto", className ?? "h-5 w-full")} aria-hidden />;
}

export function CarregandoLista({ linhas = 3 }: { linhas?: number }) {
  return (
    <div className="flex flex-col gap-3" role="status" aria-busy="true" aria-label="Carregando">
      {Array.from({ length: linhas }, (_, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: lista fixa de placeholders
        <Esqueleto key={i} className="h-12 w-full" />
      ))}
    </div>
  );
}

export function Vazio({
  icone,
  titulo,
  texto,
  acao,
}: {
  icone?: ReactNode;
  titulo: string;
  texto?: ReactNode;
  acao?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center gap-2 px-4 py-8 text-center">
      {icone ? <div className="text-texto-2">{icone}</div> : null}
      <p className="font-semibold">{titulo}</p>
      {texto ? <p className="max-w-sm text-sm text-texto-2">{texto}</p> : null}
      {acao ? <div className="mt-2">{acao}</div> : null}
    </div>
  );
}

export function Pagina({
  titulo,
  subtitulo,
  acao,
  children,
}: {
  titulo: string;
  subtitulo?: ReactNode;
  acao?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4 sm:gap-5">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{titulo}</h1>
          {subtitulo ? <p className="mt-1 text-sm text-texto-2">{subtitulo}</p> : null}
        </div>
        {acao}
      </header>
      {children}
    </div>
  );
}

/** Tela de autenticação: cartão centralizado, sem navegação. */
export function TelaAuth({
  titulo,
  subtitulo,
  children,
}: {
  titulo: string;
  subtitulo?: ReactNode;
  children: ReactNode;
}) {
  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-10">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center gap-2.5">
          <img src="/icone.svg" alt="" className="size-9" />
          <span className="text-lg font-bold tracking-tight">Finanças</span>
        </div>
        <h1 className="text-2xl font-bold tracking-tight">{titulo}</h1>
        {subtitulo ? <p className="mt-1.5 text-sm text-texto-2">{subtitulo}</p> : null}
        <div className="mt-6">{children}</div>
      </div>
    </main>
  );
}
