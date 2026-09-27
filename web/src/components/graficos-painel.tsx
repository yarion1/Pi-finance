import { useCallback } from "react";
import { nomeMes } from "../lib/dados";
import { formatarData, formatarMoeda } from "../lib/formato";
import { type CoresTema, moeda, moedaCurta, paleta, percentual, tooltip } from "../lib/graficos";
import type { PontoPatrimonio } from "../lib/tipos";
import { Grafico, useCoresTema } from "./grafico";

const mesCurto = (m: string) => nomeMes(m.slice(0, 7)).slice(0, 3) + m.slice(2, 4);

/** Seletor de período das séries de tempo (6M, 1A, 2A, 5A). */
export function SeletorPeriodo({
  valor,
  onChange,
  nome,
  opcoes = [
    { meses: 6, rotulo: "6M" },
    { meses: 12, rotulo: "1A" },
    { meses: 24, rotulo: "2A" },
    { meses: 60, rotulo: "5A" },
  ],
}: {
  valor: number;
  onChange: (m: number) => void;
  nome: string;
  opcoes?: { meses: number; rotulo: string }[];
}) {
  return (
    <fieldset className="flex rounded-xl border border-borda p-0.5">
      <legend className="sr-only">Período</legend>
      {opcoes.map((p) => (
        <label
          key={p.meses}
          className="flex min-h-9 min-w-11 cursor-pointer items-center justify-center rounded-lg px-2 text-xs font-semibold text-texto-2 has-[:checked]:bg-destaque/15 has-[:checked]:text-primaria has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-primaria"
        >
          <input
            type="radio"
            name={nome}
            className="sr-only"
            checked={valor === p.meses}
            onChange={() => onChange(p.meses)}
          />
          {p.rotulo}
        </label>
      ))}
    </fieldset>
  );
}

/** Legenda HTML (nomes em texto, cor ao lado). */
export function Legenda({ itens }: { itens: { nome: string; cor: string; tracejado?: boolean }[] }) {
  return (
    <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-texto-2" aria-label="Legenda">
      {itens.map((i) => (
        <li key={i.nome} className="flex items-center gap-1.5">
          <svg width="14" height="10" aria-hidden="true">
            {i.tracejado ? (
              <line x1="0" y1="5" x2="14" y2="5" stroke={i.cor} strokeWidth="2" strokeDasharray="3 2" />
            ) : (
              <rect width="14" height="10" rx="2" fill={i.cor} />
            )}
          </svg>
          {i.nome}
        </li>
      ))}
    </ul>
  );
}

const eixoY = (c: CoresTema, formato: (v: number) => string) => ({
  type: "value",
  splitLine: { lineStyle: { color: c.borda } },
  axisLabel: { color: c.texto2, formatter: formato },
});
const eixoX = (c: CoresTema, dados: string[]) => ({
  type: "category",
  data: dados,
  axisTick: { show: false },
  axisLine: { lineStyle: { color: c.borda } },
  axisLabel: { color: c.texto2 },
});
const grade = { left: 8, right: 8, top: 12, bottom: 8, containLabel: true };

/** Patrimônio: ativos empilhados acima de zero, dívidas abaixo, e a linha do líquido. */
export function GraficoPatrimonio({ pontos }: { pontos: PontoPatrimonio[] }) {
  const montar = useCallback(
    (c: CoresTema) => {
      const { azul, laranja, verdeAgua, vermelho } = paleta(c);
      const serie = (nome: string, cor: string, dados: number[], pilha: string) => ({
        type: "line",
        name: nome,
        stack: pilha,
        areaStyle: { color: cor, opacity: 0.85 },
        lineStyle: { width: 0 },
        itemStyle: { color: cor },
        symbol: "none",
        data: dados,
      });
      return {
        grid: grade,
        tooltip: {
          trigger: "axis",
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (ps: any[]) => {
            const p = pontos[ps[0]?.dataIndex ?? 0];
            if (!p) return "";
            return tooltip(formatarData(p.data), [
              { cor: azul, nome: "Contas", valor: moeda(p.contas_centavos, c.privado) },
              { cor: laranja, nome: "Investimentos", valor: moeda(p.investimentos_centavos, c.privado) },
              { cor: verdeAgua, nome: "Bens", valor: moeda(p.bens_centavos, c.privado) },
              { cor: vermelho, nome: "Dívidas e cartões", valor: moeda(-p.passivos_centavos, c.privado) },
              { nome: "Líquido", valor: moeda(p.liquido_centavos, c.privado) },
            ]);
          },
        },
        xAxis: {
          ...eixoX(
            c,
            pontos.map((p) => mesCurto(p.data)),
          ),
          boundaryGap: false,
        },
        yAxis: eixoY(c, (v) => moedaCurta(v, c.privado)),
        series: [
          serie(
            "Contas",
            azul,
            pontos.map((p) => p.contas_centavos),
            "ativos",
          ),
          serie(
            "Investimentos",
            laranja,
            pontos.map((p) => p.investimentos_centavos),
            "ativos",
          ),
          serie(
            "Bens",
            verdeAgua,
            pontos.map((p) => p.bens_centavos),
            "ativos",
          ),
          serie(
            "Dívidas e cartões",
            vermelho,
            pontos.map((p) => -p.passivos_centavos),
            "passivos",
          ),
          {
            type: "line",
            name: "Líquido",
            data: pontos.map((p) => p.liquido_centavos),
            lineStyle: { color: c.texto, width: 2 },
            itemStyle: { color: c.texto },
            symbol: "circle",
            symbolSize: 6,
          },
        ],
      };
    },
    [pontos],
  );
  return (
    <>
      <Grafico
        montar={montar}
        altura="h-64"
        rotulo={`Patrimônio mês a mês: líquido de ${formatarMoeda(pontos[0]?.liquido_centavos ?? 0)} a ${formatarMoeda(pontos[pontos.length - 1]?.liquido_centavos ?? 0)}`}
        tabela={
          <table className="w-full">
            <thead>
              <tr className="text-left text-xs text-texto-2">
                <th className="py-1 font-medium">Mês</th>
                <th className="py-1 text-right font-medium">Ativos</th>
                <th className="py-1 text-right font-medium">Dívidas</th>
                <th className="py-1 text-right font-medium">Líquido</th>
              </tr>
            </thead>
            <tbody>
              {pontos.map((p) => (
                <tr key={p.data}>
                  <td className="py-1">{formatarData(p.data)}</td>
                  <td className="valor num py-1 text-right">{formatarMoeda(p.ativos_centavos)}</td>
                  <td className="valor num py-1 text-right">{formatarMoeda(p.passivos_centavos)}</td>
                  <td className="valor num py-1 text-right">{formatarMoeda(p.liquido_centavos)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        }
      />
      <LegendaTema
        itens={(c) => {
          const { azul, laranja, verdeAgua, vermelho } = paleta(c);
          return [
            { nome: "Contas", cor: azul },
            { nome: "Investimentos", cor: laranja },
            { nome: "Bens", cor: verdeAgua },
            { nome: "Dívidas e cartões", cor: vermelho },
            { nome: "Líquido", cor: c.texto, tracejado: true },
          ];
        }}
      />
    </>
  );
}

/** Legenda com as cores do tema atual (acompanha a troca de tema). */
function LegendaTema({
  itens,
}: {
  itens: (c: CoresTema) => { nome: string; cor: string; tracejado?: boolean }[];
}) {
  return <Legenda itens={itens(useCoresTema())} />;
}

/** Rentabilidade acumulada: carteira contra CDI, IPCA e Ibovespa. */
export function GraficoRentabilidade({
  datas,
  series,
}: {
  datas: string[];
  series: { nome: string; valores: (number | null)[]; principal?: boolean; cor: number }[];
}) {
  const montar = useCallback(
    (c: CoresTema) => {
      const { azul, laranja, verdeAgua } = paleta(c);
      const cores = [c.destaque, azul, laranja, verdeAgua];
      return {
        grid: { ...grade, right: 96 }, // espaço para o rótulo no fim de cada linha
        tooltip: {
          trigger: "axis",
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (ps: any[]) =>
            tooltip(
              formatarData(datas[ps[0]?.dataIndex ?? 0] ?? ""),
              ps.map((p) => ({ cor: p.color, nome: p.seriesName, valor: percentual(p.value) })),
            ),
        },
        xAxis: { ...eixoX(c, datas.map(mesCurto)), boundaryGap: false },
        yAxis: eixoY(c, (v) => percentual(v)),
        series: series.map((s) => ({
          type: "line",
          name: s.nome,
          data: s.valores,
          connectNulls: false,
          symbol: "circle",
          symbolSize: s.principal ? 7 : 5,
          lineStyle: { width: s.principal ? 3 : 2, color: cores[s.cor] },
          itemStyle: { color: cores[s.cor] },
          endLabel: {
            show: true,
            color: c.texto2,
            fontSize: 11,
            // biome-ignore lint/suspicious/noExplicitAny: parâmetros do rótulo do ECharts
            formatter: (p: any) => `${p.seriesName} ${percentual(p.value)}`,
          },
        })),
      };
    },
    [datas, series],
  );
  return (
    <Grafico
      montar={montar}
      altura="h-64"
      rotulo={`Rentabilidade acumulada: ${series.map((s) => `${s.nome} ${percentual(s.valores[s.valores.length - 1])}`).join(", ")}`}
      tabela={
        <table className="w-full">
          <thead>
            <tr className="text-left text-xs text-texto-2">
              <th className="py-1 font-medium">Data</th>
              {series.map((s) => (
                <th key={s.nome} className="py-1 text-right font-medium">
                  {s.nome}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {datas.map((d, i) => (
              <tr key={d}>
                <td className="py-1">{formatarData(d)}</td>
                {series.map((s) => (
                  <td key={s.nome} className="num py-1 text-right">
                    {percentual(s.valores[i])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      }
    />
  );
}

/** Barras de valores por mês com uma cor (faturamento, DAS, proventos, parcelas futuras). */
export function GraficoMensal({
  meses,
  valores,
  cor,
  rotulo,
  nome,
  aoClicar,
}: {
  meses: string[];
  valores: number[];
  cor: (c: CoresTema) => string;
  rotulo: string;
  nome: string;
  aoClicar?: (mes: string) => void;
}) {
  const montar = useCallback(
    (c: CoresTema) => ({
      grid: grade,
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
        formatter: (ps: any[]) =>
          tooltip(nomeMes((meses[ps[0]?.dataIndex ?? 0] ?? "").slice(0, 7)), [
            { cor: cor(c), nome, valor: moeda(ps[0]?.value ?? 0, c.privado) },
          ]),
      },
      xAxis: eixoX(c, meses.map(mesCurto)),
      yAxis: eixoY(c, (v) => moedaCurta(v, c.privado)),
      series: [
        {
          type: "bar",
          name: nome,
          barMaxWidth: 28,
          itemStyle: { color: cor(c), borderRadius: [4, 4, 0, 0] },
          data: valores,
        },
      ],
    }),
    [meses, valores, cor, nome],
  );
  return (
    <Grafico
      montar={montar}
      altura="h-52"
      rotulo={rotulo}
      aoClicar={aoClicar ? (p) => meses[p.dataIndex] && aoClicar(meses[p.dataIndex] as string) : undefined}
      tabela={
        <table className="w-full">
          <tbody>
            {meses.map((m, i) => (
              <tr key={m}>
                <td className="py-1">{nomeMes(m.slice(0, 7))}</td>
                <td className="valor num py-1 text-right">{formatarMoeda(valores[i] ?? 0)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      }
    />
  );
}

/** Alocação atual × alvo por classe, em barras lado a lado. */
export function GraficoAlocacao({
  classes,
}: {
  classes: { nome: string; atual: number; alvo: number | null }[];
}) {
  const montar = useCallback(
    (c: CoresTema) => {
      const { azul } = paleta(c);
      return {
        grid: { ...grade, left: 8 },
        tooltip: {
          trigger: "axis",
          axisPointer: { type: "shadow" },
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (ps: any[]) => {
            const k = classes[ps[0]?.dataIndex ?? 0];
            return k
              ? tooltip(k.nome, [
                  { cor: c.destaque, nome: "Atual", valor: percentual(k.atual) },
                  { cor: azul, nome: "Alvo", valor: percentual(k.alvo) },
                ])
              : "";
          },
        },
        xAxis: eixoY(c, (v) => percentual(v)),
        yAxis: {
          ...eixoX(
            c,
            classes.map((k) => k.nome),
          ),
          inverse: true,
        },
        series: [
          {
            type: "bar",
            name: "Atual",
            barMaxWidth: 12,
            itemStyle: { color: c.destaque, borderRadius: [0, 4, 4, 0] },
            data: classes.map((k) => k.atual),
          },
          {
            type: "bar",
            name: "Alvo",
            barMaxWidth: 12,
            itemStyle: { color: azul, borderRadius: [0, 4, 4, 0] },
            data: classes.map((k) => k.alvo),
          },
        ],
      };
    },
    [classes],
  );
  return (
    <Grafico
      montar={montar}
      altura={classes.length > 4 ? "h-72" : "h-52"}
      rotulo={`Alocação por classe: ${classes.map((k) => `${k.nome} ${percentual(k.atual)} (alvo ${percentual(k.alvo)})`).join("; ")}`}
    />
  );
}

/** Leque do Monte Carlo: faixa entre os percentis 10 e 90, a mediana e o alvo. */
export function GraficoLeque({
  anos,
  p10,
  p50,
  p90,
  alvo,
}: {
  anos: number[];
  p10: number[];
  p50: number[];
  p90: number[];
  alvo: number;
}) {
  const montar = useCallback(
    (c: CoresTema) => {
      const { azul } = paleta(c);
      return {
        grid: grade,
        tooltip: {
          trigger: "axis",
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (ps: any[]) => {
            const i = ps[0]?.dataIndex ?? 0;
            return tooltip(`Ano ${anos[i]}`, [
              { nome: "Bom (90 %)", valor: moeda(p90[i] ?? 0, c.privado) },
              { cor: azul, nome: "Provável", valor: moeda(p50[i] ?? 0, c.privado) },
              { nome: "Ruim (10 %)", valor: moeda(p10[i] ?? 0, c.privado) },
            ]);
          },
        },
        xAxis: {
          ...eixoX(
            c,
            anos.map((a) => `${a}`),
          ),
          boundaryGap: false,
          name: "anos",
          nameTextStyle: { color: c.texto2 },
        },
        yAxis: eixoY(c, (v) => moedaCurta(v, c.privado)),
        series: [
          { type: "line", stack: "faixa", data: p10, lineStyle: { width: 0 }, symbol: "none", silent: true },
          {
            type: "line",
            stack: "faixa",
            data: p90.map((v, i) => v - (p10[i] ?? 0)),
            lineStyle: { width: 0 },
            symbol: "none",
            areaStyle: { color: azul, opacity: 0.2 },
            silent: true,
          },
          {
            type: "line",
            name: "Provável",
            data: p50,
            lineStyle: { color: azul, width: 3 },
            itemStyle: { color: azul },
            symbol: "circle",
            symbolSize: 6,
            markLine:
              alvo > 0
                ? {
                    silent: true,
                    symbol: "none",
                    lineStyle: { color: c.entrada, type: "dashed", width: 2 },
                    label: { color: c.texto2, formatter: "independência", position: "insideEndTop" },
                    data: [{ yAxis: alvo }],
                  }
                : undefined,
          },
        ],
      };
    },
    [anos, p10, p50, p90, alvo],
  );
  return (
    <Grafico
      montar={montar}
      altura="h-64"
      rotulo={`Patrimônio investível em ${anos[anos.length - 1]} anos: provável ${formatarMoeda(p50[p50.length - 1] ?? 0)}, entre ${formatarMoeda(p10[p10.length - 1] ?? 0)} e ${formatarMoeda(p90[p90.length - 1] ?? 0)}`}
    />
  );
}

/** Medidor do teto (ex.: MEI): quanto já foi, a projeção do ano e o teto. */
export function GraficoMedidor({ valor, projecao, teto }: { valor: number; projecao: number; teto: number }) {
  const montar = useCallback(
    (c: CoresTema) => {
      const fracao = teto > 0 ? valor / teto : 0;
      const proj = teto > 0 ? projecao / teto : 0;
      const cor = proj >= 1 ? c.saida : proj >= 0.8 ? c.alerta : c.entrada;
      const base = {
        type: "gauge",
        min: 0,
        max: 1.2,
        startAngle: 200,
        endAngle: -20,
        radius: "95%",
        center: ["50%", "62%"],
        axisTick: { show: false },
        splitLine: { show: false },
        axisLabel: { show: false },
        anchor: { show: false },
      };
      return {
        series: [
          {
            // o arco e o número: quanto já foi faturado
            ...base,
            progress: { show: true, width: 14, itemStyle: { color: cor } },
            axisLine: {
              lineStyle: {
                width: 14,
                color: [
                  [0.8 / 1.2, c.superficie2],
                  [1 / 1.2, c.borda],
                  [1, c.superficie2],
                ],
              },
            },
            pointer: { show: false },
            title: { show: false },
            detail: {
              valueAnimation: false,
              offsetCenter: [0, "8%"],
              color: c.texto,
              fontSize: 22,
              fontWeight: 700,
              formatter: () => percentual(fracao),
            },
            data: [{ value: Math.min(fracao, 1.2) }],
          },
          {
            // o ponteiro: onde termina o ano no ritmo atual
            ...base,
            progress: { show: false },
            axisLine: { show: false },
            pointer: { show: true, length: "55%", width: 4, itemStyle: { color: c.texto2 } },
            title: { show: true, offsetCenter: [0, "38%"], color: c.texto2, fontSize: 12 },
            detail: { show: false },
            data: [{ value: Math.min(proj, 1.2), name: `ponteiro: projeção do ano (${percentual(proj)})` }],
          },
        ],
      };
    },
    [valor, projecao, teto],
  );
  return (
    <Grafico
      montar={montar}
      altura="h-52"
      rotulo={`Teto: ${percentual(teto > 0 ? valor / teto : 0)} usado; projeção do ano ${percentual(teto > 0 ? projecao / teto : 0)} de ${formatarMoeda(teto)}`}
    />
  );
}

/** Fator R mês a mês com a linha dos 28 %. */
export function GraficoFatorR({
  meses,
  valores,
  minimo,
}: {
  meses: string[];
  valores: number[];
  minimo: number;
}) {
  const montar = useCallback(
    (c: CoresTema) => {
      const { azul } = paleta(c);
      return {
        grid: grade,
        tooltip: {
          trigger: "axis",
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (ps: any[]) =>
            tooltip(nomeMes(meses[ps[0]?.dataIndex ?? 0] ?? ""), [
              { cor: azul, nome: "Fator R", valor: percentual(ps[0]?.value) },
            ]),
        },
        xAxis: { ...eixoX(c, meses.map(mesCurto)), boundaryGap: false },
        yAxis: {
          ...eixoY(c, (v) => percentual(v)),
          min: 0,
          max: (v: { max: number }) => Math.max(0.4, v.max),
        },
        series: [
          {
            type: "line",
            name: "Fator R",
            data: valores,
            lineStyle: { color: azul, width: 3 },
            itemStyle: { color: azul },
            symbol: "circle",
            symbolSize: 6,
            markLine: {
              silent: true,
              symbol: "none",
              lineStyle: { color: c.alerta, type: "dashed", width: 2 },
              label: {
                color: c.texto2,
                formatter: `${percentual(minimo)}: Anexo III`,
                position: "insideEndTop",
              },
              data: [{ yAxis: minimo }],
            },
          },
        ],
      };
    },
    [meses, valores, minimo],
  );
  return (
    <Grafico
      montar={montar}
      altura="h-52"
      rotulo={`Fator R nos últimos meses, de ${percentual(valores[0])} a ${percentual(valores[valores.length - 1])}; o mínimo para o Anexo III é ${percentual(minimo)}`}
    />
  );
}
