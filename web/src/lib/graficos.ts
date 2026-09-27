// Ajudas dos gráficos: cores do tema, tooltips em HTML seguro e valores em pt-BR.
import { formatarMoeda } from "./formato";

/** Cores do tema atual, lidas dos tokens CSS (o gráfico acompanha claro e escuro). */
export type CoresTema = {
  fundo: string;
  superficie: string;
  superficie2: string;
  borda: string;
  texto: string;
  texto2: string;
  primaria: string;
  entrada: string;
  saida: string;
  invest: string;
  imposto: string;
  alerta: string;
  destaque: string;
  escuro: boolean;
  privado: boolean;
};

/**
 * Paleta categórica validada (skill de dataviz: ΔE para daltonismo ≥ 8 entre vizinhas e
 * visão normal ≥ 15), na ordem fixa; nunca gerar uma 9ª cor.
 */
const PALETA = {
  claro: ["#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#4a3aa7", "#e34948"],
  escuro: ["#3987e5", "#d95926", "#199e70", "#c98500", "#d55181", "#008300", "#9085e9", "#e66767"],
};

export type Slots = {
  azul: string;
  laranja: string;
  verdeAgua: string;
  amarelo: string;
  magenta: string;
  verde: string;
  violeta: string;
  vermelho: string;
};

export function paleta(c: Pick<CoresTema, "escuro">): Slots {
  const [azul, laranja, verdeAgua, amarelo, magenta, verde, violeta, vermelho] = c.escuro
    ? PALETA.escuro
    : PALETA.claro;
  return { azul, laranja, verdeAgua, amarelo, magenta, verde, violeta, vermelho } as Slots;
}

export function coresTema(): CoresTema {
  const css = getComputedStyle(document.documentElement);
  const v = (n: string) => css.getPropertyValue(`--${n}`).trim();
  return {
    fundo: v("fundo"),
    superficie: v("superficie"),
    superficie2: v("superficie-2"),
    borda: v("borda"),
    texto: v("texto"),
    texto2: v("texto-2"),
    primaria: v("primaria"),
    entrada: v("entrada"),
    saida: v("saida"),
    invest: v("invest"),
    imposto: v("imposto"),
    alerta: v("alerta"),
    destaque: v("destaque"),
    escuro: document.documentElement.dataset.theme !== "claro",
    privado: document.documentElement.dataset.privado === "sim",
  };
}

/** Escapa texto para o HTML do tooltip (nomes vêm do usuário). */
export function escapar(s: string): string {
  return s.replace(
    /[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] ?? c,
  );
}

const corValida = (c: string) => (/^#[0-9a-fA-F]{3,8}$/.test(c) || /^rgb/.test(c) ? c : "currentColor");

/** Amostra de cor em SVG (atributo fill, não style: a CSP deixa). */
export function amostra(cor: string): string {
  return `<svg class="grafico-amostra" width="10" height="10" aria-hidden="true"><rect width="10" height="10" rx="2" fill="${escapar(corValida(cor))}"/></svg>`;
}

/** Valor em reais, ou borrado no modo privacidade. */
export function moeda(centavos: number, privado: boolean): string {
  return privado ? "R$ ••••" : formatarMoeda(centavos);
}

const inteiro = new Intl.NumberFormat("pt-BR", { maximumFractionDigits: 0 });
const compacto = new Intl.NumberFormat("pt-BR", { notation: "compact", maximumFractionDigits: 1 });

/** Rótulo curto de eixo: "R$ 1,2 mil". */
export function moedaCurta(centavos: number, privado: boolean): string {
  if (privado) return "•••";
  if (Math.abs(centavos) < 100_000) return `R$ ${inteiro.format(Math.round(centavos / 100))}`; // abaixo de mil: "R$ 92"
  return `R$ ${compacto.format(centavos / 100)}`;
}

/** Tooltip com título e linhas (cor, nome, valor já formatado). */
export function tooltip(titulo: string, linhas: { cor?: string; nome: string; valor: string }[]): string {
  return `<div class="grafico-tooltip-titulo">${escapar(titulo)}</div>${linhas
    .map(
      (l) =>
        `<div class="grafico-tooltip-linha">${l.cor ? amostra(l.cor) : ""}<span>${escapar(l.nome)}</span><b>${escapar(l.valor)}</b></div>`,
    )
    .join("")}`;
}

const pct = new Intl.NumberFormat("pt-BR", {
  style: "percent",
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
});

/** Percentual em pt-BR ("12,3 %"); nulo vira "—". */
export function percentual(v: number | null | undefined): string {
  return v === null || v === undefined ? "—" : pct.format(v).replace(/\s?%/, " %");
}
