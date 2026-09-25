// Formato brasileiro sempre: R$ 1.234,56, 25/09/2026, 12,5 %.
// Valores chegam da API em centavos inteiros; o arredondamento é só na exibição.

const moedas = new Map<string, Intl.NumberFormat>();

export function formatarMoeda(centavos: number, moeda = "BRL"): string {
  let f = moedas.get(moeda);
  if (!f) {
    f = new Intl.NumberFormat("pt-BR", { style: "currency", currency: moeda });
    moedas.set(moeda, f);
  }
  return f.format(centavos / 100);
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
