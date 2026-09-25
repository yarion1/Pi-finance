import { describe, expect, it } from "vitest";
import { formatarData, formatarMoeda, formatarPercentual, mascararDocumento } from "./formato";

const semEspacoDuro = (s: string) => s.replace(/ /g, " ");

describe("formato pt-BR", () => {
  it("moeda em centavos", () => {
    expect(semEspacoDuro(formatarMoeda(123456))).toBe("R$ 1.234,56");
    expect(semEspacoDuro(formatarMoeda(-4500))).toBe("-R$ 45,00");
    expect(semEspacoDuro(formatarMoeda(8100000))).toBe("R$ 81.000,00");
    expect(semEspacoDuro(formatarMoeda(1999, "USD"))).toBe("US$ 19,99");
  });

  it("datas", () => {
    expect(formatarData("2026-09-25")).toBe("25/09/2026");
    expect(formatarData("2026-09-25T02:00:00Z")).toBe("24/09/2026");
  });

  it("percentual com vírgula", () => {
    expect(semEspacoDuro(formatarPercentual(0.125))).toMatch(/^12,5\s?%$/);
    expect(semEspacoDuro(formatarPercentual(0.28, 0))).toMatch(/^28\s?%$/);
  });

  it("máscara de documento", () => {
    expect(mascararDocumento("52998224725", "PF")).toBe("529.982.247-25");
    expect(mascararDocumento("11222333000181", "PJ")).toBe("11.222.333/0001-81");
    expect(mascararDocumento("5299", "PF")).toBe("529.9");
  });
});

import { centavosParaTexto, lerCentavos } from "./formato";

describe("valores digitados", () => {
  it("lê como o servidor", () => {
    expect(lerCentavos("1.234,56")).toBe(123456);
    expect(lerCentavos("R$ 1.234,56")).toBe(123456);
    expect(lerCentavos("-45")).toBe(-4500);
    expect(lerCentavos("45,5")).toBe(4550);
    expect(lerCentavos("1234.56")).toBe(123456);
    expect(lerCentavos("0")).toBe(0);
    for (const ruim of ["", "abc", "1,2,3", "1,234", "R$"]) expect(lerCentavos(ruim)).toBeNull();
  });
  it("ida e volta", () => {
    for (const c of [0, 5, 123456, -4500]) expect(lerCentavos(centavosParaTexto(c))).toBe(c);
  });
});
