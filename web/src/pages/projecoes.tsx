import { useMutation, useQuery } from "@tanstack/react-query";
import { type FormEvent, type ReactNode, useState } from "react";
import { AvisoSimulacao } from "../components/extras";
import {
  Aviso,
  Botao,
  Campo,
  Cartao,
  Esqueleto,
  Etiqueta,
  Pagina,
  Selecao,
  TituloCartao,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import {
  decimalBR,
  formatarData,
  formatarMoeda,
  formatarPercentual,
  hojeSP,
  lerCentavos,
} from "../lib/formato";
import type {
  FluxoDia,
  FluxoProjetado,
  Futuro,
  RespostaPatrimonio,
  SimulacaoCompra,
  SimulacaoFinanciamento,
  SimulacaoQuitacao,
} from "../lib/tipos";

const nomesClasse: Record<string, string> = {
  caixa: "Contas",
  renda_fixa: "Renda fixa",
  tesouro: "Tesouro Direto",
  fundo: "Fundos",
  previdencia: "Previdência",
  acao: "Ações",
  etf: "ETFs",
  bdr: "BDRs",
  exterior: "Exterior",
  fii: "FIIs",
  cripto: "Cripto",
  outro: "Outros",
};

/** "3 anos e 4 meses". */
function tempo(meses: number): string {
  if (meses < 0) return "não chega em 100 anos";
  if (meses === 0) return "já chegou";
  const a = Math.floor(meses / 12);
  const m = meses % 12;
  const partes = [a ? `${a} ${a === 1 ? "ano" : "anos"}` : "", m ? `${m} ${m === 1 ? "mês" : "meses"}` : ""];
  return `em ${partes.filter(Boolean).join(" e ")}`;
}

export function Projecoes() {
  return (
    <Pagina
      titulo="Projeções e simulações"
      subtitulo="Para onde vai o saldo, o patrimônio e as decisões grandes."
    >
      <FluxoCaixa />
      <PatrimonioFuturo />
      <Simulacoes />
      <AvisoSimulacao />
    </Pagina>
  );
}

// ---------------------------------------------------------------------------
// Fluxo de caixa
// ---------------------------------------------------------------------------

function FluxoCaixa() {
  const [dias, setDias] = useState(90);
  const fluxo = useQuery({
    queryKey: ["projecoes", "fluxo", dias],
    queryFn: () => obter<FluxoProjetado>(`/api/projecoes/fluxo?dias=${dias}`),
  });
  const f = fluxo.data;
  return (
    <Cartao>
      <TituloCartao
        acao={
          <Selecao rotulo="Horizonte" value={dias} onChange={(e) => setDias(Number(e.target.value))}>
            <option value={30}>30 dias</option>
            <option value={90}>90 dias</option>
            <option value={180}>6 meses</option>
          </Selecao>
        }
      >
        Saldo projetado
      </TituloCartao>
      {fluxo.error ? <Aviso tipo="erro">{mensagemDe(fluxo.error)}</Aviso> : null}
      {fluxo.isPending ? (
        <Esqueleto className="h-40 w-full" />
      ) : f ? (
        <>
          <GraficoFluxo pontos={f.pontos ?? []} />
          <dl className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Numero rotulo="Hoje" valor={f.saldo_inicial_centavos} />
            <Numero rotulo={`Em ${dias} dias`} valor={f.final.base_centavos}>
              entre {formatarMoeda(f.final.baixo_centavos)} e {formatarMoeda(f.final.alto_centavos)}
            </Numero>
            <Numero rotulo="Menor saldo" valor={f.menor.base_centavos}>
              em {formatarData(f.menor.data.slice(0, 10))}
            </Numero>
            <Numero rotulo="Gasto variável no mês" valor={-f.variavel_mensal_centavos} />
          </dl>
          {f.menor_baixo.baixo_centavos < 0 ? (
            <div className="mt-3">
              <Aviso tipo={f.menor.base_centavos < 0 ? "erro" : "alerta"}>
                {f.menor.base_centavos < 0
                  ? `O saldo previsto fica negativo em ${formatarData(f.menor.data.slice(0, 10))}.`
                  : "No cenário ruim da faixa o saldo fica negativo em algum dia: vale ter uma folga."}
              </Aviso>
            </div>
          ) : null}
          <p className="mt-3 text-xs text-texto-2">
            Contas do dia a dia: {f.itens_conhecidos} contas e recebimentos já conhecidos (agenda) mais o
            gasto variável médio, com o mesmo mês do ano passado pesando mais. A faixa cobre 80 % dos casos.
          </p>
        </>
      ) : null}
    </Cartao>
  );
}

function Numero({ rotulo, valor, children }: { rotulo: string; valor: number; children?: ReactNode }) {
  return (
    <div>
      <dt className="text-xs text-texto-2">{rotulo}</dt>
      <dd className={`valor num text-sm font-semibold ${valor < 0 ? "text-saida" : ""}`}>
        {formatarMoeda(valor)}
      </dd>
      {children ? <dd className="text-xs text-texto-2">{children}</dd> : null}
    </div>
  );
}

/** Linha do saldo base com a faixa de confiança (SVG simples; ECharts entra na fase 7). */
function GraficoFluxo({ pontos }: { pontos: FluxoDia[] }) {
  if (pontos.length < 2) return null;
  const valores = pontos.flatMap((p) => [p.baixo_centavos, p.alto_centavos]);
  const min = Math.min(0, ...valores);
  const max = Math.max(1, ...valores);
  const x = (i: number) => (i / (pontos.length - 1)) * 300;
  const y = (v: number) => 100 - ((v - min) / (max - min)) * 100;
  const base = pontos.map((p, i) => `${x(i)},${y(p.base_centavos)}`).join(" ");
  const faixa = [
    ...pontos.map((p, i) => `${x(i)},${y(p.alto_centavos)}`),
    ...pontos.map((p, i) => `${x(i)},${y(p.baixo_centavos)}`).reverse(),
  ].join(" ");
  const final = pontos[pontos.length - 1];
  return (
    <svg
      viewBox="0 0 300 100"
      preserveAspectRatio="none"
      className="h-40 w-full"
      role="img"
      aria-label={`Saldo projetado: de ${formatarMoeda(pontos[0]?.base_centavos ?? 0)} hoje a ${formatarMoeda(final?.base_centavos ?? 0)} em ${formatarData(final?.data.slice(0, 10) ?? "")}`}
    >
      <polygon points={faixa} className="fill-primaria/15" />
      {min < 0 ? (
        <line
          x1="0"
          x2="300"
          y1={y(0)}
          y2={y(0)}
          className="stroke-saida"
          strokeWidth="0.6"
          strokeDasharray="3 2"
        />
      ) : null}
      <polyline
        points={base}
        fill="none"
        className="stroke-primaria"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// Patrimônio no futuro (Monte Carlo) e independência financeira
// ---------------------------------------------------------------------------

type ParametrosFuturo = {
  aporte: string;
  anos: string;
  custo: string;
  perfil: string;
  premissas: Record<string, string>;
};

function PatrimonioFuturo() {
  const [form, setForm] = useState<ParametrosFuturo>({
    aporte: "",
    anos: "10",
    custo: "",
    perfil: "atual",
    premissas: {},
  });
  const [consulta, setConsulta] = useState("anos=10");
  const [erro, setErro] = useState("");
  const futuro = useQuery({
    queryKey: ["projecoes", "futuro", consulta],
    queryFn: () => obter<Futuro>(`/api/projecoes/futuro?${consulta}`),
  });

  function simular(e: FormEvent) {
    e.preventDefault();
    const q = new URLSearchParams({ anos: form.anos, perfil: form.perfil });
    for (const [campo, texto] of [
      ["aporte", form.aporte],
      ["custo", form.custo],
    ] as const) {
      if (!texto.trim()) continue;
      const c = lerCentavos(texto);
      if (c === null || c < 0) return setErro("Valores no formato 1.234,56.");
      q.set(campo, String(c));
    }
    for (const [chave, texto] of Object.entries(form.premissas)) {
      if (!texto.trim()) continue;
      const v = decimalBR(texto.replace(/^-/, ""));
      if (v === null) return setErro("Percentuais no formato 7,5.");
      q.set(chave, (texto.trim().startsWith("-") ? "-" : "") + v);
    }
    setErro("");
    setConsulta(q.toString());
  }

  const f = futuro.data;
  const anos = f?.pontos?.length ?? 0;
  const linhas = (f?.pontos ?? []).filter((p) => [1, 5, 10, 20, 30, anos].includes(p.ano));
  return (
    <Cartao>
      <TituloCartao>Patrimônio no futuro</TituloCartao>
      <form onSubmit={simular} className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Campo
          rotulo="Aporte por mês (R$)"
          inputMode="decimal"
          placeholder="0,00"
          value={form.aporte}
          onChange={(e) => setForm({ ...form, aporte: e.target.value })}
        />
        <Campo
          rotulo="Anos"
          inputMode="numeric"
          value={form.anos}
          onChange={(e) => setForm({ ...form, anos: e.target.value })}
        />
        <Campo
          rotulo="Custo de vida por mês (R$)"
          inputMode="decimal"
          placeholder="o médio de 6 meses"
          value={form.custo}
          onChange={(e) => setForm({ ...form, custo: e.target.value })}
        />
        <Selecao
          rotulo="Aportes vão para"
          value={form.perfil}
          onChange={(e) => setForm({ ...form, perfil: e.target.value })}
        >
          <option value="atual">como a carteira está</option>
          <option value="conservador">perfil conservador</option>
          <option value="moderado">perfil moderado</option>
          <option value="arrojado">perfil arrojado</option>
        </Selecao>
        {f?.classes?.length ? (
          <details className="col-span-2 sm:col-span-4">
            <summary className="min-h-11 cursor-pointer py-2 text-sm font-semibold text-primaria">
              Premissas por classe (retorno real e volatilidade ao ano)
            </summary>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              {f.classes.map((c) => (
                <fieldset key={c.classe} className="col-span-2 grid grid-cols-2 gap-2">
                  <legend className="mb-1 text-xs font-semibold text-texto-2">
                    {nomesClasse[c.classe] ?? c.classe}
                  </legend>
                  <Campo
                    rotulo="Retorno (%)"
                    inputMode="decimal"
                    placeholder={formatarPercentual(c.retorno_aa).replace("%", "").trim()}
                    value={form.premissas[`r_${c.classe}`] ?? ""}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        premissas: { ...form.premissas, [`r_${c.classe}`]: e.target.value },
                      })
                    }
                  />
                  <Campo
                    rotulo="Volatilidade (%)"
                    inputMode="decimal"
                    placeholder={formatarPercentual(c.volatilidade_aa).replace("%", "").trim()}
                    value={form.premissas[`v_${c.classe}`] ?? ""}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        premissas: { ...form.premissas, [`v_${c.classe}`]: e.target.value },
                      })
                    }
                  />
                </fieldset>
              ))}
            </div>
          </details>
        ) : null}
        <div className="col-span-2 sm:col-span-4">
          <Botao type="submit" carregando={futuro.isFetching}>
            Simular
          </Botao>
        </div>
      </form>
      {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
      {futuro.error ? <Aviso tipo="erro">{mensagemDe(futuro.error)}</Aviso> : null}
      {futuro.isPending ? (
        <Esqueleto className="mt-4 h-32 w-full" />
      ) : f ? (
        <div className="mt-4 flex flex-col gap-4">
          <p className="text-sm text-texto-2">
            Hoje investível:{" "}
            <span className="valor num font-semibold text-texto">{formatarMoeda(f.investivel_centavos)}</span>{" "}
            (carteira e contas; imóveis e carros ficam de fora). Retorno real médio esperado:{" "}
            {formatarPercentual(f.retorno_medio_aa)} ao ano. {f.simulacoes.toLocaleString("pt-BR")} cenários,
            em dinheiro de hoje.
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <caption className="sr-only">Patrimônio investível por ano, em dinheiro de hoje</caption>
              <thead>
                <tr className="text-left text-xs text-texto-2">
                  <th className="py-1 font-medium">Ano</th>
                  <th className="py-1 text-right font-medium">Ruim (10 %)</th>
                  <th className="py-1 text-right font-medium">Provável</th>
                  <th className="py-1 text-right font-medium">Bom (90 %)</th>
                  <th className="py-1 text-right font-medium">Independência</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-borda">
                {linhas.map((p) => (
                  <tr key={p.ano}>
                    <td className="py-2">{p.ano}</td>
                    <td className="valor num py-2 text-right">{formatarMoeda(p.p10_centavos)}</td>
                    <td className="valor num py-2 text-right font-semibold">
                      {formatarMoeda(p.p50_centavos)}
                    </td>
                    <td className="valor num py-2 text-right">{formatarMoeda(p.p90_centavos)}</td>
                    <td className="py-2 text-right">{formatarPercentual(p.chance_alvo, 0)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <section>
            <h3 className="text-sm font-semibold">Independência financeira</h3>
            <p className="mt-1 text-sm text-texto-2">
              Com custo de vida de {formatarMoeda(f.custo_mensal_centavos)} por mês e retirada de{" "}
              {formatarPercentual(f.taxa_retirada)} ao ano, o alvo é{" "}
              <span className="valor num font-semibold text-texto">{formatarMoeda(f.alvo_centavos)}</span>.
            </p>
            <ul className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-3" aria-label="Cenários de independência">
              {f.cenarios.map((c) => (
                <li key={c.nome} className="rounded-xl border border-borda p-3">
                  <p className="text-xs text-texto-2">
                    {c.nome === "base" ? "Base" : c.nome === "pessimista" ? "Pessimista" : "Otimista"} ·{" "}
                    {formatarPercentual(c.retorno_aa)} a.a.
                  </p>
                  <p className="text-sm font-semibold">{tempo(c.meses)}</p>
                </li>
              ))}
            </ul>
          </section>
        </div>
      ) : null}
    </Cartao>
  );
}

// ---------------------------------------------------------------------------
// Simulações
// ---------------------------------------------------------------------------

type Aba = "compra" | "financiamento" | "quitar";

function Simulacoes() {
  const [aba, setAba] = useState<Aba>("compra");
  const abas: [Aba, string][] = [
    ["compra", "Compra parcelada"],
    ["financiamento", "Parcelar ou à vista"],
    ["quitar", "Quitar dívida antes"],
  ];
  return (
    <Cartao>
      <TituloCartao>Simular uma decisão</TituloCartao>
      <div className="mb-3 flex gap-2 overflow-x-auto pb-1" role="tablist" aria-label="Simulações">
        {abas.map(([id, nome]) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={aba === id}
            onClick={() => setAba(id)}
            className="min-h-11 shrink-0 rounded-xl border border-borda px-4 text-sm font-semibold text-texto-2 aria-selected:border-primaria aria-selected:text-texto"
          >
            {nome}
          </button>
        ))}
      </div>
      {aba === "compra" ? (
        <SimularCompra />
      ) : aba === "financiamento" ? (
        <SimularFinanciamento />
      ) : (
        <SimularQuitacao />
      )}
    </Cartao>
  );
}

const OPCOES_PARCELAS = Array.from({ length: 24 }, (_, i) => i + 1);

const textoSituacao = {
  folgada: { cor: "entrada", texto: "Cabe com folga" },
  apertada: { cor: "alerta", texto: "Cabe, mas apertado" },
  nao_cabe: { cor: "alerta", texto: "Não cabe: o saldo fica negativo" },
} as const;

function SimularCompra() {
  const daquiUmMes = new Date(Date.parse(`${hojeSP()}T12:00:00Z`) + 30 * 86_400_000)
    .toISOString()
    .slice(0, 10);
  const [d, setD] = useState({ valor: "", parcelas: "1", primeira: daquiUmMes });
  const simular = useMutation({
    mutationFn: () => {
      const valor = lerCentavos(d.valor);
      if (valor === null || valor <= 0) throw new Error("Valor inválido. Use o formato 1.234,56.");
      return api<SimulacaoCompra>("POST", "/api/simulacoes/compra", {
        valor_centavos: valor,
        parcelas: Number(d.parcelas),
        primeira: d.primeira,
      });
    },
  });
  const r = simular.data;
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        simular.mutate();
      }}
      className="flex flex-col gap-3"
    >
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <Campo
          rotulo="Valor total (R$)"
          inputMode="decimal"
          required
          value={d.valor}
          onChange={(e) => setD({ ...d, valor: e.target.value })}
        />
        <Selecao
          rotulo="Parcelas"
          value={d.parcelas}
          onChange={(e) => setD({ ...d, parcelas: e.target.value })}
        >
          {OPCOES_PARCELAS.map((n) => (
            <option key={n} value={n}>
              {n}×
            </option>
          ))}
        </Selecao>
        <Campo
          rotulo="Primeira parcela"
          type="date"
          required
          value={d.primeira}
          onChange={(e) => setD({ ...d, primeira: e.target.value })}
        />
      </div>
      <div>
        <Botao type="submit" carregando={simular.isPending}>
          Simular compra
        </Botao>
      </div>
      {simular.error ? <Aviso tipo="erro">{mensagemDe(simular.error)}</Aviso> : null}
      {r ? (
        <div className="flex flex-col gap-2" aria-live="polite">
          <Etiqueta cor={textoSituacao[r.situacao].cor}>{textoSituacao[r.situacao].texto}</Etiqueta>
          <p className="text-sm">
            {d.parcelas}× de{" "}
            <span className="valor num font-semibold">{formatarMoeda(r.parcela_centavos)}</span>, de{" "}
            {formatarData(r.primeira_data)} a {formatarData(r.ultima_data)}.
          </p>
          <dl className="grid grid-cols-2 gap-3">
            <Numero rotulo="Menor saldo sem a compra" valor={r.sem.menor.base_centavos}>
              em {formatarData(r.sem.menor.data.slice(0, 10))}
            </Numero>
            <Numero rotulo="Menor saldo com a compra" valor={r.com.menor.base_centavos}>
              em {formatarData(r.com.menor.data.slice(0, 10))}
            </Numero>
          </dl>
        </div>
      ) : null}
    </form>
  );
}

function SimularFinanciamento() {
  const [d, setD] = useState({ avista: "", parcela: "", parcelas: "10", primeira: "1", rendimento: "" });
  const simular = useMutation({
    mutationFn: () => {
      const avista = lerCentavos(d.avista);
      const parcela = lerCentavos(d.parcela);
      if (avista === null || parcela === null || avista <= 0 || parcela <= 0)
        throw new Error("Valores inválidos. Use o formato 1.234,56.");
      const corpo: Record<string, unknown> = {
        avista_centavos: avista,
        parcela_centavos: parcela,
        parcelas: Number(d.parcelas),
        primeira_em_meses: Number(d.primeira),
      };
      if (d.rendimento.trim()) {
        const r = decimalBR(d.rendimento);
        if (r === null) throw new Error("Rendimento no formato 0,9.");
        corpo.rendimento_mes = Number(r) / 100;
      }
      return api<SimulacaoFinanciamento>("POST", "/api/simulacoes/financiamento", corpo);
    },
  });
  const r = simular.data;
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        simular.mutate();
      }}
      className="flex flex-col gap-3"
    >
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <Campo
          rotulo="Preço à vista (R$)"
          inputMode="decimal"
          required
          value={d.avista}
          onChange={(e) => setD({ ...d, avista: e.target.value })}
        />
        <Campo
          rotulo="Valor da parcela (R$)"
          inputMode="decimal"
          required
          value={d.parcela}
          onChange={(e) => setD({ ...d, parcela: e.target.value })}
        />
        <Campo
          rotulo="Parcelas"
          inputMode="numeric"
          required
          value={d.parcelas}
          onChange={(e) => setD({ ...d, parcelas: e.target.value })}
        />
        <Selecao
          rotulo="Primeira parcela"
          value={d.primeira}
          onChange={(e) => setD({ ...d, primeira: e.target.value })}
        >
          <option value="0">hoje (entrada)</option>
          <option value="1">daqui a 1 mês</option>
          <option value="2">daqui a 2 meses</option>
        </Selecao>
        <Campo
          rotulo="Rendimento ao mês (%)"
          inputMode="decimal"
          placeholder="o CDI"
          value={d.rendimento}
          onChange={(e) => setD({ ...d, rendimento: e.target.value })}
        />
      </div>
      <div>
        <Botao type="submit" carregando={simular.isPending}>
          Comparar
        </Botao>
      </div>
      {simular.error ? <Aviso tipo="erro">{mensagemDe(simular.error)}</Aviso> : null}
      {r ? (
        <div className="flex flex-col gap-2" aria-live="polite">
          <Etiqueta cor={r.compensa_parcelar ? "entrada" : "alerta"}>
            {r.compensa_parcelar ? "Parcelar compensa" : "À vista compensa"}
          </Etiqueta>
          <p className="text-sm">
            Parcelado você paga {formatarMoeda(r.total_parcelado_centavos)}. Deixando o dinheiro rendendo{" "}
            {formatarPercentual(r.rendimento_mes, 2)} ao mês{r.do_cdi ? " (CDI)" : ""}, as parcelas valem hoje{" "}
            <span className="valor num font-semibold">
              {formatarMoeda(r.valor_presente_parcelas_centavos)}
            </span>
            : {formatarMoeda(Math.abs(r.diferenca_centavos))} {r.compensa_parcelar ? "a menos" : "a mais"} que
            à vista.
          </p>
          <p className="text-xs text-texto-2">
            Juros embutidos no parcelamento: {formatarPercentual(r.juros_implicitos_mes, 2)} ao mês. O
            rendimento é bruto (sem o IR da aplicação).
          </p>
        </div>
      ) : null}
    </form>
  );
}

function SimularQuitacao() {
  const patrimonio = useQuery({
    queryKey: ["patrimonio"],
    queryFn: () => obter<RespostaPatrimonio>("/api/patrimonio"),
  });
  const dividas = (patrimonio.data?.dividas ?? []).filter((x) => x.parcelas_restantes > 0);
  const [d, setD] = useState({ divida: "", valor: "", modo: "prazo" });
  const escolhida = d.divida || dividas[0]?.id || "";
  const simular = useMutation({
    mutationFn: () => {
      const valor = lerCentavos(d.valor);
      if (valor === null || valor <= 0) throw new Error("Valor inválido. Use o formato 1.234,56.");
      return api<SimulacaoQuitacao>("POST", "/api/simulacoes/quitar", {
        divida_id: escolhida,
        valor_centavos: valor,
        modo: d.modo,
      });
    },
  });
  const r = simular.data;
  if (patrimonio.isPending) return <Esqueleto className="h-24 w-full" />;
  if (!dividas.length)
    return (
      <p className="text-sm text-texto-2">
        Cadastre um financiamento ou empréstimo em Patrimônio para simular.
      </p>
    );
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        simular.mutate();
      }}
      className="flex flex-col gap-3"
    >
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <Selecao rotulo="Dívida" value={escolhida} onChange={(e) => setD({ ...d, divida: e.target.value })}>
          {dividas.map((x) => (
            <option key={x.id} value={x.id}>
              {x.nome} ({formatarMoeda(x.saldo_devedor_centavos)})
            </option>
          ))}
        </Selecao>
        <Campo
          rotulo="Pagar a mais agora (R$)"
          inputMode="decimal"
          required
          value={d.valor}
          onChange={(e) => setD({ ...d, valor: e.target.value })}
        />
        <Selecao rotulo="Para" value={d.modo} onChange={(e) => setD({ ...d, modo: e.target.value })}>
          <option value="prazo">diminuir o prazo</option>
          <option value="parcela">diminuir a parcela</option>
        </Selecao>
      </div>
      <div>
        <Botao type="submit" carregando={simular.isPending}>
          Simular
        </Botao>
      </div>
      {simular.error ? <Aviso tipo="erro">{mensagemDe(simular.error)}</Aviso> : null}
      {r ? (
        <div className="flex flex-col gap-2" aria-live="polite">
          <Etiqueta cor="entrada">Economia de {formatarMoeda(r.economia_centavos)} em juros</Etiqueta>
          <p className="text-sm">
            {r.quitada
              ? "Esse valor quita a dívida."
              : d.modo === "prazo"
                ? `Faltariam ${r.novas_parcelas} parcelas em vez de ${r.parcelas_restantes}, com a mesma prestação de ${formatarMoeda(r.nova_prestacao_centavos)}.`
                : `A prestação cairia de ${formatarMoeda(r.prestacao_atual_centavos)} para ${formatarMoeda(r.nova_prestacao_centavos)}, nas mesmas ${r.novas_parcelas} parcelas.`}
          </p>
        </div>
      ) : null}
    </form>
  );
}
