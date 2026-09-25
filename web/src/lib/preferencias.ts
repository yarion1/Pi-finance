// Preferências só deste aparelho (tema e modo privacidade). O armazenamento
// pode falhar (aba anônima, dados bloqueados): tudo tem padrão.

import { useCallback, useEffect, useState } from "react";

function ler(chave: string): string | null {
  try {
    return localStorage.getItem(chave);
  } catch {
    return null;
  }
}

function gravar(chave: string, valor: string) {
  try {
    localStorage.setItem(chave, valor);
  } catch {
    // sem armazenamento: vale só nesta visita
  }
}

export type Tema = "escuro" | "claro";

export function aplicarPreferenciasSalvas() {
  document.documentElement.dataset.theme = ler("financas.tema") === "claro" ? "claro" : "escuro";
  document.documentElement.dataset.privado = ler("financas.privado") === "sim" ? "sim" : "nao";
}

function usePreferencia<T extends string>(chave: string, atributo: "theme" | "privado", inicial: T) {
  const [valor, setValor] = useState<T>(() => (document.documentElement.dataset[atributo] as T) || inicial);
  useEffect(() => {
    document.documentElement.dataset[atributo] = valor;
    if (atributo === "theme") {
      document
        .querySelector('meta[name="theme-color"]')
        ?.setAttribute("content", valor === "claro" ? "#f5f7fa" : "#0b0f14");
    }
    gravar(chave, valor);
  }, [chave, atributo, valor]);
  return [valor, setValor] as const;
}

export function useTema() {
  const [tema, setTema] = usePreferencia<Tema>("financas.tema", "theme", "escuro");
  const alternar = useCallback(() => setTema((t) => (t === "escuro" ? "claro" : "escuro")), [setTema]);
  return { tema, alternar };
}

export function usePrivacidade() {
  const [privado, setPrivado] = usePreferencia<"sim" | "nao">("financas.privado", "privado", "nao");
  const alternar = useCallback(() => setPrivado((p) => (p === "sim" ? "nao" : "sim")), [setPrivado]);
  return { privado: privado === "sim", alternar };
}
