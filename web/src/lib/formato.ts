// Formato brasileiro sempre: R$ 1.234,56, 25/09/2026, 12,5 %.
// Valores chegam da API em centavos inteiros; o arredondamento é só na exibição.

const moedas = new Map<string, Intl.NumberFormat>();

export function formatarMoeda(centavos: number, moeda = "BRL"): string {
  let f = moedas.get(moeda);
  if (!f) {
    f = new Intl.NumberFormat("pt-BR", { style: "currency", currency: moeda });
    moedas.set(moeda, f);
  }
  return f.format((centavos || 0) / 100); // nunca "-R$ 0,00"
}

const datas = new Intl.DateTimeFormat("pt-BR", { timeZone: "America/Sao_Paulo" });
const datasHoras = new Intl.DateTimeFormat("pt-BR", {
  timeZone: "America/Sao_Paulo",
  dateStyle: "short",
  timeStyle: "short",
});

/** "2026-09-25" (data pura) ou ISO com horário. */
export function formatarData(valor: string): string {
  if (/^\d{4}-\d{2}-\d{2}$/.test(valor)) {
    const [a, m, d] = valor.split("-");
    return `${d}/${m}/${a}`;
  }
  return datas.format(new Date(valor));
}

export function formatarDataHora(iso: string): string {
  return datasHoras.format(new Date(iso));
}

export function formatarPercentual(fracao: number, casas = 1): string {
  return new Intl.NumberFormat("pt-BR", {
    style: "percent",
    minimumFractionDigits: casas,
    maximumFractionDigits: casas,
  }).format(fracao);
}

/** Máscara de CPF/CNPJ enquanto digita. */
export function mascararDocumento(texto: string, tipo: "PF" | "PJ"): string {
  const d = texto.replace(/\D/g, "").slice(0, tipo === "PF" ? 11 : 14);
  if (tipo === "PF") {
    return d
      .replace(/^(\d{3})(\d)/, "$1.$2")
      .replace(/^(\d{3})\.(\d{3})(\d)/, "$1.$2.$3")
      .replace(/\.(\d{3})(\d)/, ".$1-$2");
  }
  return d
    .replace(/^(\d{2})(\d)/, "$1.$2")
    .replace(/^(\d{2})\.(\d{3})(\d)/, "$1.$2.$3")
    .replace(/\.(\d{3})(\d)/, ".$1/$2")
    .replace(/(\d{4})(\d)/, "$1-$2");
}

/** Lê "1.234,56", "-45", "45,5" ou "R$ 10" em centavos; null se inválido. Mesmas regras de core.ParseBRL. */
export function lerCentavos(texto: string): number | null {
  let s = texto.replace(/\s| /g, "").replace(/^R\$/, "");
  let negativo = false;
  if (s.startsWith("-")) {
    negativo = true;
    s = s.slice(1).replace(/^R\$/, "");
  }
  if (!s) return null;
  let inteiro: string;
  let fracao = "";
  if (s.includes(",")) {
    const partes = s.split(",");
    if (partes.length !== 2) return null;
    inteiro = (partes[0] ?? "").replace(/\./g, "");
    fracao = partes[1] ?? "";
  } else if ((s.match(/\./g) ?? []).length === 1 && s.length - s.lastIndexOf(".") - 1 <= 2) {
    inteiro = s.slice(0, s.lastIndexOf("."));
    fracao = s.slice(s.lastIndexOf(".") + 1);
  } else {
    inteiro = s.replace(/\./g, "");
  }
  if (inteiro === "") inteiro = "0";
  if (fracao.length > 2 || !/^\d+$/.test(inteiro) || (fracao && !/^\d+$/.test(fracao))) return null;
  const n = Number(inteiro) * 100 + Number(fracao.padEnd(2, "0"));
  if (!Number.isSafeInteger(n)) return null;
  return negativo ? -n : n;
}

/** Centavos para o campo de edição: 123456 → "1.234,56". */
export function centavosParaTexto(c: number): string {
  return new Intl.NumberFormat("pt-BR", { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(
    c / 100,
  );
}

/** Taxa digitada em % ("0,99") para a fração que a API guarda ("0.0099"), sem float. null se inválida. */
export function percentualParaFracao(texto: string): string | null {
  const s = texto.replace(/\s|%/g, "").replace(",", ".");
  if (!/^\d+(\.\d+)?$/.test(s)) return null;
  const [inteiro = "0", fracao = ""] = s.split(".");
  const casas = fracao.length + 2;
  const cheio = (inteiro + fracao).padStart(casas + 1, "0");
  const i = cheio.slice(0, cheio.length - casas).replace(/^0+(?=\d)/, "");
  const f = cheio.slice(cheio.length - casas).replace(/0+$/, "");
  return f ? `${i}.${f}` : i;
}

/** Fração da API ("0.0099") para % na tela ("0,99"). */
export function fracaoParaPercentual(fracao: string): string {
  const [inteiro = "0", resto = ""] = fracao.split(".");
  const digitos = inteiro + resto.padEnd(2, "0");
  const casas = Math.max(resto.length - 2, 0);
  const cheio = digitos.padStart(casas + 1, "0");
  const i = cheio.slice(0, cheio.length - casas).replace(/^0+(?=\d)/, "");
  const f = cheio.slice(cheio.length - casas).replace(/0+$/, "");
  return f ? `${i},${f}` : i;
}

/** Só a data ("2026-10-05T00:00:00Z" → "2026-10-05"), sem passar pelo fuso. */
export const soData = (valor: string) => valor.slice(0, 10);

/** Hoje em São Paulo, "AAAA-MM-DD". */
export const hojeSP = () =>
  new Intl.DateTimeFormat("en-CA", { timeZone: "America/Sao_Paulo" }).format(new Date());

/** Dias entre duas datas "AAAA-MM-DD" (b − a). */
export function diasEntre(a: string, b: string): number {
  return Math.round((Date.parse(`${b}T00:00:00Z`) - Date.parse(`${a}T00:00:00Z`)) / 86_400_000);
}

/**
 * Quantidade ou preço digitado em pt-BR ("1.234,56", "0,5", "1.000") para o texto que a
 * API lê ("1234.56"), sem float. Sem vírgula, "1.000" é mil; "30.5" é trinta e meio. null se inválido.
 */
export function decimalBR(texto: string): string | null {
  let s = texto.replace(/\s/g, "");
  if (s.includes(",")) s = s.replace(/\./g, "").replace(",", ".");
  else if (/^\d{1,3}(\.\d{3})+$/.test(s)) s = s.replace(/\./g, "");
  return /^\d+(\.\d+)?$/.test(s) ? s : null;
}
