import { useQuery } from "@tanstack/react-query";
import { CreditCard } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { Barra, Valor } from "../components/extras";
import { Aviso, CarregandoLista, Cartao, Etiqueta, Pagina, TituloCartao, Vazio } from "../components/ui";
import { mensagemDe, obter } from "../lib/api";
import { nomeMes, useContas } from "../lib/dados";
import { formatarData, formatarMoeda } from "../lib/formato";
import type { Conta, Fatura, ParcelaFutura, RespostaFaturas } from "../lib/tipos";

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
  // o saldo do cartão é negativo quando há compras a pagar
  const usado = Math.max(-(cartao.saldo_centavos ?? 0), 0);
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
              <span className="text-texto-2">Limite usado</span>
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
              <ParcelasPorMes parcelas={dados.data.parcelas_futuras} moeda={cartao.moeda} />
            ) : (
              <Vazio titulo="Nenhuma parcela a vencer" texto="Compras parceladas aparecem aqui mês a mês." />
            )}
          </Cartao>
        </div>
      ) : null}
    </>
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
