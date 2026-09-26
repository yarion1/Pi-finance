import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarRange, FileText } from "lucide-react";
import { Link, useParams } from "react-router";
import { AvisoSimulacao, Barra } from "../components/extras";
import { Aviso, Botao, CarregandoLista, Cartao, Pagina, TituloCartao, Vazio } from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { nomeMes } from "../lib/dados";
import { formatarData, formatarMoeda, formatarPercentual } from "../lib/formato";
import type { PontoPatrimonio, Relatorio, RelatorioMes, RelatorioResumo, ResumoSemana } from "../lib/tipos";

/** "Agosto de 2026" ou "Semana de 14/09". */
export function tituloRelatorio(r: Pick<RelatorioResumo, "tipo" | "periodo">): string {
  return r.tipo === "mes"
    ? `Relatório de ${nomeMes(r.periodo).toLowerCase()}`
    : `Resumo da semana de ${formatarData(r.periodo)}`;
}

export function Relatorios() {
  const lista = useQuery({
    queryKey: ["relatorios"],
    queryFn: () => obter<RelatorioResumo[]>("/api/relatorios"),
  });
  const cliente = useQueryClient();
  const gerar = useMutation({
    mutationFn: () => api<{ novos: string[] }>("POST", "/api/relatorios/gerar"),
    onSuccess: () => {
      cliente.invalidateQueries({ queryKey: ["relatorios"] });
      cliente.invalidateQueries({ queryKey: ["alertas"] });
    },
  });
  return (
    <Pagina
      titulo="Relatórios"
      subtitulo="O relatório do mês sai no dia 1; o resumo da semana, domingo à noite."
      acao={
        <Botao variante="secundario" onClick={() => gerar.mutate()} carregando={gerar.isPending}>
          Gerar os que faltam
        </Botao>
      }
    >
      {(lista.error ?? gerar.error) ? (
        <Aviso tipo="erro">{mensagemDe(lista.error ?? gerar.error)}</Aviso>
      ) : null}
      {gerar.data ? (
        <Aviso tipo="info">
          {gerar.data.novos.length
            ? `${gerar.data.novos.length} ${gerar.data.novos.length === 1 ? "relatório novo" : "relatórios novos"}.`
            : "Nada novo: os relatórios do último mês e da última semana já existem ou não houve movimento."}
        </Aviso>
      ) : null}
      <Cartao>
        {lista.isPending ? (
          <CarregandoLista />
        ) : lista.data?.length ? (
          <ul className="flex flex-col divide-y divide-borda" aria-label="Relatórios">
            {lista.data.map((r) => (
              <li key={r.id}>
                <Link to={`/relatorios/${r.id}`} className="flex min-h-14 items-center gap-3 py-2">
                  {r.tipo === "mes" ? (
                    <FileText className="size-5 shrink-0 text-primaria" aria-hidden />
                  ) : (
                    <CalendarRange className="size-5 shrink-0 text-texto-2" aria-hidden />
                  )}
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium">{tituloRelatorio(r)}</span>
                    <span className="block text-xs text-texto-2">
                      gerado em {formatarData(r.criado_em.slice(0, 10))}
                      {r.texto ? " · com resumo da IA" : ""}
                    </span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <Vazio
            icone={<FileText className="size-8" />}
            titulo="Nenhum relatório ainda"
            texto="Assim que um mês fechar (ou uma semana acabar) com movimento, o relatório aparece aqui e no início."
          />
        )}
      </Cartao>
    </Pagina>
  );
}

export function DetalheRelatorio() {
  const { id = "" } = useParams();
  const rel = useQuery({
    queryKey: ["relatorios", id],
    queryFn: () => obter<Relatorio>(`/api/relatorios/${id}`),
  });
  const r = rel.data;
  return (
    <Pagina
      titulo={r ? tituloRelatorio(r) : "Relatório"}
      acao={
        <Link to="/relatorios" className="text-sm font-semibold text-primaria">
          Todos os relatórios
        </Link>
      }
    >
      {rel.error ? <Aviso tipo="erro">{mensagemDe(rel.error)}</Aviso> : null}
      {rel.isPending ? (
        <Cartao>
          <CarregandoLista />
        </Cartao>
      ) : r?.tipo === "mes" ? (
        <DoMes dados={r.dados} texto={r.texto} />
      ) : r?.tipo === "semana" ? (
        <DaSemana dados={r.dados} />
      ) : null}
      <AvisoSimulacao />
    </Pagina>
  );
}

function Variacao({ atual, anterior, inverter }: { atual: number; anterior: number; inverter?: boolean }) {
  if (!anterior) return null;
  const v = (atual - anterior) / Math.abs(anterior);
  const ruim = inverter ? v > 0 : v < 0;
  return (
    <span className={`text-xs ${ruim ? "text-saida" : "text-entrada"}`}>
      {v >= 0 ? "+" : ""}
      {formatarPercentual(v, 0)} contra o mês anterior
    </span>
  );
}

function DoMes({ dados: d, texto }: { dados: RelatorioMes; texto: string | null }) {
  const maior = Math.max(1, ...d.categorias.map((c) => Math.max(c.total_centavos, c.media_3_meses_centavos)));
  return (
    <>
      <Cartao>
        <dl className="grid grid-cols-3 gap-3">
          <div>
            <dt className="text-xs text-texto-2">Receitas</dt>
            <dd className="valor num text-base font-semibold">{formatarMoeda(d.totais.receitas_centavos)}</dd>
            <dd>
              <Variacao atual={d.totais.receitas_centavos} anterior={d.anterior.receitas_centavos} />
            </dd>
          </div>
          <div>
            <dt className="text-xs text-texto-2">Gastos</dt>
            <dd className="valor num text-base font-semibold">{formatarMoeda(d.totais.gastos_centavos)}</dd>
            <dd>
              <Variacao atual={d.totais.gastos_centavos} anterior={d.anterior.gastos_centavos} inverter />
            </dd>
          </div>
          <div>
            <dt className="text-xs text-texto-2">Sobrou</dt>
            <dd
              className={`valor num text-base font-semibold ${d.totais.saldo_centavos < 0 ? "text-saida" : "text-entrada"}`}
            >
              {formatarMoeda(d.totais.saldo_centavos)}
            </dd>
          </div>
        </dl>
        {texto ? <p className="mt-4 whitespace-pre-line text-sm leading-relaxed">{texto}</p> : null}
      </Cartao>

      <Cartao>
        <TituloCartao>O que fazer agora</TituloCartao>
        {d.acoes.length ? (
          <ol className="flex list-decimal flex-col gap-2 pl-5 text-sm" aria-label="Ações sugeridas">
            {d.acoes.map((a) => (
              <li key={a}>{a}</li>
            ))}
          </ol>
        ) : (
          <p className="text-sm text-texto-2">Nada que peça atenção: mês tranquilo.</p>
        )}
      </Cartao>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Cartao>
          <TituloCartao>Gastos por categoria</TituloCartao>
          <ul className="flex flex-col gap-3" aria-label="Gastos por categoria contra a média">
            {d.categorias.map((c) => (
              <li key={c.nome}>
                <div className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="truncate">{c.nome}</span>
                  <span className="valor num">{formatarMoeda(c.total_centavos)}</span>
                </div>
                <Barra
                  fracao={c.total_centavos / maior}
                  marca={c.media_3_meses_centavos / maior}
                  cor={c.cor ?? "var(--primaria)"}
                  rotulo={`${c.nome}: ${formatarMoeda(c.total_centavos)}; média ${formatarMoeda(c.media_3_meses_centavos)}`}
                />
                {c.media_3_meses_centavos ? (
                  <p className="mt-0.5 text-xs text-texto-2">
                    média de 3 meses: {formatarMoeda(c.media_3_meses_centavos)}
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        </Cartao>
        <Cartao>
          <TituloCartao>Gasto acumulado no mês</TituloCartao>
          <Acumulado pontos={d.acumulado} />
          <p className="mt-2 flex gap-4 text-xs text-texto-2">
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-4 bg-primaria" aria-hidden /> este mês
            </span>
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-0.5 w-4 bg-texto-2" aria-hidden /> mês anterior
            </span>
          </p>
        </Cartao>
      </div>

      {d.patrimonio?.length ? (
        <Cartao>
          <TituloCartao>Patrimônio líquido</TituloCartao>
          <BarrasPatrimonio pontos={d.patrimonio} />
        </Cartao>
      ) : null}
    </>
  );
}

function Acumulado({ pontos }: { pontos: RelatorioMes["acumulado"] }) {
  if (pontos.length < 2) return null;
  const max = Math.max(1, ...pontos.flatMap((p) => [p.mes_centavos, p.anterior_centavos]));
  const linha = (f: (p: RelatorioMes["acumulado"][number]) => number) =>
    pontos.map((p, i) => `${(i / (pontos.length - 1)) * 300},${100 - (f(p) / max) * 100}`).join(" ");
  const ultimo = pontos[pontos.length - 1];
  return (
    <svg
      viewBox="0 0 300 100"
      preserveAspectRatio="none"
      className="h-36 w-full"
      role="img"
      aria-label={`Gasto acumulado: ${formatarMoeda(ultimo?.mes_centavos ?? 0)} no mês, ${formatarMoeda(ultimo?.anterior_centavos ?? 0)} no anterior`}
    >
      <polyline
        points={linha((p) => p.anterior_centavos)}
        fill="none"
        className="stroke-texto-2"
        strokeWidth="1"
        strokeDasharray="4 3"
        vectorEffect="non-scaling-stroke"
      />
      <polyline
        points={linha((p) => p.mes_centavos)}
        fill="none"
        className="stroke-primaria"
        strokeWidth="2"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}

function BarrasPatrimonio({ pontos }: { pontos: PontoPatrimonio[] }) {
  const maior = Math.max(...pontos.map((p) => Math.abs(p.liquido_centavos)), 1);
  return (
    <ol className="flex h-32 items-end gap-2" aria-label="Patrimônio líquido por mês">
      {pontos.map((p) => (
        <li key={p.data} className="flex h-full min-w-0 flex-1 flex-col justify-end">
          <span
            className={`block rounded-t ${p.liquido_centavos < 0 ? "bg-saida" : "bg-invest"}`}
            style={{ height: `${Math.max(2, (Math.abs(p.liquido_centavos) / maior) * 100)}%` }}
          />
          <span className="mt-1 truncate text-center text-[10px] text-texto-2">
            {formatarData(p.data).slice(3)}
          </span>
          <span className="sr-only">{formatarMoeda(p.liquido_centavos)}</span>
        </li>
      ))}
    </ol>
  );
}

function DaSemana({ dados: d }: { dados: ResumoSemana }) {
  return (
    <>
      <Cartao>
        <p className="text-sm text-texto-2">
          De {formatarData(d.inicio)} a {formatarData(d.fim)}
        </p>
        <p className="valor num mt-1 text-2xl font-bold">{formatarMoeda(d.gasto_centavos)}</p>
        <p className="text-sm text-texto-2">
          gastos na semana; na anterior, {formatarMoeda(d.semana_anterior_centavos)}
        </p>
        {d.categorias.length ? (
          <ul className="mt-3 flex flex-col gap-1 text-sm" aria-label="Onde mais gastou">
            {d.categorias.map((c) => (
              <li key={c.nome} className="flex justify-between gap-3">
                <span className="truncate">{c.nome}</span>
                <span className="valor num">{formatarMoeda(c.total_centavos)}</span>
              </li>
            ))}
          </ul>
        ) : null}
      </Cartao>
      {d.orcamento ? (
        <Cartao>
          <TituloCartao>Orçamento de {nomeMes(d.orcamento.mes).toLowerCase()}</TituloCartao>
          <p className="text-sm">
            Restam{" "}
            <span className="valor num font-semibold">
              {formatarMoeda(Math.max(d.orcamento.disponivel_centavos - d.orcamento.gasto_centavos, 0))}
            </span>{" "}
            de {formatarMoeda(d.orcamento.disponivel_centavos)}.
          </p>
          <Barra
            fracao={d.orcamento.gasto_centavos / d.orcamento.disponivel_centavos}
            cor={
              d.orcamento.gasto_centavos > d.orcamento.disponivel_centavos
                ? "var(--saida)"
                : "var(--primaria)"
            }
            rotulo="Orçamento usado"
          />
        </Cartao>
      ) : null}
      <Cartao>
        <TituloCartao>Próximos 7 dias</TituloCartao>
        {d.proximas.length ? (
          <ul className="flex flex-col divide-y divide-borda" aria-label="Contas dos próximos 7 dias">
            {d.proximas.map((it) => (
              <li
                key={`${it.origem}-${it.id}-${it.data}`}
                className="flex min-h-12 items-center gap-3 py-2 text-sm"
              >
                <span className="w-12 shrink-0 text-xs text-texto-2">
                  {formatarData(it.data).slice(0, 5)}
                </span>
                <span className="min-w-0 flex-1 truncate">{it.descricao}</span>
                <span className={`valor num ${it.valor_centavos < 0 ? "" : "text-entrada"}`}>
                  {formatarMoeda(it.valor_centavos)}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-texto-2">Nenhuma conta prevista.</p>
        )}
      </Cartao>
    </>
  );
}
