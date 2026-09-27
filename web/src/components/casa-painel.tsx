import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { HandCoins, Plus, Target, Trash2 } from "lucide-react";
import { useState } from "react";
import { api, mensagemDe, obter } from "../lib/api";
import { formatarData, formatarMoeda, formatarPercentual, lerCentavos } from "../lib/formato";
import { paleta } from "../lib/graficos";
import { useSessao } from "../lib/sessao";
import { nomesVisibilidade } from "../lib/tipos";
import { Barra, SeletorMes, Valor } from "./extras";
import { useCoresTema } from "./grafico";
import { Aviso, Botao, Campo, CarregandoLista, Cartao, Etiqueta, Selecao, TituloCartao } from "./ui";

export type PainelCasa = {
  mes: string;
  saldo_total_centavos: number;
  gastos_centavos: number;
  pode_editar: boolean;
  membros: {
    usuario_id: string;
    nome: string;
    saldo_centavos: number;
    gastos_centavos: number;
    contribuicao_centavos: number;
    percentual: number;
  }[];
  contas: {
    id: string;
    nome: string;
    tipo: string;
    visibilidade: "privada" | "saldo" | "compartilhada";
    dono: string;
    saldo_centavos: number;
  }[];
  categorias: { categoria_id: string | null; nome: string; cor: string | null; total_centavos: number }[];
  quem_deve_quem: { usuario_id: string; nome: string; valor_centavos: number }[];
  despesas: {
    id: string;
    descricao: string;
    data: string;
    total_centavos: number;
    modo: string;
    pago_por: string;
    pago_por_nome: string;
    partes: { usuario_id: string; nome: string; valor_centavos: number; acertada: boolean }[];
  }[];
  metas: {
    id: string;
    nome: string;
    tipo: string;
    alvo_centavos: number;
    data_alvo: string | null;
    atual_centavos: number;
    percentual: number;
    aporte_necessario_centavos: number;
    meses_restantes: number;
    contribuicoes: { usuario_id: string; nome: string; total_centavos: number }[];
    pode_apagar: boolean;
  }[];
};

const mesAtual = () => new Date().toLocaleDateString("sv-SE", { timeZone: "America/Sao_Paulo" }).slice(0, 7);

/** O consolidado da casa: só o que cada pessoa compartilhou, e de quem vem cada número. */
export function PainelDaCasa({ casaId }: { casaId: string }) {
  const [mes, setMes] = useState(mesAtual);
  const chave = ["casa-painel", casaId, mes];
  const {
    data: p,
    isPending,
    error,
  } = useQuery({
    queryKey: chave,
    queryFn: () => obter<PainelCasa>(`/api/casas/${casaId}/painel?mes=${mes}`),
  });
  // cores das pessoas: a paleta categórica validada (D37), na ordem de entrada na casa
  const coresPessoas = Object.values(paleta(useCoresTema()));
  if (isPending) return <CarregandoLista />;
  if (error || !p) return <Aviso tipo="erro">{mensagemDe(error)}</Aviso>;
  const cor = (id: string) =>
    coresPessoas[p.membros.findIndex((m) => m.usuario_id === id) % coresPessoas.length] ?? "var(--primaria)";
  const maiorCategoria = p.categorias[0]?.total_centavos ?? 0;

  return (
    <>
      <Cartao>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3 [&>div]:mb-0">
          <TituloCartao>Consolidado da casa</TituloCartao>
          <SeletorMes mes={mes} onChange={setMes} />
        </div>
        <dl className="grid grid-cols-2 gap-3">
          <div>
            <dt className="text-xs text-texto-2">Saldo compartilhado</dt>
            <dd>
              <Valor centavos={p.saldo_total_centavos} saldo className="text-xl font-bold" />
            </dd>
          </div>
          <div>
            <dt className="text-xs text-texto-2">Gastos do mês</dt>
            <dd>
              <Valor centavos={p.gastos_centavos} saldo className="text-xl font-bold" />
            </dd>
          </div>
        </dl>
        <h3 className="mt-5 mb-1 text-sm font-semibold">Contribuição de cada um</h3>
        <p className="mb-2 text-xs text-texto-2">
          Gastos nas contas compartilhadas mais as despesas divididas que a pessoa pagou.
        </p>
        <ul className="flex flex-col gap-3" aria-label="Contribuição por membro">
          {p.membros.map((m) => (
            <li key={m.usuario_id}>
              <div className="flex items-baseline justify-between gap-2 text-sm">
                <span className="min-w-0 truncate font-medium">{m.nome}</span>
                <span className="shrink-0">
                  <span className="valor num">{formatarMoeda(m.contribuicao_centavos)}</span>{" "}
                  <span className="text-xs text-texto-2">{formatarPercentual(m.percentual, 0)}</span>
                </span>
              </div>
              <Barra fracao={m.percentual} cor={cor(m.usuario_id)} rotulo={`Contribuição de ${m.nome}`} />
              <p className="mt-1 text-xs text-texto-2">
                compartilha <span className="valor num">{formatarMoeda(m.saldo_centavos)}</span> em saldo
              </p>
            </li>
          ))}
        </ul>
      </Cartao>

      <QuemDeveQuem casaId={casaId} p={p} />

      {p.categorias.length ? (
        <Cartao>
          <TituloCartao>Gastos da casa por categoria</TituloCartao>
          <ul className="flex flex-col gap-3">
            {p.categorias.map((c) => (
              <li key={c.categoria_id ?? c.nome}>
                <div className="flex justify-between gap-2 text-sm">
                  <span className="truncate">{c.nome}</span>
                  <span className="valor num shrink-0">{formatarMoeda(c.total_centavos)}</span>
                </div>
                <Barra
                  fracao={maiorCategoria ? c.total_centavos / maiorCategoria : 0}
                  cor={c.cor ?? "var(--destaque)"}
                  rotulo={`Gasto em ${c.nome}`}
                />
              </li>
            ))}
          </ul>
        </Cartao>
      ) : null}

      <Cartao>
        <TituloCartao>Contas compartilhadas</TituloCartao>
        {p.contas.length ? (
          <ul className="flex flex-col divide-y divide-borda">
            {p.contas.map((c) => (
              <li key={c.id} className="flex min-h-12 items-center justify-between gap-3 py-2 text-sm">
                <span className="min-w-0">
                  <span className="block truncate font-medium">{c.nome}</span>
                  <span className="text-xs text-texto-2">
                    de {c.dono} · {nomesVisibilidade[c.visibilidade]}
                  </span>
                </span>
                <Valor centavos={c.saldo_centavos} saldo />
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-texto-2">
            Ninguém compartilhou conta ainda. Em Contas, escolha "compartilhada" ou "só saldo" e esta casa.
          </p>
        )}
      </Cartao>

      <MetasDaCasa casaId={casaId} p={p} />
    </>
  );
}

function QuemDeveQuem({ casaId, p }: { casaId: string; p: PainelCasa }) {
  const cliente = useQueryClient();
  const { data: sessao } = useSessao();
  const atualizar = () => cliente.invalidateQueries({ queryKey: ["casa-painel", casaId] });
  const acertar = useMutation({
    mutationFn: (usuario: string) => api("POST", `/api/casas/${casaId}/acertos`, { usuario_id: usuario }),
    onSuccess: atualizar,
  });
  const desfazer = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/divisoes/${id}`),
    onSuccess: atualizar,
  });
  const erro = acertar.error ?? desfazer.error;

  return (
    <Cartao>
      <TituloCartao>
        <span className="flex items-center gap-2">
          <HandCoins className="size-4 text-destaque" aria-hidden /> Quem deve quem
        </span>
      </TituloCartao>
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      {p.quem_deve_quem.length ? (
        <ul className="flex flex-col divide-y divide-borda" aria-label="Saldos com cada pessoa">
          {p.quem_deve_quem.map((s) => {
            const valor = formatarMoeda(Math.abs(s.valor_centavos));
            return (
              <li
                key={s.usuario_id}
                className="flex min-h-14 flex-wrap items-center justify-between gap-3 py-2"
              >
                <p className="text-sm">
                  {s.valor_centavos > 0 ? (
                    <>
                      {s.nome} te deve <strong className="valor num text-entrada">{valor}</strong>
                    </>
                  ) : (
                    <>
                      Você deve <strong className="valor num text-saida">{valor}</strong> a {s.nome}
                    </>
                  )}
                </p>
                {p.pode_editar ? (
                  <Botao
                    variante="secundario"
                    carregando={acertar.isPending && acertar.variables === s.usuario_id}
                    onClick={() =>
                      confirm(
                        s.valor_centavos > 0
                          ? `${s.nome} já te pagou ${valor}? Tudo em aberto entre vocês fica acertado.`
                          : `Você já pagou ${valor} a ${s.nome}? Tudo em aberto entre vocês fica acertado.`,
                      ) && acertar.mutate(s.usuario_id)
                    }
                  >
                    Acertar
                  </Botao>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="text-sm text-texto-2">
          Tudo acertado. Para dividir uma compra, abra a transação em Gastos e toque em "Dividir com a casa".
        </p>
      )}
      {p.despesas.length ? (
        <>
          <h3 className="mt-5 mb-2 text-sm font-semibold">Despesas divididas</h3>
          <ul className="flex flex-col divide-y divide-borda" aria-label="Despesas divididas">
            {p.despesas.map((d) => (
              <li key={d.id} className="flex flex-col gap-1 py-2 text-sm">
                <div className="flex items-start justify-between gap-2">
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{d.descricao}</span>
                    <span className="text-xs text-texto-2">
                      {formatarData(d.data)} ·{" "}
                      {d.pago_por === sessao?.usuario_id ? "você pagou" : `${d.pago_por_nome} pagou`}
                    </span>
                  </span>
                  <span className="flex shrink-0 items-center gap-1">
                    <span className="valor num">{formatarMoeda(d.total_centavos)}</span>
                    {d.pago_por === sessao?.usuario_id ? (
                      <button
                        type="button"
                        aria-label={`Desfazer a divisão de ${d.descricao}`}
                        onClick={() => confirm("Desfazer esta divisão?") && desfazer.mutate(d.id)}
                        className="grid size-11 place-items-center rounded-xl text-texto-2 hover:bg-superficie-2"
                      >
                        <Trash2 className="size-4" aria-hidden />
                      </button>
                    ) : null}
                  </span>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {d.partes.map((pt) => (
                    <Etiqueta key={pt.usuario_id}>
                      {`${pt.nome}:\u00a0`}
                      <span className="valor num">{formatarMoeda(pt.valor_centavos)}</span>
                      {pt.acertada && pt.usuario_id !== d.pago_por ? " ✓" : ""}
                    </Etiqueta>
                  ))}
                </div>
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </Cartao>
  );
}

const tiposMeta: [string, string][] = [
  ["viagem", "Viagem"],
  ["reserva", "Reserva"],
  ["compra", "Compra"],
  ["outra", "Outra"],
];

function MetasDaCasa({ casaId, p }: { casaId: string; p: PainelCasa }) {
  const cliente = useQueryClient();
  const atualizar = () => cliente.invalidateQueries({ queryKey: ["casa-painel", casaId] });
  const [nova, setNova] = useState(false);
  const [nome, setNome] = useState("");
  const [tipo, setTipo] = useState("viagem");
  const [alvo, setAlvo] = useState("");
  const [data, setData] = useState("");
  const [valores, setValores] = useState<Record<string, string>>({});
  const alvoC = lerCentavos(alvo);
  const criar = useMutation({
    mutationFn: () =>
      api("POST", `/api/casas/${casaId}/metas`, {
        nome,
        tipo,
        alvo_centavos: alvoC,
        data_alvo: data || null,
      }),
    onSuccess: async () => {
      setNova(false);
      setNome("");
      setAlvo("");
      setData("");
      await atualizar();
    },
  });
  const contribuir = useMutation({
    mutationFn: (meta: string) =>
      api("POST", `/api/metas/${meta}/contribuicoes`, { valor_centavos: lerCentavos(valores[meta] ?? "") }),
    onSuccess: async (_, meta) => {
      setValores((v) => ({ ...v, [meta]: "" }));
      await atualizar();
    },
  });
  const apagar = useMutation({
    mutationFn: (meta: string) => api("DELETE", `/api/metas/${meta}`),
    onSuccess: atualizar,
  });
  const erro = criar.error ?? contribuir.error ?? apagar.error;

  return (
    <Cartao>
      <TituloCartao
        acao={
          p.pode_editar && !nova ? (
            <Botao variante="fantasma" onClick={() => setNova(true)}>
              <Plus className="size-4" aria-hidden /> Nova meta
            </Botao>
          ) : null
        }
      >
        <span className="flex items-center gap-2">
          <Target className="size-4 text-destaque" aria-hidden /> Metas da casa
        </span>
      </TituloCartao>
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      {nova ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            criar.mutate();
          }}
          className="mb-4 grid gap-3 sm:grid-cols-2"
        >
          <Campo
            rotulo="Nome da meta"
            value={nome}
            onChange={(e) => setNome(e.target.value)}
            required
            maxLength={100}
          />
          <Selecao rotulo="Tipo" value={tipo} onChange={(e) => setTipo(e.target.value)}>
            {tiposMeta.map(([v, n]) => (
              <option key={v} value={v}>
                {n}
              </option>
            ))}
          </Selecao>
          <Campo
            rotulo="Quanto juntar (R$)"
            inputMode="decimal"
            value={alvo}
            onChange={(e) => setAlvo(e.target.value)}
            required
            erro={alvo && !alvoC ? "Valor inválido" : undefined}
          />
          <Campo
            rotulo="Até quando (opcional)"
            type="date"
            value={data}
            onChange={(e) => setData(e.target.value)}
          />
          <div className="flex gap-2 sm:col-span-2">
            <Botao type="submit" carregando={criar.isPending} disabled={!alvoC}>
              Criar meta
            </Botao>
            <Botao variante="fantasma" onClick={() => setNova(false)}>
              Cancelar
            </Botao>
          </div>
        </form>
      ) : null}
      {p.metas.length ? (
        <ul className="flex flex-col gap-5">
          {p.metas.map((m) => (
            <li key={m.id} aria-label={m.nome}>
              <div className="flex items-start justify-between gap-2">
                <span className="min-w-0">
                  <span className="block truncate font-medium">{m.nome}</span>
                  <span className="text-xs text-texto-2">
                    <span className="valor num">{formatarMoeda(m.atual_centavos)}</span> de{" "}
                    <span className="valor num">{formatarMoeda(m.alvo_centavos)}</span>
                    {m.data_alvo ? ` até ${formatarData(m.data_alvo)}` : ""}
                  </span>
                </span>
                <span className="flex shrink-0 items-center gap-1">
                  <span className="text-sm font-semibold">
                    {formatarPercentual(Math.min(m.percentual, 1), 0)}
                  </span>
                  {m.pode_apagar ? (
                    <button
                      type="button"
                      aria-label={`Apagar a meta ${m.nome}`}
                      onClick={() => confirm(`Apagar a meta ${m.nome}?`) && apagar.mutate(m.id)}
                      className="grid size-11 place-items-center rounded-xl text-texto-2 hover:bg-superficie-2"
                    >
                      <Trash2 className="size-4" aria-hidden />
                    </button>
                  ) : null}
                </span>
              </div>
              <Barra fracao={m.percentual} cor="var(--entrada)" rotulo={`Progresso de ${m.nome}`} />
              {m.data_alvo && m.atual_centavos < m.alvo_centavos && m.meses_restantes > 0 ? (
                <p className="mt-1 text-xs text-texto-2">
                  Faltam <span className="valor num">{formatarMoeda(m.aporte_necessario_centavos)}</span> por
                  mês para chegar a tempo.
                </p>
              ) : null}
              {m.contribuicoes.length ? (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {m.contribuicoes.map((c) => (
                    <Etiqueta key={c.usuario_id}>
                      {`${c.nome}:\u00a0`}
                      <span className="valor num">{formatarMoeda(c.total_centavos)}</span>
                    </Etiqueta>
                  ))}
                </div>
              ) : null}
              {p.pode_editar ? (
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    contribuir.mutate(m.id);
                  }}
                  className="mt-3 grid grid-cols-[minmax(0,1fr)_auto] items-end gap-2"
                >
                  <Campo
                    rotulo="Quanto você pôs (R$)"
                    className="min-w-0"
                    inputMode="decimal"
                    value={valores[m.id] ?? ""}
                    onChange={(e) => setValores((v) => ({ ...v, [m.id]: e.target.value }))}
                  />
                  <Botao
                    type="submit"
                    variante="secundario"
                    carregando={contribuir.isPending && contribuir.variables === m.id}
                    disabled={!lerCentavos(valores[m.id] ?? "")}
                  >
                    Contribuir
                  </Botao>
                </form>
              ) : null}
            </li>
          ))}
        </ul>
      ) : !nova ? (
        <p className="text-sm text-texto-2">
          Nenhuma meta conjunta. Viagem, reserva da casa, uma compra grande…
        </p>
      ) : null}
    </Cartao>
  );
}
