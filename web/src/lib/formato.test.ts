import { describe, expect, it } from "vitest";
import {
  decimalBR,
  diasEntre,
  formatarData,
  formatarMoeda,
  formatarPercentual,
  fracaoParaPercentual,
  hojeSP,
  mascararDocumento,
  percentualParaFracao,
  soData,
} from "./formato";

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

describe("taxas sem float", () => {
  it("percentual para fração", () => {
    expect(percentualParaFracao("0,99")).toBe("0.0099");
    expect(percentualParaFracao("1")).toBe("0.01");
    expect(percentualParaFracao("12,5 %")).toBe("0.125");
    expect(percentualParaFracao("100")).toBe("1");
    expect(percentualParaFracao("abc")).toBeNull();
    expect(percentualParaFracao("")).toBeNull();
  });
  it("fração para percentual", () => {
    expect(fracaoParaPercentual("0.00990000")).toBe("0,99");
    expect(fracaoParaPercentual("0.01000000")).toBe("1");
    expect(fracaoParaPercentual("0.125")).toBe("12,5");
    expect(fracaoParaPercentual("0.5")).toBe("50");
  });
  it("datas", () => {
    expect(soData("2026-10-05T00:00:00Z")).toBe("2026-10-05");
    expect(diasEntre("2026-09-25", "2026-10-05")).toBe(10);
    expect(hojeSP()).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});

it("zero negativo vira zero", () => {
  expect(semEspacoDuro(formatarMoeda(-0))).toBe("R$ 0,00");
});

describe("decimalBR", () => {
  it("lê quantidade e preço em pt-BR", () => {
    expect(decimalBR("1.234,56")).toBe("1234.56");
    expect(decimalBR("0,5")).toBe("0.5");
    expect(decimalBR("1.000")).toBe("1000");
    expect(decimalBR("30.5")).toBe("30.5");
    expect(decimalBR(" 100 ")).toBe("100");
    expect(decimalBR("abc")).toBeNull();
    expect(decimalBR("-1")).toBeNull();
    expect(decimalBR("")).toBeNull();
  });
});
