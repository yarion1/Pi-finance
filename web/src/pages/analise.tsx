import { useQuery } from "@tanstack/react-query";
import { BarChart3, List, Waves } from "lucide-react";
import { useCallback, useState } from "react";
import { Link, NavLink, useNavigate, useSearchParams } from "react-router";
import { Barra, SeletorMes } from "../components/extras";
import { Grafico } from "../components/grafico";
import { Aviso, Cartao, Esqueleto, Pagina, TituloCartao, Vazio } from "../components/ui";
import { mensagemDe, obter } from "../lib/api";
import { mesAtual, nomeMes } from "../lib/dados";
import { formatarData, formatarMoeda } from "../lib/formato";
import { type CoresTema, moeda, moedaCurta, tooltip } from "../lib/graficos";
import type { AnaliseFluxo, AnaliseGastos } from "../lib/tipos";

/** Lista, gráficos e fluxo: as três vistas da tela de gastos, com o mesmo mês. */
export function AbasGastos() {
  const [params] = useSearchParams();
  const mes = params.get("mes");
  const q = mes ? `?mes=${mes}` : "";
  const abas = [
    { para: "/gastos", nome: "Lista", icone: List },
    { para: "/gastos/graficos", nome: "Gráficos", icone: BarChart3 },
    { para: "/gastos/fluxo", nome: "Para onde vai", icone: Waves },
  ];
  return (
    <nav aria-label="Vistas dos gastos" className="flex gap-2 overflow-x-auto pb-1">
      {abas.map((a) => (
        <NavLink
          key={a.para}
          to={a.para + q}
          end
          className="flex min-h-11 shrink-0 items-center gap-2 rounded-xl border border-borda px-4 text-sm font-semibold text-texto-2 aria-[current=page]:border-primaria aria-[current=page]:text-texto"
        >
          <a.icone className="size-4" aria-hidden /> {a.nome}
        </NavLink>
      ))}
    </nav>
  );
}

function useMes(): [string, (m: string) => void] {
  const [params, setParams] = useSearchParams();
  const mes = params.get("mes") ?? mesAtual();
  return [mes, (m) => setParams({ mes: m }, { replace: true })];
}

const mesCurto = (m: string) => nomeMes(m).slice(0, 3) + m.slice(2, 4);

function SemDados() {
  return (
    <Vazio
      titulo="Ainda sem gastos neste período"
      texto="Importe um extrato ou conecte um banco para ver os gráficos."
      acao={
        <Link to="/importar" className="text-sm font-semibold text-primaria">
          Importar extrato
        </Link>
      }
    />
  );
}

// ---------------------------------------------------------------------------
// Gráficos dos gastos
// ---------------------------------------------------------------------------

const periodos = [
  { meses: 6, nome: "6M" },
  { meses: 12, nome: "1A" },
  { meses: 24, nome: "2A" },
  { meses: 60, nome: "5A" },
];

export function GastosGraficos() {
  const [mes, setMes] = useMes();
  const [meses, setMeses] = useState(12);
  const navegar = useNavigate();
  const dados = useQuery({
    queryKey: ["analise", "gastos", mes, meses],
    queryFn: () => obter<AnaliseGastos>(`/api/analise/gastos?mes=${mes}&meses=${meses}`),
  });
  const a = dados.data;
  const filtroCategoria = useCallback(
    (m: string, id?: string | null) =>
      navegar(
        id
          ? `/gastos?mes=${m}&categoria=${id}`
          : id === ""
            ? `/gastos?mes=${m}&sem_categoria=1`
            : `/gastos?mes=${m}`,
      ),
    [navegar],
  );

  const treemap = useCallback(
    (c: CoresTema) => ({
      tooltip: {
        // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
        formatter: (p: any) =>
          tooltip(
            (p.treePathInfo as { name: string }[])
              .slice(1)
              .map((x) => x.name)
              .join(" › "),
            [{ nome: "Gasto", valor: moeda(p.value, c.privado) }],
          ),
      },
      series: [
        {
          type: "treemap",
          roam: false,
          nodeClick: false,
          breadcrumb: { show: false },
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          upperLabel: { show: true, height: 22, color: "#111", fontWeight: 600 },
          label: {
            color: "#111",
            fontSize: 12,
            lineHeight: 16,
            // biome-ignore lint/suspicious/noExplicitAny: parâmetros do rótulo do ECharts
            formatter: (p: any) => `${p.name}\n${moedaCurta(p.value, c.privado)}`,
          },
          levels: [
            { itemStyle: { borderColor: c.superficie, borderWidth: 2, gapWidth: 2 } },
            {
              itemStyle: { borderColor: c.superficie, borderWidth: 2, gapWidth: 1 },
              colorSaturation: [0.45, 0.75],
            },
          ],
          data: (a?.treemap ?? []).map((g) => ({
            name: g.nome,
            value: g.valor_centavos,
            id: `m:${g.id}`,
            categoria: g.id,
            itemStyle: { color: g.cor ?? c.texto2 },
            children: g.filhas.map((f) => ({
              name: f.nome,
              value: f.valor_centavos,
              id: `f:${f.id}`,
              categoria: f.id,
            })),
          })),
        },
      ],
    }),
    [a],
  );

  const barras = useCallback(
    (c: CoresTema) => ({
      grid: { left: 8, right: 8, top: 12, bottom: 8, containLabel: true },
      legend: { show: false },
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
        formatter: (ps: any[]) => {
          const total = ps.reduce((s, p) => s + (p.value ?? 0), 0);
          return tooltip(nomeMes(a?.meses[ps[0]?.dataIndex ?? 0] ?? ""), [
            ...ps
              .filter((p) => p.value > 0)
              .reverse()
              .map((p) => ({ cor: p.color, nome: p.seriesName, valor: moeda(p.value, c.privado) })),
            { nome: "Total", valor: moeda(total, c.privado) },
          ]);
        },
      },
      xAxis: {
        type: "category",
        data: (a?.meses ?? []).map(mesCurto),
        axisLine: { lineStyle: { color: c.borda } },
        axisTick: { show: false },
        axisLabel: { color: c.texto2 },
      },
      yAxis: {
        type: "value",
        splitLine: { lineStyle: { color: c.borda } },
        axisLabel: { color: c.texto2, formatter: (v: number) => moedaCurta(v, c.privado) },
      },
      series: (a?.series ?? []).map((s) => ({
        type: "bar",
        stack: "gastos",
        name: s.nome,
        barMaxWidth: 28,
        emphasis: { focus: "series" },
        itemStyle: { color: s.cor ?? c.texto2, borderColor: c.superficie, borderWidth: 1, borderRadius: 2 },
        data: s.valores_centavos,
      })),
    }),
    [a],
  );

  const calendario = useCallback(
    (c: CoresTema) => {
      const [ano, m] = mes.split("-").map(Number);
      const fim = new Date(Date.UTC(ano ?? 2026, m ?? 1, 0)).toISOString().slice(0, 10);
      const inicio = new Date(Date.UTC(ano ?? 2026, (m ?? 1) - 6, 1)).toISOString().slice(0, 10);
      const valores = (a?.dias ?? []).filter((d) => d.data >= inicio && d.data <= fim);
      const ordenados = valores.map((d) => d.total_centavos).sort((x, y) => x - y);
      const teto = ordenados[Math.floor(ordenados.length * 0.95)] ?? 1;
      return {
        tooltip: {
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (p: any) =>
            tooltip(formatarData(p.value[0]), [{ nome: "Gasto", valor: moeda(p.value[1], c.privado) }]),
        },
        visualMap: { show: false, min: 0, max: teto, inRange: { color: [c.superficie2, c.saida] } },
        calendar: {
          range: [inicio, fim],
          top: 22,
          left: 22,
          right: 4,
          bottom: 4,
          cellSize: ["auto", "auto"],
          splitLine: { show: false },
          itemStyle: { color: c.superficie2, borderColor: c.superficie, borderWidth: 2 },
          yearLabel: { show: false },
          dayLabel: {
            firstDay: 0,
            nameMap: ["D", "S", "T", "Q", "Q", "S", "S"],
            color: c.texto2,
            fontSize: 10,
          },
          monthLabel: {
            nameMap: ["jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"],
            color: c.texto2,
            fontSize: 10,
          },
        },
        series: [
          {
            type: "heatmap",
            coordinateSystem: "calendar",
            data: valores.map((d) => [d.data, d.total_centavos]),
          },
        ],
      };
    },
    [a, mes],
  );

  const semDados = a && !(a.treemap ?? []).length && !a.series.length;
  return (
    <Pagina titulo="Gastos em gráficos" subtitulo="Com o que você gasta mais e quando.">
      <AbasGastos />
      <SeletorMes mes={mes} onChange={setMes} />
      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}
      {dados.isPending ? (
        <Esqueleto className="h-64 w-full" />
      ) : semDados ? (
        <Cartao>
          <SemDados />
        </Cartao>
      ) : a ? (
        <>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Cartao>
              <TituloCartao>Por categoria em {nomeMes(mes).toLowerCase()}</TituloCartao>
              {(a.treemap ?? []).length ? (
                <Grafico
                  montar={treemap}
                  altura="h-72"
                  rotulo={`Gastos por categoria em ${nomeMes(mes)}`}
                  aoClicar={(p) => p.data?.categoria !== undefined && filtroCategoria(mes, p.data.categoria)}
                  tabela={
                    <table className="w-full">
                      <tbody>
                        {(a.treemap ?? []).map((g) => (
                          <tr key={g.id}>
                            <td className="py-1">{g.nome}</td>
                            <td className="valor num py-1 text-right">{formatarMoeda(g.valor_centavos)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  }
                />
              ) : (
                <SemDados />
              )}
            </Cartao>
            <Cartao>
              <TituloCartao>Onde mais gastou</TituloCartao>
              {a.estabelecimentos.length ? (
                <ul className="flex flex-col gap-3" aria-label="Estabelecimentos com mais gasto">
                  {a.estabelecimentos.map((e) => (
                    <li key={e.chave}>
                      <Link
                        to={`/gastos?busca=${encodeURIComponent(e.nome)}`}
                        className="block rounded-lg hover:bg-superficie-2"
                      >
                        <span className="flex items-baseline justify-between gap-3 text-sm">
                          <span className="truncate">
                            {e.nome}
                            <span className="text-texto-2"> · {e.vezes}×</span>
                          </span>
                          <span className="valor num shrink-0">{formatarMoeda(e.total_centavos)}</span>
                        </span>
                        <Barra
                          fracao={e.total_centavos / (a.estabelecimentos[0]?.total_centavos || 1)}
                          cor="var(--saida)"
                          rotulo={e.nome}
                        />
                      </Link>
                    </li>
                  ))}
                </ul>
              ) : (
                <SemDados />
              )}
            </Cartao>
          </div>

          <Cartao>
            <TituloCartao
              acao={
                <fieldset className="flex rounded-xl border border-borda p-0.5">
                  <legend className="sr-only">Período</legend>
                  {periodos.map((p) => (
                    <label
                      key={p.meses}
                      className="flex min-h-9 min-w-11 cursor-pointer items-center justify-center rounded-lg px-2 text-xs font-semibold text-texto-2 has-[:checked]:bg-destaque/15 has-[:checked]:text-primaria has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-primaria"
                    >
                      <input
                        type="radio"
                        name="periodo"
                        className="sr-only"
                        checked={meses === p.meses}
                        onChange={() => setMeses(p.meses)}
                      />
                      {p.nome}
                    </label>
                  ))}
                </fieldset>
              }
            >
              Mês a mês
            </TituloCartao>
            <Grafico
              montar={barras}
              rotulo={`Gastos por categoria nos últimos ${meses} meses`}
              aoClicar={(p) => {
                const s = a.series[p.seriesIndex];
                const m = a.meses[p.dataIndex];
                if (s && m) filtroCategoria(m, s.nome === "Outras" ? undefined : s.categoria_id);
              }}
              tabela={
                <table className="w-full">
                  <thead>
                    <tr className="text-left text-xs text-texto-2">
                      <th className="py-1 font-medium">Mês</th>
                      <th className="py-1 text-right font-medium">Total</th>
                    </tr>
                  </thead>
                  <tbody>
                    {a.meses.map((m, i) => (
                      <tr key={m}>
                        <td className="py-1">{nomeMes(m)}</td>
                        <td className="valor num py-1 text-right">
                          {formatarMoeda(a.series.reduce((s, x) => s + (x.valores_centavos[i] ?? 0), 0))}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              }
            />
            <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-texto-2" aria-label="Legenda">
              {a.series.map((s) => (
                <li key={s.nome} className="flex items-center gap-1.5">
                  <svg width="10" height="10" aria-hidden="true">
                    <rect width="10" height="10" rx="2" fill={s.cor ?? "currentColor"} />
                  </svg>
                  {s.nome}
                </li>
              ))}
            </ul>
          </Cartao>

          <Cartao>
            <TituloCartao>Dia a dia (6 meses)</TituloCartao>
            <Grafico
              montar={calendario}
              altura="h-44"
              rotulo="Calendário do gasto de cada dia; mais escuro, mais gasto"
              aoClicar={(p) => p.value?.[0] && navegar(`/gastos?dia=${p.value[0]}`)}
            />
            <p className="mt-1 text-xs text-texto-2">
              Quanto mais forte a cor, mais gasto no dia. Toque num dia para ver as compras.
            </p>
          </Cartao>
        </>
      ) : null}
    </Pagina>
  );
}

// ---------------------------------------------------------------------------
// Para onde vai o dinheiro (Sankey e cascata)
// ---------------------------------------------------------------------------

export function FluxoDinheiro() {
  const [mes, setMes] = useMes();
  const navegar = useNavigate();
  const dados = useQuery({
    queryKey: ["analise", "fluxo", mes],
    queryFn: () => obter<AnaliseFluxo>(`/api/analise/fluxo?mes=${mes}`),
  });
  const a = dados.data;
  const rotulos = new Map((a?.nos ?? []).map((n) => [n.nome, n.rotulo]));

  const sankey = useCallback(
    (c: CoresTema) => {
      const rotulos = new Map((a?.nos ?? []).map((n) => [n.nome, n.rotulo]));
      const cor = (n: AnaliseFluxo["nos"][number]) =>
        n.tipo === "receita" || n.tipo === "sobra"
          ? c.entrada
          : n.tipo === "deficit"
            ? c.saida
            : n.tipo === "renda"
              ? c.texto2
              : (n.cor ?? c.texto2);
      return {
        tooltip: {
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (p: any) =>
            p.dataType === "edge"
              ? tooltip(`${rotulos.get(p.data.source)} → ${rotulos.get(p.data.target)}`, [
                  { nome: "Valor", valor: moeda(p.data.value, c.privado) },
                ])
              : tooltip(rotulos.get(p.name) ?? p.name, [{ nome: "Total", valor: moeda(p.value, c.privado) }]),
        },
        series: [
          {
            type: "sankey",
            animationDuration: 400,
            left: 4,
            right: 96,
            top: 8,
            bottom: 8,
            nodeGap: 10,
            nodeWidth: 10,
            draggable: false,
            emphasis: { focus: "adjacency" },
            label: {
              color: c.texto,
              fontSize: 11,
              // biome-ignore lint/suspicious/noExplicitAny: parâmetros do rótulo do ECharts
              formatter: (p: any) => rotulos.get(p.name) ?? p.name,
            },
            lineStyle: { color: "gradient", opacity: 0.35, curveness: 0.5 },
            data: (a?.nos ?? []).map((n) => ({ name: n.nome, itemStyle: { color: cor(n), borderWidth: 0 } })),
            links: (a?.ligacoes ?? []).map((l) => ({
              source: l.origem,
              target: l.destino,
              value: l.valor_centavos,
            })),
          },
        ],
      };
    },
    [a],
  );

  const cascata = useCallback(
    (c: CoresTema) => {
      const k = a?.cascata;
      if (!k) return {};
      const meio = k.saldo_inicial_centavos + k.entradas_centavos;
      const flutua = k.saldo_inicial_centavos >= 0 && k.saldo_final_centavos >= 0;
      const nomes = ["Saldo inicial", "Entradas", "Saídas", "Saldo final"];
      const valores = [
        k.saldo_inicial_centavos,
        k.entradas_centavos,
        k.saidas_centavos,
        k.saldo_final_centavos,
      ];
      const cores = [c.texto2, c.entrada, c.saida, c.texto2]; // saldos neutros; entrada e saída com a cor delas
      return {
        grid: { left: 8, right: 8, top: 12, bottom: 8, containLabel: true },
        tooltip: {
          trigger: "axis",
          axisPointer: { type: "shadow" },
          // biome-ignore lint/suspicious/noExplicitAny: parâmetros do tooltip do ECharts
          formatter: (ps: any[]) => {
            const i = ps[0]?.dataIndex ?? 0;
            return tooltip(nomes[i] ?? "", [
              { cor: cores[i], nome: "Valor", valor: moeda(valores[i] ?? 0, c.privado) },
            ]);
          },
        },
        xAxis: {
          type: "category",
          data: nomes,
          axisTick: { show: false },
          axisLine: { lineStyle: { color: c.borda } },
        },
        yAxis: {
          type: "value",
          splitLine: { lineStyle: { color: c.borda } },
          axisLabel: { color: c.texto2, formatter: (v: number) => moedaCurta(v, c.privado) },
        },
        series: [
          {
            type: "bar",
            stack: "cascata",
            silent: true,
            itemStyle: { color: "transparent" },
            data: flutua ? [0, k.saldo_inicial_centavos, meio - k.saidas_centavos, 0] : [0, 0, 0, 0],
          },
          {
            type: "bar",
            stack: "cascata",
            barMaxWidth: 48,
            data: valores.map((v, i) => ({ value: v, itemStyle: { color: cores[i], borderRadius: 4 } })),
          },
        ],
      };
    },
    [a],
  );

  const semDados = a && !a.ligacoes.length;
  return (
    <Pagina
      titulo="Para onde vai o dinheiro"
      subtitulo="Das entradas às categorias, e o saldo do começo ao fim do mês."
    >
      <AbasGastos />
      <SeletorMes mes={mes} onChange={setMes} />
      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}
      {dados.isPending ? (
        <Esqueleto className="h-96 w-full" />
      ) : a ? (
        <>
          <Cartao>
            <TituloCartao>Entradas → categorias</TituloCartao>
            {semDados ? (
              <SemDados />
            ) : (
              <Grafico
                montar={sankey}
                altura="h-[28rem]"
                rotulo={`Fluxo do dinheiro em ${nomeMes(mes)}: das entradas para as categorias de gasto`}
                aoClicar={(p) => {
                  const n = a.nos.find((x) => x.nome === p.name);
                  if (n?.categoria_id) navegar(`/gastos?mes=${mes}&categoria=${n.categoria_id}`);
                }}
                tabela={
                  <table className="w-full">
                    <tbody>
                      {a.ligacoes.map((l) => (
                        <tr key={`${l.origem}>${l.destino}`}>
                          <td className="py-1">
                            {rotulos.get(l.origem)} → {rotulos.get(l.destino)}
                          </td>
                          <td className="valor num py-1 text-right">{formatarMoeda(l.valor_centavos)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                }
              />
            )}
          </Cartao>
          <Cartao>
            <TituloCartao>Saldo do mês nas contas do dia a dia</TituloCartao>
            <Grafico
              montar={cascata}
              rotulo={`Saldo inicial ${formatarMoeda(a.cascata.saldo_inicial_centavos)}, entradas ${formatarMoeda(a.cascata.entradas_centavos)}, saídas ${formatarMoeda(a.cascata.saidas_centavos)}, saldo final ${formatarMoeda(a.cascata.saldo_final_centavos)}`}
            />
            <p className="mt-1 text-xs text-texto-2">
              Inclui transferências entre as suas contas e o pagamento das faturas dos cartões.
            </p>
          </Cartao>
        </>
      ) : null}
    </Pagina>
  );
}
