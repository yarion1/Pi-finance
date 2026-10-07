import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CreditCard, Pencil } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Barra, Dialogo, Valor } from "../components/extras";
import { GraficoCartoesMes, GraficoMensal } from "../components/graficos-painel";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { nomeMes, useContas } from "../lib/dados";
import { centavosParaTexto, formatarData, formatarMoeda, lerCentavos } from "../lib/formato";
import type { Conta, Fatura, FaturaBanco, ParcelaFutura, RespostaFaturas } from "../lib/tipos";

const nomesStatus: Record<Fatura["status"], string> = {
  aberta: "Aberta",
  futura: "Futura",
  fechada: "Fechada",
  anterior: "Anterior",
};

export function Cartoes() {
  const contas = useContas();
  const [params, setParams] = useSearchParams();
  const cartoes = (contas.data ?? []).filter((c) => c.tipo === "cartao" && !c.arquivada);
  const escolhido = cartoes.find((c) => c.id === params.get("id")) ?? cartoes[0];

  return (
    <Pagina titulo="Cartões" subtitulo="Quanto já está comprometido nas próximas faturas.">
      {contas.error ? <Aviso tipo="erro">{mensagemDe(contas.error)}</Aviso> : null}
      {contas.isPending ? (
        <Cartao>
          <CarregandoLista />
        </Cartao>
      ) : cartoes.length === 0 ? (
        <Cartao>
          <Vazio
            icone={<CreditCard className="size-8" />}
            titulo="Nenhum cartão cadastrado"
            texto="Cadastre o cartão com os dias de fechamento e vencimento para ver as faturas e as parcelas futuras."
            acao={
              <Link to="/contas" className="text-sm font-semibold text-primaria">
                Cadastrar cartão
              </Link>
            }
          />
        </Cartao>
      ) : null}

      {cartoes.length ? <VisaoGeral /> : null}

      {cartoes.length > 1 ? (
        <div className="flex gap-2 overflow-x-auto pb-1" role="tablist" aria-label="Cartões">
          {cartoes.map((c) => (
            <button
              key={c.id}
              type="button"
              role="tab"
              aria-selected={c.id === escolhido?.id}
              onClick={() => setParams({ id: c.id }, { replace: true })}
              className="min-h-11 shrink-0 rounded-xl border border-borda px-4 text-sm font-semibold text-texto-2 aria-selected:border-primaria aria-selected:text-texto"
            >
              {c.nome}
            </button>
          ))}
        </div>
      ) : null}

      {escolhido ? <DetalheCartao key={escolhido.id} cartao={escolhido} /> : null}
    </Pagina>
  );
}

function DetalheCartao({ cartao }: { cartao: Conta }) {
  const dados = useQuery({
    queryKey: ["faturas", cartao.id],
    queryFn: () => obter<RespostaFaturas>(`/api/contas/${cartao.id}/faturas`),
  });
  const faturas = [...(dados.data?.faturas ?? [])].sort((a, b) => a.vencimento.localeCompare(b.vencimento));
  const aberta = faturas.find((f) => f.status === "aberta");
  const proximas = faturas.filter((f) => f.status === "aberta" || f.status === "futura");
  const passadas = faturas.filter((f) => f.status === "fechada" || f.status === "anterior").reverse();
  // pelo Open Finance o banco diz quanto do limite está usado (já com as parcelas futuras);
  // sem ele, o saldo do cartão (negativo quando há compras a pagar)
  const doBanco = cartao.limite_usado_banco_centavos;
  const usado = doBanco ?? Math.max(-(cartao.saldo_centavos ?? 0), 0);
  const semDias = cartao.fechamento === null || cartao.vencimento === null;

  return (
    <>
      <Cartao>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate text-lg font-semibold">{cartao.nome}</h2>
            <p className="text-sm text-texto-2">
              {semDias
                ? "Sem dias de fechamento e vencimento"
                : `Fecha dia ${cartao.fechamento} · vence dia ${cartao.vencimento}`}
            </p>
          </div>
          {!semDias ? <Etiqueta cor="entrada">Melhor dia de compra: {cartao.fechamento}</Etiqueta> : null}
        </div>
        {cartao.limite_centavos ? (
          <div className="mt-4">
            <div className="flex items-baseline justify-between gap-3 text-sm">
              <span className="text-texto-2">Limite usado{doBanco !== null ? " (segundo o banco)" : ""}</span>
              <span className="valor num">
                {formatarMoeda(usado, cartao.moeda)} de {formatarMoeda(cartao.limite_centavos, cartao.moeda)}
              </span>
            </div>
            <Barra
              fracao={usado / cartao.limite_centavos}
              cor={usado > cartao.limite_centavos * 0.9 ? "var(--saida)" : "var(--primaria)"}
              rotulo="Limite usado"
            />
            <p className="mt-1.5 text-xs text-texto-2">
              Disponível:{" "}
              <span className="valor num">
                {formatarMoeda(Math.max(cartao.limite_centavos - usado, 0), cartao.moeda)}
              </span>
            </p>
          </div>
        ) : null}
        {semDias ? (
          <div className="mt-3">
            <Aviso tipo="alerta">
              Informe os dias em{" "}
              <Link to="/contas" className="font-semibold underline">
                Contas
              </Link>{" "}
              para separar as compras por fatura.
            </Aviso>
          </div>
        ) : null}
      </Cartao>

      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}

      {!semDias ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Cartao>
            <TituloCartao>Próximas faturas</TituloCartao>
            {dados.isPending ? (
              <CarregandoLista />
            ) : (
              <ul className="flex flex-col divide-y divide-borda" aria-label="Próximas faturas">
                {proximas.map((f) => (
                  <LinhaFatura key={f.vencimento} fatura={f} moeda={cartao.moeda} destaque={f === aberta} />
                ))}
              </ul>
            )}
            {passadas.length ? (
              <details className="mt-3">
                <summary className="min-h-11 cursor-pointer py-2 text-sm font-semibold text-primaria">
                  Faturas anteriores ({passadas.length})
                </summary>
                <ul className="flex flex-col divide-y divide-borda">
                  {passadas.map((f) => (
                    <LinhaFatura key={f.vencimento} fatura={f} moeda={cartao.moeda} />
                  ))}
                </ul>
              </details>
            ) : null}
          </Cartao>

          <Cartao>
            <TituloCartao>Parcelas futuras</TituloCartao>
            {dados.isPending ? (
              <CarregandoLista />
            ) : dados.data?.parcelas_futuras.length ? (
              <>
                <GraficoParcelas parcelas={dados.data.parcelas_futuras} />
                <ParcelasPorMes parcelas={dados.data.parcelas_futuras} moeda={cartao.moeda} />
              </>
            ) : (
              <Vazio titulo="Nenhuma parcela a vencer" texto="Compras parceladas aparecem aqui mês a mês." />
            )}
          </Cartao>
        </div>
      ) : null}

      {dados.data?.faturas_banco?.length ? <FaturasDoBanco faturas={dados.data.faturas_banco} /> : null}
    </>
  );
}

// Faturas fechadas pelo próprio banco (Open Finance), com mínimo e encargos cobrados.
function FaturasDoBanco({ faturas }: { faturas: FaturaBanco[] }) {
  return (
    <Cartao>
      <TituloCartao>Faturas segundo o banco</TituloCartao>
      <ul className="flex flex-col divide-y divide-borda" aria-label="Faturas segundo o banco">
        {faturas.map((f) => (
          <li key={f.vencimento} className="flex min-h-14 items-center gap-3 py-2">
            <span className="min-w-0 flex-1">
              <span className="block text-sm">Vence {formatarData(f.vencimento)}</span>
              <span className="block text-xs text-texto-2">
                {f.fechamento ? `Fechou ${formatarData(f.fechamento)}` : "Fechamento não informado"}
                {f.minimo_centavos !== null ? ` · mínimo ${formatarMoeda(f.minimo_centavos, f.moeda)}` : ""}
              </span>
            </span>
            <span className="flex flex-col items-end gap-1">
              <span className="valor num whitespace-nowrap text-sm">
                {formatarMoeda(f.total_centavos, f.moeda)}
              </span>
              {f.encargos_centavos > 0 ? (
                <Etiqueta cor="alerta">Encargos {formatarMoeda(f.encargos_centavos, f.moeda)}</Etiqueta>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
    </Cartao>
  );
}

function LinhaFatura({ fatura: f, moeda, destaque }: { fatura: Fatura; moeda: string; destaque?: boolean }) {
  const total = f.compras_centavos + f.projetado_centavos;
  return (
    <li className={`flex min-h-14 items-center gap-3 py-2 ${destaque ? "font-semibold" : ""}`}>
      <span className="min-w-0 flex-1">
        <span className="block text-sm">Vence {formatarData(f.vencimento)}</span>
        <span className="block text-xs text-texto-2">
          Fecha {formatarData(f.fechamento)} · {f.transacoes}{" "}
          {f.transacoes === 1 ? "lançamento" : "lançamentos"}
          {f.projetado_centavos > 0 ? " · inclui parcelas previstas" : ""}
        </span>
      </span>
      <span className="flex flex-col items-end gap-1">
        <span className="valor num whitespace-nowrap text-sm">{formatarMoeda(total, moeda)}</span>
        {f.paga ? (
          <Etiqueta cor="entrada">Paga</Etiqueta>
        ) : (
          <Etiqueta cor={f.status === "aberta" ? "primaria" : "texto-2"}>{nomesStatus[f.status]}</Etiqueta>
        )}
      </span>
    </li>
  );
}

/** Quanto das parcelas cai em cada mês. */
function GraficoParcelas({ parcelas }: { parcelas: ParcelaFutura[] }) {
  const porMes = new Map<string, number>();
  for (const p of parcelas)
    porMes.set(p.mes.slice(0, 7), (porMes.get(p.mes.slice(0, 7)) ?? 0) - p.valor_centavos);
  const meses = [...porMes.keys()].sort();
  return (
    <div className="mb-3">
      <GraficoMensal
        meses={meses}
        valores={meses.map((m) => porMes.get(m) ?? 0)}
        cor={(c) => c.alerta}
        nome="Parcelas"
        rotulo={`Parcelas futuras por mês, de ${meses[0] ? nomeMes(meses[0]) : ""} a ${meses.length ? nomeMes(meses[meses.length - 1] ?? "") : ""}`}
      />
    </div>
  );
}

function ParcelasPorMes({ parcelas, moeda }: { parcelas: ParcelaFutura[]; moeda: string }) {
  const meses = new Map<string, ParcelaFutura[]>();
  for (const p of parcelas) meses.set(p.mes, [...(meses.get(p.mes) ?? []), p]);
  return (
    <div className="flex flex-col gap-4">
      {[...meses.entries()].map(([mes, lista]) => (
        <section key={mes}>
          <h3 className="flex items-baseline justify-between gap-3 text-sm font-semibold">
            <span>{nomeMes(mes.slice(0, 7))}</span>
            <span className="valor num">
              {formatarMoeda(
                lista.reduce((s, p) => s - p.valor_centavos, 0),
                moeda,
              )}
            </span>
          </h3>
          <ul className="mt-1 flex flex-col">
            {lista.map((p) => (
              <li
                key={`${p.descricao}-${p.parcela}-${p.data}`}
                className="flex min-h-10 items-center gap-3 text-sm"
              >
                <span className="min-w-0 flex-1 truncate">
                  {p.descricao.includes(`(${p.parcela}/${p.total})`)
                    ? p.descricao
                    : `${p.descricao} (${p.parcela}/${p.total})`}
                  {p.projetada ? <span className="text-texto-2"> · prevista</span> : null}
                </span>
                <Valor centavos={p.valor_centavos} moeda={moeda} />
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

type FaturaResumo = {
  vencimento: string;
  valor_centavos: number;
  status: "fechada" | "aberta" | "futura";
  paga: boolean;
  informado: boolean;
};
type CartaoResumo = {
  id: string;
  nome: string;
  moeda: string;
  limite_centavos: number | null;
  devendo_centavos: number;
  sem_dias: boolean;
  faturas: FaturaResumo[];
};
type ResumoCartoes = {
  meses: string[];
  total_por_mes_centavos: number[];
  total_devendo_centavos: number;
  cartoes: CartaoResumo[];
};

// até 8 cartões cada um com a sua cor; do 9º em diante, "Outros" (a cor segue o cartão)
const MAX_SERIES = 8;

/** Quanto devo em cada cartão e quanto vence em cada mês. */
function VisaoGeral() {
  const resumo = useQuery({
    queryKey: ["cartoes-resumo"],
    queryFn: () => obter<ResumoCartoes>("/api/cartoes/resumo"),
  });
  const [editando, setEditando] = useState<CartaoResumo | null>(null);
  const r = resumo.data;
  if (resumo.isPending) {
    return (
      <Cartao>
        <CarregandoLista linhas={2} />
      </Cartao>
    );
  }
  if (!r) return resumo.error ? <Aviso tipo="erro">{mensagemDe(resumo.error)}</Aviso> : null;

  const comDias = r.cartoes.filter((c) => !c.sem_dias && c.moeda === "BRL");
  const valoresDe = (c: CartaoResumo) =>
    r.meses.map((m) => c.faturas.find((f) => f.vencimento.startsWith(m))?.valor_centavos ?? 0);
  const series =
    comDias.length <= MAX_SERIES
      ? comDias.map((c) => ({ nome: c.nome, valores: valoresDe(c) }))
      : [
          ...comDias.slice(0, MAX_SERIES - 1).map((c) => ({ nome: c.nome, valores: valoresDe(c) })),
          {
            nome: "Outros",
            valores: r.meses.map((_, i) =>
              comDias.slice(MAX_SERIES - 1).reduce((s, c) => s + (valoresDe(c)[i] ?? 0), 0),
            ),
          },
        ];
  const temValor = r.total_por_mes_centavos.some((v) => v > 0);

  return (
    <Cartao>
      <TituloCartao>Quanto devo nos cartões</TituloCartao>
      <dl className="grid grid-cols-2 gap-3">
        <div>
          <dt className="text-xs text-texto-2">Total a pagar</dt>
          <dd className="valor num text-2xl font-bold">{formatarMoeda(r.total_devendo_centavos)}</dd>
        </div>
        <div>
          <dt className="text-xs text-texto-2">
            Vence em{" "}
            {nomeMes(r.meses[0] ?? "")
              .split(" ")[0]
              ?.toLowerCase()}
          </dt>
          <dd className="valor num text-2xl font-bold">{formatarMoeda(r.total_por_mes_centavos[0] ?? 0)}</dd>
        </div>
      </dl>

      <ul className="mt-4 flex flex-col divide-y divide-borda" aria-label="Dívida por cartão">
        {r.cartoes.map((c) => {
          const proxima = c.faturas.find((f) => !f.paga && f.valor_centavos > 0);
          return (
            <li key={c.id} className="flex min-h-14 items-center gap-2 py-2">
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{c.nome}</span>
                <span className="block truncate text-xs text-texto-2">
                  {c.sem_dias
                    ? "sem dias de fechamento e vencimento"
                    : proxima
                      ? `próxima: ${formatarData(proxima.vencimento)} · ${formatarMoeda(proxima.valor_centavos, c.moeda)}`
                      : "nada a pagar"}
                </span>
              </span>
              <span className="valor num shrink-0 font-semibold">
                {formatarMoeda(c.devendo_centavos, c.moeda)}
              </span>
              {c.sem_dias ? null : (
                <button
                  type="button"
                  onClick={() => setEditando(c)}
                  aria-label={`Informar valores das faturas do ${c.nome}`}
                  className="grid size-11 shrink-0 place-items-center rounded-xl text-texto-2 hover:bg-superficie-2"
                >
                  <Pencil className="size-4" aria-hidden />
                </button>
              )}
            </li>
          );
        })}
      </ul>

      {temValor ? (
        <div className="mt-4">
          <h3 className="mb-1 text-sm font-semibold">Por mês</h3>
          <GraficoCartoesMes meses={r.meses} series={series} />
          <ul className="mt-3 grid grid-cols-3 gap-2 sm:grid-cols-6" aria-label="Total por mês">
            {r.meses.map((m, i) => (
              <li key={m} className="rounded-xl bg-superficie-2 px-2 py-1.5 text-center">
                <span className="block text-xs text-texto-2">{nomeMes(m).split(" ")[0]}</span>
                <span className="valor num block text-sm font-semibold">
                  {formatarMoeda(r.total_por_mes_centavos[i] ?? 0)}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <p className="mt-3 text-sm text-texto-2">
          Toque no lápis de um cartão para informar quanto vem em cada fatura, ou lance as compras nele.
        </p>
      )}

      <Dialogo
        aberto={editando !== null}
        aoFechar={() => setEditando(null)}
        titulo={editando ? `Faturas do ${editando.nome}` : "Faturas"}
      >
        {editando ? <ValoresFaturas cartao={editando} aoFechar={() => setEditando(null)} /> : null}
      </Dialogo>
    </Cartao>
  );
}

const nomesStatusResumo: Record<FaturaResumo["status"], string> = {
  fechada: "fechada",
  aberta: "aberta",
  futura: "futura",
};

/** Digitar o total de cada fatura; a diferença para as compras lançadas vira um ajuste nela. */
function ValoresFaturas({ cartao, aoFechar }: { cartao: CartaoResumo; aoFechar: () => void }) {
  const cliente = useQueryClient();
  const inicial = Object.fromEntries(
    cartao.faturas.map((f) => [f.vencimento, f.valor_centavos ? centavosParaTexto(f.valor_centavos) : ""]),
  );
  const [valores, setValores] = useState<Record<string, string>>(inicial);
  const alterados = cartao.faturas.filter((f) => !f.paga && valores[f.vencimento] !== inicial[f.vencimento]);
  const invalido = alterados.some((f) => {
    const t = (valores[f.vencimento] ?? "").trim();
    const v = t === "" ? 0 : lerCentavos(t);
    return v === null || v < 0;
  });
  const salvar = useMutation({
    mutationFn: async () => {
      for (const f of alterados) {
        const t = (valores[f.vencimento] ?? "").trim();
        await api("PUT", `/api/contas/${cartao.id}/faturas/${f.vencimento}`, {
          valor_centavos: t === "" ? 0 : lerCentavos(t),
        });
      }
    },
    onSuccess: async () => {
      await Promise.all(
        ["cartoes-resumo", "faturas", "contas", "transacoes", "resumo", "agenda"].map((k) =>
          cliente.invalidateQueries({ queryKey: [k] }),
        ),
      );
      aoFechar();
    },
  });
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        salvar.mutate();
      }}
      className="flex flex-col gap-3"
    >
      <p className="text-sm text-texto-2">
        Quanto vem em cada fatura. Se você lança as compras, o painel guarda só a diferença como um ajuste.
      </p>
      {cartao.faturas.map((f) => (
        <Campo
          key={f.vencimento}
          rotulo={`Vence ${formatarData(f.vencimento)} (${f.paga ? "paga" : nomesStatusResumo[f.status]})`}
          inputMode="decimal"
          placeholder="0,00"
          disabled={f.paga}
          value={valores[f.vencimento] ?? ""}
          onChange={(e) => setValores((v) => ({ ...v, [f.vencimento]: e.target.value }))}
        />
      ))}
      {salvar.error ? <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso> : null}
      <div className="flex justify-end gap-2">
        <Botao variante="fantasma" onClick={aoFechar}>
          Cancelar
        </Botao>
        <Botao type="submit" carregando={salvar.isPending} disabled={!alterados.length || invalido}>
          Salvar
        </Botao>
      </div>
    </form>
  );
}
