import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeftRight, Plus, ReceiptText, Search, Trash2, Upload } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Dialogo, SeletorCategoria, SeletorMes, Valor } from "../components/extras";
import { Aviso, Botao, Campo, CarregandoLista, Cartao, Pagina, Selecao, Vazio } from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { limitesMes, mesAtual, useCategorias, useContas } from "../lib/dados";
import { formatarData, lerCentavos } from "../lib/formato";
import type { Transacao } from "../lib/tipos";

type Pagina_ = { itens: Transacao[]; total: number };
const POR_PAGINA = 100;

function useInvalidarTransacoes() {
  const cliente = useQueryClient();
  return () =>
    Promise.all(
      ["transacoes", "resumo", "contas", "importacoes"].map((k) =>
        cliente.invalidateQueries({ queryKey: [k] }),
      ),
    );
}

export function Transacoes() {
  const [params, setParams] = useSearchParams();
  const mes = params.get("mes") ?? mesAtual();
  const conta = params.get("conta") ?? "";
  const categoria = params.get("categoria") ?? "";
  const semCategoria = params.get("sem_categoria") === "1";
  const importacao = params.get("importacao") ?? "";
  const [busca, setBusca] = useState(params.get("busca") ?? "");
  const [buscaAtiva, setBuscaAtiva] = useState(busca);
  const [selecionadas, setSelecionadas] = useState<Set<string>>(new Set());
  const [editando, setEditando] = useState<Transacao | null>(null);
  const [nova, setNova] = useState(false);
  const campoBusca = useRef<HTMLInputElement>(null);
  const contas = useContas();
  const categorias = useCategorias();

  const definir = (k: string, v: string) => {
    const p = new URLSearchParams(params);
    if (v) p.set(k, v);
    else p.delete(k);
    setParams(p, { replace: true });
    setSelecionadas(new Set());
  };

  // busca com atraso curto, para não consultar a cada tecla
  useEffect(() => {
    const t = setTimeout(() => setBuscaAtiva(busca.trim()), 250);
    return () => clearTimeout(t);
  }, [busca]);

  // atalhos: "/" busca, "n" nova transação
  useEffect(() => {
    const aoTeclar = (e: KeyboardEvent) => {
      const alvo = e.target as HTMLElement;
      if (alvo.closest("input, textarea, select, dialog") || e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "/") {
        e.preventDefault();
        campoBusca.current?.focus();
      } else if (e.key === "n") {
        e.preventDefault();
        setNova(true);
      }
    };
    window.addEventListener("keydown", aoTeclar);
    return () => window.removeEventListener("keydown", aoTeclar);
  }, []);

  const filtro = useMemo(() => {
    const q = new URLSearchParams();
    // buscando ou vendo uma importação, o mês não limita
    if (!buscaAtiva && !importacao) {
      const [de, ate] = limitesMes(mes);
      q.set("de", de);
      q.set("ate", ate);
    }
    if (conta) q.set("conta_id", conta);
    if (categoria) q.set("categoria_id", categoria);
    if (semCategoria) q.set("sem_categoria", "1");
    if (importacao) q.set("importacao_id", importacao);
    if (buscaAtiva) q.set("busca", buscaAtiva);
    return q.toString();
  }, [mes, conta, categoria, semCategoria, importacao, buscaAtiva]);

  const consulta = useInfiniteQuery({
    queryKey: ["transacoes", filtro],
    queryFn: ({ pageParam }) =>
      obter<Pagina_>(`/api/transacoes?${filtro}&limite=${POR_PAGINA}&pular=${pageParam}`),
    initialPageParam: 0,
    getNextPageParam: (ultima, todas) => {
      const carregadas = todas.reduce((s, p) => s + p.itens.length, 0);
      return carregadas < ultima.total ? carregadas : undefined;
    },
  });
  const itens = consulta.data?.pages.flatMap((p) => p.itens) ?? [];
  const total = consulta.data?.pages[0]?.total ?? 0;

  const porDia = new Map<string, Transacao[]>();
  for (const t of itens) porDia.set(t.data, [...(porDia.get(t.data) ?? []), t]);

  const alternar = (id: string) =>
    setSelecionadas((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

  const semContas = contas.data?.filter((c) => c.pode_editar).length === 0;

  return (
    <Pagina
      titulo="Gastos e transações"
      subtitulo="Onde está aquela compra?"
      acao={
        <div className="flex flex-wrap gap-2">
          <Link
            to="/importar"
            className="inline-flex min-h-11 items-center gap-2 rounded-xl border border-borda bg-superficie-2 px-4 text-sm font-semibold hover:border-texto-2"
          >
            <Upload className="size-4" aria-hidden /> Importar
          </Link>
          <Botao onClick={() => setNova(true)} disabled={semContas} title="Nova transação (n)">
            <Plus className="size-4" aria-hidden /> Nova
          </Botao>
        </div>
      }
    >
      <Cartao className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          {!buscaAtiva && !importacao ? <SeletorMes mes={mes} onChange={(m) => definir("mes", m)} /> : null}
          <label className="relative min-w-0 flex-1 basis-56">
            <span className="sr-only">Buscar</span>
            <Search
              className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-texto-2"
              aria-hidden
            />
            <input
              ref={campoBusca}
              type="search"
              value={busca}
              onChange={(e) => setBusca(e.target.value)}
              placeholder="Buscar descrição ou nota  ( / )"
              className="min-h-11 w-full rounded-xl border border-borda bg-superficie-2 pr-3 pl-9 text-base focus:border-primaria focus:outline-none"
            />
          </label>
        </div>
        <div className="grid gap-3 sm:grid-cols-3">
          <Selecao rotulo="Conta" value={conta} onChange={(e) => definir("conta", e.target.value)}>
            <option value="">Todas as contas</option>
            {(contas.data ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.nome}
                {c.entidade_nome ? "" : c.dono ? ` (de ${c.dono})` : ""}
              </option>
            ))}
          </Selecao>
          <SeletorCategoria
            categorias={categorias.data ?? []}
            valor={categoria}
            onChange={(v) => definir("categoria", v)}
            vazio="Todas as categorias"
          />
          <label className="flex min-h-11 items-center gap-3 self-end text-sm">
            <input
              type="checkbox"
              className="size-5 accent-[var(--primaria)]"
              checked={semCategoria}
              onChange={(e) => definir("sem_categoria", e.target.checked ? "1" : "")}
            />
            Só sem categoria
          </label>
        </div>
        {importacao ? (
          <Aviso>
            Mostrando as transações de uma importação.{" "}
            <button
              type="button"
              className="font-semibold underline"
              onClick={() => definir("importacao", "")}
            >
              Ver todas
            </button>
          </Aviso>
        ) : null}
      </Cartao>

      {consulta.error ? <Aviso tipo="erro">{mensagemDe(consulta.error)}</Aviso> : null}

      <Cartao className="p-2 sm:p-3">
        {consulta.isPending ? (
          <div className="p-2">
            <CarregandoLista linhas={6} />
          </div>
        ) : itens.length === 0 ? (
          <Vazio
            icone={<ReceiptText className="size-8" />}
            titulo="Nenhuma transação aqui"
            texto={
              semContas
                ? "Cadastre uma conta e importe o extrato (OFX ou CSV)."
                : "Importe um extrato ou mude os filtros."
            }
            acao={
              <Link to={semContas ? "/contas" : "/importar"} className="text-sm font-semibold text-primaria">
                {semContas ? "Cadastrar conta" : "Importar extrato"}
              </Link>
            }
          />
        ) : (
          <>
            <p className="px-2 pt-1 pb-2 text-xs text-texto-2">
              {total} {total === 1 ? "transação" : "transações"}
            </p>
            {[...porDia.entries()].map(([dia, lista]) => (
              <section key={dia} aria-label={formatarData(dia)}>
                <h2 className="sticky top-14 z-[1] bg-superficie px-2 py-1.5 text-xs font-semibold tracking-wide text-texto-2 uppercase md:top-0">
                  {new Intl.DateTimeFormat("pt-BR", {
                    weekday: "short",
                    day: "2-digit",
                    month: "short",
                    timeZone: "UTC",
                  }).format(new Date(`${dia}T00:00:00Z`))}
                </h2>
                <ul>
                  {lista.map((t) => (
                    <LinhaTransacao
                      key={t.id}
                      t={t}
                      marcada={selecionadas.has(t.id)}
                      aoMarcar={() => alternar(t.id)}
                      aoAbrir={() => setEditando(t)}
                    />
                  ))}
                </ul>
              </section>
            ))}
            {consulta.hasNextPage ? (
              <div className="flex justify-center p-3">
                <Botao
                  variante="secundario"
                  carregando={consulta.isFetchingNextPage}
                  onClick={() => consulta.fetchNextPage()}
                >
                  Carregar mais
                </Botao>
              </div>
            ) : null}
          </>
        )}
      </Cartao>

      {selecionadas.size ? (
        <BarraLote ids={[...selecionadas]} itens={itens} aoConcluir={() => setSelecionadas(new Set())} />
      ) : null}

      <Dialogo aberto={editando !== null} aoFechar={() => setEditando(null)} titulo="Transação">
        {editando ? <EditarTransacao t={editando} aoFechar={() => setEditando(null)} /> : null}
      </Dialogo>
      <Dialogo aberto={nova} aoFechar={() => setNova(false)} titulo="Nova transação">
        <NovaTransacao contaInicial={conta} aoFechar={() => setNova(false)} />
      </Dialogo>
    </Pagina>
  );
}

function LinhaTransacao({
  t,
  marcada,
  aoMarcar,
  aoAbrir,
}: {
  t: Transacao;
  marcada: boolean;
  aoMarcar: () => void;
  aoAbrir: () => void;
}) {
  const categoria = t.categoria
    ? t.categoria_pai
      ? `${t.categoria_pai} › ${t.categoria}`
      : t.categoria
    : null;
  return (
    <li
      className={`flex items-center gap-2 rounded-xl px-1 ${marcada ? "bg-primaria/10" : "hover:bg-superficie-2"}`}
    >
      <input
        type="checkbox"
        aria-label={`Selecionar ${t.descricao}`}
        checked={marcada}
        onChange={aoMarcar}
        className="m-3 size-5 shrink-0 accent-[var(--primaria)]"
      />
      <button
        type="button"
        onClick={aoAbrir}
        className="flex min-h-14 min-w-0 flex-1 items-center gap-3 py-2 pr-2 text-left"
      >
        <span
          className="size-2.5 shrink-0 rounded-full"
          style={{ backgroundColor: t.cor ?? "var(--borda)" }}
          aria-hidden
        />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{t.descricao}</span>
          <span className="flex items-center gap-1 truncate text-xs text-texto-2">
            {t.tipo === "transferencia" ? (
              <ArrowLeftRight className="size-3 shrink-0" aria-label="transferência" />
            ) : null}
            {t.conta} · {categoria ?? <span className="text-alerta">sem categoria</span>}
          </span>
        </span>
        <Valor centavos={t.valor_centavos} moeda={t.moeda} tipo={t.tipo} className="text-sm font-semibold" />
      </button>
    </li>
  );
}

function BarraLote({
  ids,
  itens,
  aoConcluir,
}: {
  ids: string[];
  itens: Transacao[];
  aoConcluir: () => void;
}) {
  const categorias = useCategorias();
  const invalidar = useInvalidarTransacoes();
  const [categoria, setCategoria] = useState("");
  const primeira = itens.find((t) => t.id === ids[0]);
  const [regra, setRegra] = useState("");
  const [criarRegra, setCriarRegra] = useState(false);
  const aplicar = useMutation({
    mutationFn: () =>
      api("POST", "/api/transacoes/lote", {
        ids,
        categoria_id: categoria || null,
        criar_regra: criarRegra ? regra : "",
      }),
    onSuccess: async () => {
      await invalidar();
      aoConcluir();
    },
  });
  return (
    <div className="fixed inset-x-0 bottom-16 z-20 px-3 pb-[env(safe-area-inset-bottom)] md:bottom-4 md:left-[232px]">
      <div className="mx-auto flex max-w-3xl flex-col gap-3 rounded-cartao border border-borda bg-superficie p-4 shadow-cartao">
        {aplicar.error ? <Aviso tipo="erro">{mensagemDe(aplicar.error)}</Aviso> : null}
        <div className="flex flex-wrap items-end gap-3">
          <p className="text-sm font-semibold">
            {ids.length} {ids.length === 1 ? "selecionada" : "selecionadas"}
          </p>
          <div className="min-w-0 flex-1 basis-52">
            <SeletorCategoria
              categorias={categorias.data ?? []}
              entidadeId={primeira?.entidade_id}
              valor={categoria}
              onChange={setCategoria}
              rotulo="Mudar categoria para"
              vazio="Escolha a categoria…"
            />
          </div>
          <Botao onClick={() => aplicar.mutate()} carregando={aplicar.isPending} disabled={!categoria}>
            Aplicar
          </Botao>
          <Botao variante="fantasma" onClick={aoConcluir}>
            Cancelar
          </Botao>
        </div>
        {categoria ? (
          <div className="flex flex-wrap items-center gap-3 text-sm">
            <label className="flex min-h-11 items-center gap-2">
              <input
                type="checkbox"
                className="size-5 accent-[var(--primaria)]"
                checked={criarRegra}
                onChange={(e) => {
                  setCriarRegra(e.target.checked);
                  if (e.target.checked && !regra)
                    setRegra(
                      primeira?.descricao
                        .split(/[\s*\-0-9]+/)
                        .filter(Boolean)
                        .slice(0, 2)
                        .join(" ") ?? "",
                    );
                }}
              />
              Criar regra: descrição contém
            </label>
            {criarRegra ? (
              <input
                aria-label="Texto da regra"
                value={regra}
                onChange={(e) => setRegra(e.target.value)}
                className="min-h-11 flex-1 rounded-xl border border-borda bg-superficie-2 px-3"
              />
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function EditarTransacao({ t, aoFechar }: { t: Transacao; aoFechar: () => void }) {
  const categorias = useCategorias();
  const invalidar = useInvalidarTransacoes();
  const [descricao, setDescricao] = useState(t.descricao);
  const [notas, setNotas] = useState(t.notas ?? "");
  const [categoria, setCategoria] = useState(t.categoria_id ?? "");
  const salvar = useMutation({
    mutationFn: () =>
      api("PATCH", `/api/transacoes/${t.id}`, {
        descricao,
        notas,
        alterar_categoria: categoria !== (t.categoria_id ?? ""),
        categoria_id: categoria || null,
      }),
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const apagar = useMutation({
    mutationFn: () => api("DELETE", `/api/transacoes/${t.id}`),
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const erro = salvar.error ?? apagar.error;
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        salvar.mutate();
      }}
      className="flex flex-col gap-4"
    >
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-sm text-texto-2">
          {formatarData(t.data)} · {t.conta}
        </span>
        <Valor centavos={t.valor_centavos} moeda={t.moeda} tipo={t.tipo} className="text-xl font-bold" />
      </div>
      {t.tipo === "transferencia" && t.transferencia_par_id ? (
        <Aviso>
          Transferência entre contas: não conta como gasto nem receita. Escolha outra categoria para desfazer
          o par.
        </Aviso>
      ) : null}
      {t.tipo === "estorno" ? <Aviso>Estorno: abate o gasto da categoria.</Aviso> : null}
      <Campo
        rotulo="Descrição"
        value={descricao}
        onChange={(e) => setDescricao(e.target.value)}
        required
        maxLength={200}
      />
      {t.descricao_original !== t.descricao ? (
        <p className="-mt-2 text-xs text-texto-2">Original: {t.descricao_original}</p>
      ) : null}
      <SeletorCategoria
        categorias={categorias.data ?? []}
        entidadeId={t.entidade_id}
        valor={categoria}
        onChange={setCategoria}
      />
      <Campo rotulo="Notas" value={notas} onChange={(e) => setNotas(e.target.value)} maxLength={500} />
      <p className="text-xs text-texto-2">
        Origem: {t.origem.toUpperCase()}
        {t.importacao_id ? (
          <>
            {" · "}
            <Link to={`/gastos?importacao=${t.importacao_id}`} onClick={aoFechar} className="underline">
              ver a importação
            </Link>
          </>
        ) : null}
      </p>
      <div className="flex flex-wrap justify-between gap-2">
        <Botao
          variante="perigo"
          carregando={apagar.isPending}
          onClick={() => confirm("Apagar esta transação?") && apagar.mutate()}
        >
          <Trash2 className="size-4" aria-hidden /> Apagar
        </Botao>
        <div className="flex gap-2">
          <Botao variante="fantasma" onClick={aoFechar}>
            Cancelar
          </Botao>
          <Botao type="submit" carregando={salvar.isPending}>
            Salvar
          </Botao>
        </div>
      </div>
    </form>
  );
}

function NovaTransacao({ contaInicial, aoFechar }: { contaInicial: string; aoFechar: () => void }) {
  const contas = useContas();
  const categorias = useCategorias();
  const invalidar = useInvalidarTransacoes();
  const editaveis = (contas.data ?? []).filter((c) => c.pode_editar && !c.arquivada);
  const hoje = new Intl.DateTimeFormat("en-CA", { timeZone: "America/Sao_Paulo" }).format(new Date());
  const [contaId, setContaId] = useState(
    editaveis.some((c) => c.id === contaInicial) ? contaInicial : (editaveis[0]?.id ?? ""),
  );
  const [data, setData] = useState(hoje);
  const [descricao, setDescricao] = useState("");
  const [valor, setValor] = useState("");
  const [saida, setSaida] = useState(true);
  const [categoria, setCategoria] = useState("");
  const conta = editaveis.find((c) => c.id === contaId);
  const criar = useMutation({
    mutationFn: () => {
      const c = lerCentavos(valor);
      if (c === null || c === 0) throw new Error("Valor inválido. Use o formato 45,90.");
      return api("POST", "/api/transacoes", {
        conta_id: contaId,
        data,
        descricao,
        valor_centavos: saida ? -Math.abs(c) : Math.abs(c),
        categoria_id: categoria || null,
      });
    },
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        criar.mutate();
      }}
      className="flex flex-col gap-4"
    >
      {criar.error ? <Aviso tipo="erro">{mensagemDe(criar.error)}</Aviso> : null}
      <fieldset className="grid grid-cols-2 gap-2">
        <legend className="sr-only">Tipo</legend>
        {[
          {
            saida: true,
            nome: "Saída",
            cor: "has-[:checked]:border-saida has-[:checked]:bg-saida/10 has-[:checked]:text-saida",
          },
          {
            saida: false,
            nome: "Entrada",
            cor: "has-[:checked]:border-entrada has-[:checked]:bg-entrada/10 has-[:checked]:text-entrada",
          },
        ].map((o) => (
          <label
            key={o.nome}
            className={`flex min-h-11 cursor-pointer items-center justify-center rounded-xl border border-borda text-sm font-semibold text-texto-2 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-primaria ${o.cor}`}
          >
            <input
              type="radio"
              name="tipo"
              className="sr-only"
              checked={saida === o.saida}
              onChange={() => setSaida(o.saida)}
            />
            {o.nome}
          </label>
        ))}
      </fieldset>
      <Campo
        rotulo="Valor"
        inputMode="decimal"
        value={valor}
        onChange={(e) => setValor(e.target.value)}
        required
        placeholder="45,90"
        autoFocus
      />
      <Campo
        rotulo="Descrição"
        value={descricao}
        onChange={(e) => setDescricao(e.target.value)}
        required
        maxLength={200}
        placeholder="Padaria"
      />
      <div className="grid grid-cols-2 gap-3">
        <Selecao rotulo="Conta" value={contaId} onChange={(e) => setContaId(e.target.value)}>
          {editaveis.map((c) => (
            <option key={c.id} value={c.id}>
              {c.nome}
            </option>
          ))}
        </Selecao>
        <Campo rotulo="Data" type="date" value={data} onChange={(e) => setData(e.target.value)} required />
      </div>
      <SeletorCategoria
        categorias={categorias.data ?? []}
        entidadeId={conta?.entidade_id}
        valor={categoria}
        onChange={setCategoria}
        vazio="Automática (regras e histórico)"
      />
      <div className="flex justify-end gap-2">
        <Botao variante="fantasma" onClick={aoFechar}>
          Cancelar
        </Botao>
        <Botao type="submit" carregando={criar.isPending}>
          Lançar
        </Botao>
      </div>
    </form>
  );
}
