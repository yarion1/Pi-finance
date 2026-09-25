import { describe, expect, it } from "vitest";
import { arvoreCategorias, limitesMes, nomeMes, rotuloCategoria, somarMes } from "./dados";
import type { Categoria } from "./tipos";

const cat = (
  id: string,
  nome: string,
  pai: string | null = null,
  entidade: string | null = null,
): Categoria => ({
  id,
  nome,
  pai_id: pai,
  entidade_id: entidade,
  icone: null,
  cor: null,
  tipo: "gasto",
  ordem: 0,
});

describe("dados", () => {
  it("meses", () => {
    expect(somarMes("2026-01", -1)).toBe("2025-12");
    expect(somarMes("2026-12", 1)).toBe("2027-01");
    expect(limitesMes("2026-02")).toEqual(["2026-02-01", "2026-02-28"]);
    expect(limitesMes("2028-02")).toEqual(["2028-02-01", "2028-02-29"]);
    expect(nomeMes("2026-09")).toBe("Setembro de 2026");
  });

  it("categorias em árvore e rótulo", () => {
    const todas = [
      cat("m", "Moradia"),
      cat("e", "Energia", "m"),
      cat("x", "Só da Ana", null, "ana"),
      cat("y", "Só do Bruno", null, "bruno"),
    ];
    const arvore = arvoreCategorias(todas, "ana");
    expect(arvore.map((c) => c.nome)).toEqual(["Moradia", "Só da Ana"]);
    expect(arvore[0]?.filhas.map((c) => c.nome)).toEqual(["Energia"]);
    expect(rotuloCategoria(todas, "e")).toBe("Moradia › Energia");
    expect(rotuloCategoria(todas, null)).toBe("Sem categoria");
  });
});
