import { Briefcase, ChevronDown, LineChart, Plug, TrendingUp } from "lucide-react";
import { Link } from "react-router";
import { AvisoSimulacao, Barra } from "../components/extras";
import { Aviso, CarregandoLista, Etiqueta, Pagina, Vazio } from "../components/ui";
import { Bloco } from "../components/visao";
import { mensagemDe } from "../lib/api";
import { useCarteira } from "../lib/dados";
import { formatarData, formatarMoeda, formatarPercentual } from "../lib/formato";
import { type AtivoCarteira, nomesClasse } from "../lib/tipos";

export function Investimentos() {
  const dados = useCarteira();
  const c = dados.data;
  const ativos = (c?.ativos ?? []).filter((a) => !(a.encerrado && a.valor_centavos === 0));
  const encerrados = (c?.ativos ?? []).filter((a) => a.encerrado && a.valor_centavos === 0);
  const porClasse = new Map<string, AtivoCarteira[]>();
  for (const a of ativos) porClasse.set(a.classe, [...(porClasse.get(a.classe) ?? []), a]);
  const rendimento = (c?.total_centavos ?? 0) - (c?.custo_centavos ?? 0);

  return (
    <Pagina titulo="Investimentos" subtitulo="Minha carteira está indo bem?">
      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Bloco
          icone={<TrendingUp className="size-4" />}
          rotulo="Carteira"
          valor={c?.total_centavos}
          cor="text-entrada"
          carregando={dados.isPending}
          sub={
            c
              ? `${c.classes.length} ${c.classes.length === 1 ? "classe" : "classes"} · ${ativos.length} ${ativos.length === 1 ? "ativo" : "ativos"}${
                  c.encerrados ? ` (${c.encerrados} encerrados)` : ""
                }`
              : undefined
          }
        >
          {c?.classes.length ? (
            <ul className="flex flex-col gap-4 p-5">
              {c.classes.map((cl) => (
                <li key={cl.classe}>
                  <div className="flex items-baseline justify-between gap-3 text-sm">
                    <span>
                      {nomesClasse[cl.classe] ?? cl.classe}{" "}
                      <span className="text-texto-2">({cl.ativos})</span>
                    </span>
                    <span className="valor num">
                      <span className="text-texto-2">{formatarPercentual(cl.percentual)}</span>{" "}
                      {formatarMoeda(cl.valor_centavos)}
                    </span>
                  </div>
                  <Barra
                    fracao={cl.percentual}
                    cor="var(--destaque)"
                    rotulo={nomesClasse[cl.classe] ?? cl.classe}
                  />
                </li>
              ))}
            </ul>
          ) : null}
        </Bloco>
        <Bloco
          icone={<LineChart className="size-4" />}
          rotulo="Rendimento"
          valor={rendimento}
          cor={rendimento < 0 ? "text-saida" : "text-entrada"}
          carregando={dados.isPending}
          sub={
            c && c.custo_centavos > 0
              ? `${formatarPercentual(rendimento / c.custo_centavos)} sobre ${formatarMoeda(c.custo_centavos)} aplicados`
              : "Valor de hoje menos o que foi aplicado"
          }
        >
          <p className="p-5 text-sm text-texto-2">
            Rentabilidade contra CDI, IPCA e Ibovespa, proventos e o IR mensal chegam com as cotações e a
            importação da B3. Os investimentos do Open Finance usam o valor que o banco informa.
          </p>
        </Bloco>
      </div>

      <section className="overflow-hidden rounded-cartao border border-borda bg-superficie">
        <header className="flex items-center justify-between gap-3 p-5">
          <h2 className="flex items-center gap-2 font-semibold">
            <Briefcase className="size-4 text-destaque" aria-hidden /> Carteira ({ativos.length}{" "}
            {ativos.length === 1 ? "ativo" : "ativos"})
          </h2>
          {c ? (
            <span className="valor num text-lg font-bold text-entrada">
              {formatarMoeda(c.total_centavos)}
            </span>
          ) : null}
        </header>
        {dados.isPending ? (
          <div className="p-5">
            <CarregandoLista />
          </div>
        ) : ativos.length === 0 ? (
          <Vazio
            icone={<Plug className="size-8" />}
            titulo="Nenhum investimento ainda"
            texto="Conecte o banco pelo Open Finance: os investimentos (CDB, Tesouro, fundos) entram sozinhos."
            acao={
              <Link to="/open-finance" className="text-sm font-semibold text-primaria">
                Abrir Open Finance
              </Link>
            }
          />
        ) : (
          [...porClasse.entries()].map(([classe, lista]) => (
            <div key={classe}>
              <div className="flex justify-between border-y border-borda bg-fundo/40 px-5 py-2 text-xs font-semibold uppercase tracking-wider text-texto-2">
                <span>{nomesClasse[classe] ?? classe}</span>
                <span className="valor num">
                  {formatarMoeda(lista.reduce((s, a) => s + a.valor_centavos, 0))}
                </span>
              </div>
              <ul className="divide-y divide-borda">
                {lista.map((a) => (
                  <LinhaAtivo key={a.id} a={a} />
                ))}
              </ul>
            </div>
          ))
        )}
        {encerrados.length ? (
          <details className="border-t border-borda">
            <summary className="flex min-h-12 cursor-pointer items-center gap-2 px-5 text-sm text-texto-2">
              <ChevronDown className="size-4" aria-hidden /> {encerrados.length} encerrados (resgatados)
            </summary>
            <ul className="divide-y divide-borda">
              {encerrados.map((a) => (
                <LinhaAtivo key={a.id} a={a} />
              ))}
            </ul>
          </details>
        ) : null}
      </section>
      <AvisoSimulacao />
    </Pagina>
  );
}

function LinhaAtivo({ a }: { a: AtivoCarteira }) {
  const detalhe = [a.instituicao, a.subtipo, a.vencimento ? `vence ${formatarData(a.vencimento)}` : null]
    .filter(Boolean)
    .join(" · ");
  return (
    <li className="flex min-h-16 items-center gap-3 px-5 py-2.5">
      <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-superficie-2 text-texto-2">
        <TrendingUp className="size-4" aria-hidden />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium">{a.nome}</span>
        <span className="flex flex-wrap items-center gap-1 text-xs text-texto-2">
          <span className="truncate">{detalhe}</span>
          {a.isento_ir ? <Etiqueta cor="entrada">isento de IR</Etiqueta> : null}
        </span>
      </span>
      <span className="shrink-0 text-right">
        <span className="valor num block font-semibold text-entrada">{formatarMoeda(a.valor_centavos)}</span>
        <span className="valor num block text-xs text-texto-2">
          {a.custo_centavos > 0 && a.valor_centavos > 0
            ? `${a.rendimento_centavos >= 0 ? "+" : ""}${formatarPercentual(a.rendimento_centavos / a.custo_centavos)}`
            : formatarPercentual(a.percentual)}
        </span>
      </span>
    </li>
  );
}
