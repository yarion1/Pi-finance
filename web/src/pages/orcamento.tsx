import { useMutation, useQuery } from "@tanstack/react-query";
import { Copy, PiggyBank, SlidersHorizontal } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { AvisoSimulacao, Barra, Dialogo, SeletorMes } from "../components/extras";
import {
  Aviso,
  Botao,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  Selecao,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import {
  arvoreCategorias,
  gastoOrcado,
  mesAtual,
  somarMes,
  useCategorias,
  useEditaveis,
  useInvalidarPlanejamento,
} from "../lib/dados";
import { centavosParaTexto, formatarMoeda, formatarPercentual, lerCentavos } from "../lib/formato";
import type { ItemOrcamento, Orcamento as TOrcamento } from "../lib/tipos";

const nomesModo: Record<TOrcamento["modo"], string> = {
  categoria: "Limite por categoria",
  envelopes: "Envelopes (a sobra passa para o mês seguinte)",
  "50_30_20": "Regra 50/30/20",
};

const situacoes: Record<string, { nome: string; cor: "entrada" | "alerta" | "imposto"; barra: string }> = {
  ok: { nome: "No ritmo", cor: "entrada", barra: "var(--entrada)" },
  acima_do_ritmo: { nome: "Acima do ritmo", cor: "alerta", barra: "var(--alerta)" },
  estourado: { nome: "Estourou", cor: "imposto", barra: "var(--saida)" },
};

export function Orcamento() {
  const [mes, setMes] = useState(mesAtual);
  const editaveis = useEditaveis();
  const [entidade, setEntidade] = useState("");
  const [editando, setEditando] = useState(false);
  const invalidar = useInvalidarPlanejamento();
  const dados = useQuery({
    queryKey: ["orcamento", mes, entidade],
    queryFn: () =>
      obter<TOrcamento>(`/api/orcamento?mes=${mes}${entidade ? `&entidade_id=${entidade}` : ""}`),
  });
  const o = dados.data;
  const itens = o?.itens ?? [];
  const semOrcamento = o?.sem_orcamento ?? [];

  const modo = useMutation({
    mutationFn: (novo: TOrcamento["modo"]) =>
      api("PUT", "/api/orcamento/config", {
        entidade_id: o?.entidade_id,
        modo: novo,
        grupos: o?.grupos ?? {},
      }),
    onSuccess: invalidar,
  });
  const copiar = useMutation({
    mutationFn: () =>
      api<{ copiados: number }>("POST", "/api/orcamento/copiar", {
        entidade_id: o?.entidade_id,
        de: somarMes(mes, -1),
        para: mes,
      }),
    onSuccess: invalidar,
  });

  const erro = dados.error ?? modo.error ?? copiar.error;
  const t = o?.totais;
  const gasto = gastoOrcado(itens);
  return (
    <Pagina
      titulo="Orçamento"
      subtitulo="Estou gastando no ritmo certo?"
      acao={<SeletorMes mes={mes} onChange={setMes} />}
    >
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      {editaveis.lista.length > 1 ? (
        <Selecao
          rotulo="Entidade"
          value={entidade || o?.entidade_id || ""}
          onChange={(e) => setEntidade(e.target.value)}
        >
          {editaveis.lista.map((e) => (
            <option key={e.id} value={e.id}>
              {e.nome}
            </option>
          ))}
        </Selecao>
      ) : null}

      {dados.isPending ? (
        <Cartao>
          <CarregandoLista />
        </Cartao>
      ) : o ? (
        <>
          <Cartao>
            <div className="flex flex-wrap items-end justify-between gap-3">
              <div>
                <p className="text-sm text-texto-2">Gasto no mês</p>
                <p className="valor num mt-1 text-3xl font-bold tracking-tight">
                  {formatarMoeda(t?.gasto_centavos ?? 0)}
                </p>
                {t && t.disponivel_centavos > 0 ? (
                  <p className="mt-1 text-sm text-texto-2">
                    Nas categorias orçadas: <span className="valor num">{formatarMoeda(gasto)}</span> de{" "}
                    <span className="valor num">{formatarMoeda(t.disponivel_centavos)}</span> · ritmo ideal
                    hoje: <span className="valor num">{formatarMoeda(t.ritmo_ideal_centavos)}</span>
                  </p>
                ) : null}
              </div>
              <p className="text-sm text-texto-2">
                {o.dia === 0 ? "Mês ainda não começou" : `Dia ${o.dia} de ${o.dias_no_mes}`}
              </p>
            </div>
            {t && t.disponivel_centavos > 0 ? (
              <Barra
                fracao={gasto / t.disponivel_centavos}
                marca={o.dia / o.dias_no_mes}
                cor={
                  gasto > t.disponivel_centavos
                    ? "var(--saida)"
                    : gasto > t.ritmo_ideal_centavos
                      ? "var(--alerta)"
                      : "var(--entrada)"
                }
                rotulo="Gasto nas categorias orçadas em relação ao orçado"
              />
            ) : null}
            <div className="mt-4 flex flex-wrap gap-2">
              <Botao variante="secundario" onClick={() => setEditando(true)}>
                <SlidersHorizontal className="size-4" aria-hidden /> Definir limites
              </Botao>
              <Botao variante="fantasma" carregando={copiar.isPending} onClick={() => copiar.mutate()}>
                <Copy className="size-4" aria-hidden /> Copiar do mês anterior
              </Botao>
            </div>
            {copiar.data ? (
              <p className="mt-2 text-sm text-texto-2" role="status">
                {copiar.data.copiados === 0
                  ? "Nada novo para copiar."
                  : `${copiar.data.copiados} ${copiar.data.copiados === 1 ? "limite copiado" : "limites copiados"}.`}
              </p>
            ) : null}
          </Cartao>

          <Selecao
            rotulo="Como orçar"
            value={o.modo}
            onChange={(e) => modo.mutate(e.target.value as TOrcamento["modo"])}
          >
            {Object.entries(nomesModo).map(([v, n]) => (
              <option key={v} value={v}>
                {n}
              </option>
            ))}
          </Selecao>

          {o.modo === "50_30_20" ? <Regra503020 o={o} /> : null}

          <Cartao>
            <TituloCartao>Por categoria</TituloCartao>
            {itens.length ? (
              <ul className="flex flex-col gap-4" aria-label="Limites por categoria">
                {itens
                  .slice()
                  .sort(
                    (a, b) =>
                      b.gasto_centavos / (b.disponivel_centavos || 1) -
                      a.gasto_centavos / (a.disponivel_centavos || 1),
                  )
                  .map((i) => (
                    <LinhaOrcamento key={i.categoria_id} item={i} o={o} />
                  ))}
              </ul>
            ) : (
              <Vazio
                icone={<PiggyBank className="size-8" />}
                titulo="Nenhum limite neste mês"
                texto="Defina quanto quer gastar em cada categoria. A barra mostra o ritmo ideal para o dia de hoje."
                acao={<Botao onClick={() => setEditando(true)}>Definir limites</Botao>}
              />
            )}
          </Cartao>

          {semOrcamento.length ? (
            <Cartao>
              <TituloCartao>Gastos sem limite</TituloCartao>
              <ul className="flex flex-col divide-y divide-borda">
                {semOrcamento.map((i) => (
                  <li key={i.categoria_id} className="flex min-h-12 items-center gap-3 py-1.5 text-sm">
                    <Link
                      to={`/gastos?mes=${mes}&categoria=${i.categoria_id}`}
                      className="min-w-0 flex-1 truncate hover:underline"
                    >
                      {i.nome}
                    </Link>
                    <span className="text-right">
                      <span className="valor num block">{formatarMoeda(i.gasto_centavos)}</span>
                      {i.media_3_meses_centavos > 0 ? (
                        <span className="block text-xs text-texto-2">
                          média 3 meses:{" "}
                          <span className="valor num">{formatarMoeda(i.media_3_meses_centavos)}</span>
                        </span>
                      ) : null}
                    </span>
                  </li>
                ))}
              </ul>
            </Cartao>
          ) : null}
        </>
      ) : null}

      <Dialogo
        aberto={editando}
        aoFechar={() => setEditando(false)}
        titulo={`Limites de ${mes.split("-").reverse().join("/")}`}
      >
        {o ? <FormLimites o={o} aoFechar={() => setEditando(false)} /> : null}
      </Dialogo>
      <AvisoSimulacao />
    </Pagina>
  );
}

function LinhaOrcamento({ item: i, o }: { item: ItemOrcamento; o: TOrcamento }) {
  const s = situacoes[i.situacao] ?? situacoes.ok;
  const restante = i.disponivel_centavos - i.gasto_centavos;
  return (
    <li>
      <div className="flex items-baseline justify-between gap-3 text-sm">
        <span className="min-w-0 truncate font-medium">
          {i.pai ? <span className="text-texto-2">{i.pai} › </span> : null}
          {i.nome}
        </span>
        <span className="valor num shrink-0">
          {formatarMoeda(i.gasto_centavos)}
          <span className="text-texto-2"> / {formatarMoeda(i.disponivel_centavos)}</span>
        </span>
      </div>
      <Barra
        fracao={i.disponivel_centavos > 0 ? i.gasto_centavos / i.disponivel_centavos : 1}
        marca={o.dia / o.dias_no_mes}
        cor={s?.barra}
        rotulo={`${i.nome}: gasto em relação ao limite`}
      />
      <div className="mt-1.5 flex flex-wrap items-center justify-between gap-2 text-xs text-texto-2">
        <span>
          {restante >= 0 ? (
            <>
              Resta <span className="valor num">{formatarMoeda(restante)}</span>
            </>
          ) : (
            <>
              Passou <span className="valor num text-saida">{formatarMoeda(-restante)}</span>
            </>
          )}
          {o.modo === "envelopes" && i.sobra_anterior_centavos !== 0 ? (
            <>
              {" "}
              · sobra anterior <span className="valor num">{formatarMoeda(i.sobra_anterior_centavos)}</span>
            </>
          ) : null}
        </span>
        {s ? <Etiqueta cor={s.cor}>{s.nome}</Etiqueta> : null}
      </div>
    </li>
  );
}

function Regra503020({ o }: { o: TOrcamento }) {
  const r = o.regra_50_30_20;
  if (!r) return null;
  const linhas = [
    { nome: "Necessidades", meta: "50 %", g: r.necessidades, maxima: true },
    { nome: "Desejos", meta: "30 %", g: r.desejos, maxima: true },
    { nome: "Poupança", meta: "20 %", g: r.poupanca, maxima: false },
  ];
  return (
    <Cartao>
      <TituloCartao>Regra 50/30/20</TituloCartao>
      {r.renda_centavos <= 0 ? (
        <Aviso>Sem receitas neste mês: a regra divide a renda do mês.</Aviso>
      ) : (
        <>
          <p className="mb-3 text-sm text-texto-2">
            Renda do mês: <span className="valor num">{formatarMoeda(r.renda_centavos)}</span>
          </p>
          <ul className="flex flex-col gap-4">
            {linhas.map((l) => {
              const ruim = l.maxima
                ? l.g.valor_centavos > l.g.alvo_centavos
                : l.g.valor_centavos < l.g.alvo_centavos;
              return (
                <li key={l.nome}>
                  <div className="flex items-baseline justify-between gap-3 text-sm">
                    <span className="font-medium">
                      {l.nome} <span className="text-texto-2">({l.meta})</span>
                    </span>
                    <span className="valor num">
                      {formatarMoeda(l.g.valor_centavos)}
                      <span className="text-texto-2"> / {formatarMoeda(l.g.alvo_centavos)}</span>
                    </span>
                  </div>
                  <Barra
                    fracao={l.g.alvo_centavos > 0 ? l.g.valor_centavos / l.g.alvo_centavos : 0}
                    cor={ruim ? "var(--alerta)" : "var(--entrada)"}
                    rotulo={l.nome}
                  />
                  <p className="mt-1 text-xs text-texto-2">
                    {formatarPercentual(r.renda_centavos > 0 ? l.g.valor_centavos / r.renda_centavos : 0, 0)}{" "}
                    da renda
                  </p>
                </li>
              );
            })}
          </ul>
        </>
      )}
      <GruposRegra o={o} />
    </Cartao>
  );
}

/** Qual categoria mãe é necessidade e qual é desejo. */
function GruposRegra({ o }: { o: TOrcamento }) {
  const categorias = useCategorias();
  const invalidar = useInvalidarPlanejamento();
  const maes = arvoreCategorias(categorias.data ?? [], o.entidade_id).filter((c) => c.tipo === "gasto");
  const salvar = useMutation({
    mutationFn: (grupos: Record<string, string>) =>
      api("PUT", "/api/orcamento/config", { entidade_id: o.entidade_id, modo: o.modo, grupos }),
    onSuccess: invalidar,
  });
  return (
    <details className="mt-4">
      <summary className="min-h-11 cursor-pointer py-2 text-sm font-semibold text-primaria">
        Ajustar o que é necessidade e desejo
      </summary>
      {salvar.error ? <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso> : null}
      <ul className="flex flex-col divide-y divide-borda">
        {maes.map((m) => (
          <li key={m.id} className="flex min-h-12 items-center justify-between gap-3 text-sm">
            <span className="truncate">{m.nome}</span>
            <select
              aria-label={`Grupo de ${m.nome}`}
              value={o.grupos?.[m.id] ?? ""}
              onChange={(e) => {
                const g = { ...(o.grupos ?? {}) } as Record<string, string>;
                if (e.target.value) g[m.id] = e.target.value;
                else delete g[m.id];
                salvar.mutate(g);
              }}
              className="min-h-11 rounded-xl border border-borda bg-superficie-2 px-2 text-sm"
            >
              <option value="">Automático</option>
              <option value="necessidades">Necessidade</option>
              <option value="desejos">Desejo</option>
            </select>
          </li>
        ))}
      </ul>
    </details>
  );
}

function FormLimites({ o, aoFechar }: { o: TOrcamento; aoFechar: () => void }) {
  const categorias = useCategorias();
  const invalidar = useInvalidarPlanejamento();
  const arvore = arvoreCategorias(categorias.data ?? [], o.entidade_id).filter((c) => c.tipo === "gasto");
  const medias = new Map<string, number>();
  for (const i of [...(o.itens ?? []), ...(o.sem_orcamento ?? [])])
    medias.set(i.categoria_id, i.media_3_meses_centavos);
  const [valores, setValores] = useState<Record<string, string>>(() =>
    Object.fromEntries((o.itens ?? []).map((i) => [i.categoria_id, centavosParaTexto(i.limite_centavos)])),
  );
  const salvar = useMutation({
    mutationFn: () => {
      const itens: { categoria_id: string; limite_centavos: number }[] = [];
      for (const [id, texto] of Object.entries(valores)) {
        if (!texto.trim()) continue;
        const c = lerCentavos(texto);
        if (c === null || c < 0) throw new Error("Limite inválido. Use o formato 1.234,56.");
        itens.push({ categoria_id: id, limite_centavos: c });
      }
      return api("PUT", "/api/orcamento", { entidade_id: o.entidade_id, mes: o.mes, itens });
    },
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const campo = (id: string, nome: string, filha: boolean) => {
    const media = medias.get(id);
    return (
      <li key={id} className={`flex min-h-12 items-center justify-between gap-3 ${filha ? "pl-4" : ""}`}>
        <label
          htmlFor={`lim-${id}`}
          className={`min-w-0 flex-1 truncate text-sm ${filha ? "text-texto-2" : "font-medium"}`}
        >
          {nome}
        </label>
        <input
          id={`lim-${id}`}
          inputMode="decimal"
          value={valores[id] ?? ""}
          placeholder={media ? centavosParaTexto(media) : "—"}
          onChange={(e) => setValores((v) => ({ ...v, [id]: e.target.value }))}
          className="min-h-11 w-32 rounded-xl border border-borda bg-superficie-2 px-3 text-right text-base"
        />
      </li>
    );
  };
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        salvar.mutate();
      }}
      className="flex flex-col gap-3"
    >
      <p className="text-sm text-texto-2">
        Deixe em branco para não orçar. A sugestão é a média dos últimos 3 meses.
      </p>
      {salvar.error ? <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso> : null}
      <ul className="flex flex-col divide-y divide-borda">
        {arvore.flatMap((m) => [
          campo(m.id, m.nome, false),
          ...m.filhas.map((f) => campo(f.id, f.nome, true)),
        ])}
      </ul>
      <div className="flex justify-end gap-2">
        <Botao variante="fantasma" onClick={aoFechar}>
          Cancelar
        </Botao>
        <Botao type="submit" carregando={salvar.isPending}>
          Salvar limites
        </Botao>
      </div>
    </form>
  );
}
