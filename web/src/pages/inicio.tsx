import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowDownLeft,
  Building2,
  CalendarDays,
  CheckCircle2,
  Circle,
  CreditCard,
  Landmark,
  ReceiptText,
  Scale,
  ShieldAlert,
  Tags,
  Target,
  Upload,
  Users,
  Wallet,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { Link } from "react-router";
import { Barra, SeletorMes, Valor } from "../components/extras";
import { Aviso, Botao, Cartao, Esqueleto, Etiqueta, Pagina, TituloCartao, Vazio } from "../components/ui";
import { Cartoes, ContasBancarias, EvolucaoSaldo, Investimentos } from "../components/visao";
import { api, mensagemDe, obter } from "../lib/api";
import {
  gastoOrcado,
  mesAtual,
  nomeMes,
  useCarteira,
  useCasas,
  useEntidades,
  useIndicadores,
  useResumo,
} from "../lib/dados";
import { formatarData, formatarDataHora, formatarMoeda, formatarPercentual } from "../lib/formato";
import { useSessao } from "../lib/sessao";
import type { Alerta, Indicadores, Orcamento, Resumo } from "../lib/tipos";

/** Texto de cada tipo de alerta (e, quando há, a transação de origem). */
function textoAlerta(a: Alerta): { titulo: string; detalhe?: string; link?: string } {
  const d = a.dados as Record<string, unknown>;
  const desc = typeof d.descricao === "string" ? d.descricao : "";
  const moeda = (v: unknown) => (typeof v === "number" ? formatarMoeda(Math.abs(v)) : "");
  const data = typeof d.data === "string" ? formatarData(d.data) : "";
  const conta = typeof d.conta === "string" ? ` · ${d.conta}` : "";
  const link = desc ? `/transacoes?busca=${encodeURIComponent(desc)}` : undefined;
  switch (a.tipo) {
    case "login_aparelho_novo":
      return { titulo: "Login em aparelho novo", detalhe: typeof d.ip === "string" ? d.ip : undefined };
    case "recorrencia_subiu":
      return {
        titulo: `${desc} ficou mais cara`,
        detalhe: `de ${moeda(d.de_centavos)} para ${moeda(d.para_centavos)}`,
      };
    case "saldo_divergente":
      return {
        titulo: `Saldo de ${typeof d.conta === "string" ? d.conta : "uma conta"} diferente do banco`,
        detalhe: `banco ${moeda(d.banco_centavos)}, painel ${moeda(d.calculado_centavos)}`,
      };
    case "assinatura_detectada":
      return { titulo: `Nova cobrança recorrente: ${desc}`, detalhe: moeda(d.valor_centavos) };
    case "gasto_fora_do_padrao":
      return {
        titulo: `Gasto fora do padrão${typeof d.categoria === "string" ? ` em ${d.categoria}` : ""}: ${desc}`,
        detalhe: `${moeda(d.valor_centavos)} em ${data}; o normal é perto de ${moeda(d.referencia_centavos)}`,
        link,
      };
    case "cobranca_duplicada":
      return {
        titulo: `Possível cobrança duplicada: ${desc}`,
        detalhe: `${moeda(d.valor_centavos)} duas vezes até ${data}${conta}`,
        link,
      };
    case "tarifa_bancaria":
      return {
        titulo: `Tarifa bancária: ${desc}`,
        detalhe: `${moeda(d.valor_centavos)} em ${data}${conta}`,
        link,
      };
    case "relatorio_pronto": {
      const periodo = typeof d.periodo === "string" ? d.periodo : "";
      return {
        titulo:
          d.tipo === "mes"
            ? `Relatório de ${nomeMes(periodo).toLowerCase()} pronto`
            : `Resumo da semana de ${formatarData(periodo)} pronto`,
        link: typeof d.relatorio_id === "string" ? `/relatorios/${d.relatorio_id}` : "/relatorios",
      };
    }
    case "juros_iof":
      return {
        titulo: `Juros ou IOF: ${desc}`,
        detalhe: `${moeda(d.valor_centavos)} em ${data}${conta}`,
        link,
      };
    default:
      return { titulo: a.tipo };
  }
}

const temEntidade = (l?: { papel: string }[]) => !!l?.some((e) => e.papel === "dono");

function saudacao() {
  const h = Number(
    new Intl.DateTimeFormat("pt-BR", {
      hour: "numeric",
      hourCycle: "h23",
      timeZone: "America/Sao_Paulo",
    }).format(new Date()),
  );
  return h < 12 ? "Bom dia" : h < 18 ? "Boa tarde" : "Boa noite";
}

export function Inicio() {
  const { data: sessao } = useSessao();
  const [mes, setMes] = useState(mesAtual);
  const resumo = useResumo(mes);
  const entidades = useEntidades();
  const casas = useCasas();
  const alertas = useQuery({ queryKey: ["alertas"], queryFn: () => obter<Alerta[]>("/api/alertas") });
  const cliente = useQueryClient();
  const dispensar = useMutation({
    mutationFn: (id?: string) => api("POST", id ? `/api/alertas/${id}/lido` : "/api/alertas/lidos"),
    onSuccess: () => cliente.invalidateQueries({ queryKey: ["alertas"] }),
  });
  const indicadores = useIndicadores();
  const carteira = useCarteira();
  const orcamento = useQuery({
    queryKey: ["orcamento", mes, ""],
    queryFn: () => obter<Orcamento>(`/api/orcamento?mes=${mes}`),
    enabled: temEntidade(entidades.data),
  });
  const metas = useQuery({ queryKey: ["metas"], queryFn: () => obter<{ metas: unknown[] }>("/api/metas") });
  const r = resumo.data;
  const primeiroNome = sessao?.nome?.split(" ")[0];

  const temPF = !!entidades.data?.some((e) => e.tipo === "PF" && e.papel === "dono");
  const temContas = (r?.contas.length ?? 0) > 0;
  const temTransacoes = (r?.ultimas.length ?? 0) > 0;
  const categoriasOk = temTransacoes && r?.sem_categoria === 0;
  const passosFeitos =
    temPF && temContas && temTransacoes && categoriasOk && (metas.data?.metas.length ?? 0) > 0;

  return (
    <Pagina
      titulo={`${saudacao()}, ${primeiroNome ?? ""}`}
      subtitulo="Como estou este mês?"
      acao={<SeletorMes mes={mes} onChange={setMes} />}
    >
      {resumo.error ? <Aviso tipo="erro">{mensagemDe(resumo.error)}</Aviso> : null}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <ContasBancarias contas={r?.contas ?? []} carregando={resumo.isPending} />
        <Cartoes contas={r?.contas ?? []} carregando={resumo.isPending} />
        <Investimentos
          contas={r?.contas ?? []}
          carteira={carteira.data}
          carregando={resumo.isPending || carteira.isPending}
        />
      </div>

      <EvolucaoSaldo serie={indicadores.data?.patrimonio.serie ?? []} carregando={indicadores.isPending} />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <CartaoNumero
          titulo="Gasto do mês"
          icone={<ReceiptText />}
          carregando={resumo.isPending}
          valor={r?.gastos_centavos}
          cor="text-saida"
        >
          {r ? (
            <Comparacao atual={r.gastos_centavos} anterior={r.gastos_mes_anterior_mesmo_periodo_centavos} />
          ) : null}
        </CartaoNumero>
        <CartaoNumero
          titulo="Receitas do mês"
          icone={<ArrowDownLeft />}
          carregando={resumo.isPending}
          valor={r?.receitas_centavos}
          cor="text-entrada"
        >
          {r && r.receitas_centavos > 0 ? (
            <>
              Taxa de poupança:{" "}
              <strong className="valor num">
                {formatarPercentual((r.receitas_centavos - r.gastos_centavos) / r.receitas_centavos)}
              </strong>
            </>
          ) : (
            "Taxa de poupança aparece quando houver receitas."
          )}
        </CartaoNumero>
      </div>

      {r && r.sem_categoria > 0 ? (
        <Aviso tipo="alerta">
          {r.sem_categoria} {r.sem_categoria === 1 ? "transação está" : "transações estão"} sem categoria.{" "}
          <Link to="/gastos?sem_categoria=1" className="font-semibold underline">
            Revisar
          </Link>
        </Aviso>
      ) : null}

      <PainelIndicadores
        i={indicadores.data}
        carregando={indicadores.isPending}
        orcamento={orcamento.data}
        mes={mes}
      />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Cartao>
          <TituloCartao
            acao={
              <Link to={`/gastos?mes=${mes}`} className="text-sm font-semibold text-primaria">
                Ver tudo
              </Link>
            }
          >
            Para onde foi o dinheiro
          </TituloCartao>
          {resumo.isPending ? (
            <Esqueleto className="h-40 w-full" />
          ) : r?.gastos_por_categoria.length ? (
            <GastosPorCategoria itens={r.gastos_por_categoria} total={r.gastos_centavos} mes={mes} />
          ) : (
            <Vazio
              titulo="Sem gastos neste mês"
              texto="Importe um extrato para ver os gastos por categoria."
            />
          )}
        </Cartao>

        <Cartao>
          <TituloCartao
            acao={
              <Link to="/gastos" className="text-sm font-semibold text-primaria">
                Transações
              </Link>
            }
          >
            Últimas transações
          </TituloCartao>
          {resumo.isPending ? (
            <Esqueleto className="h-40 w-full" />
          ) : r?.ultimas.length ? (
            <ul className="flex flex-col divide-y divide-borda">
              {r.ultimas.map((t) => (
                <li key={t.id} className="flex min-h-12 items-center gap-3 py-1.5">
                  <span
                    className="size-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: t.cor ?? "var(--borda)" }}
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{t.descricao}</span>
                    <span className="block truncate text-xs text-texto-2">
                      {t.data.split("-").reverse().slice(0, 2).join("/")} · {t.categoria ?? "sem categoria"}
                    </span>
                  </span>
                  <Valor
                    centavos={t.valor_centavos}
                    moeda={t.moeda}
                    tipo={t.tipo}
                    className="text-sm font-medium"
                  />
                </li>
              ))}
            </ul>
          ) : (
            <Vazio
              icone={<Upload className="size-8" />}
              titulo="Nada por aqui ainda"
              texto="Importe o extrato do banco (OFX ou CSV)."
              acao={
                <Link to="/importar" className="text-sm font-semibold text-primaria">
                  Importar extrato
                </Link>
              }
            />
          )}
        </Cartao>
      </div>

      <div className="grid grid-cols-1 gap-4">
        <Cartao>
          <TituloCartao
            acao={
              (alertas.data?.length ?? 0) > 1 ? (
                <Botao
                  variante="fantasma"
                  onClick={() => dispensar.mutate(undefined)}
                  carregando={dispensar.isPending}
                >
                  Dispensar todos
                </Botao>
              ) : undefined
            }
          >
            Alertas
          </TituloCartao>
          {dispensar.error ? <Aviso tipo="erro">{mensagemDe(dispensar.error)}</Aviso> : null}
          {alertas.isPending ? (
            <Esqueleto className="h-12 w-full" />
          ) : alertas.data?.length ? (
            <ul className="flex flex-col divide-y divide-borda">
              {alertas.data.slice(0, 5).map((a) => {
                const t = textoAlerta(a);
                return (
                  <li key={a.id} className="flex items-start gap-3 py-2.5">
                    <ShieldAlert className="mt-0.5 size-5 shrink-0 text-alerta" aria-hidden />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium">
                        {t.link ? (
                          <Link to={t.link} className="hover:underline">
                            {t.titulo}
                          </Link>
                        ) : (
                          t.titulo
                        )}
                      </p>
                      <p className="truncate text-xs text-texto-2">
                        {formatarDataHora(a.criado_em)}
                        {t.detalhe ? ` · ${t.detalhe}` : ""}
                      </p>
                    </div>
                    <Botao
                      variante="fantasma"
                      aria-label={`Dispensar: ${t.titulo}`}
                      onClick={() => dispensar.mutate(a.id)}
                      disabled={dispensar.isPending}
                    >
                      <X className="size-4" />
                    </Botao>
                  </li>
                );
              })}
            </ul>
          ) : (
            <Vazio
              titulo="Nenhum alerta"
              texto="Gastos fora do padrão, cobranças duplicadas e logins novos aparecem aqui."
            />
          )}
        </Cartao>
      </div>

      {!passosFeitos ? (
        <Cartao>
          <TituloCartao>Primeiros passos</TituloCartao>
          <ol className="flex flex-col gap-1">
            <Passo feito={temPF} para="/entidades" icone={<Users className="size-4" />}>
              Criar sua entidade pessoa física
            </Passo>
            <Passo feito={temContas} para="/contas" icone={<Landmark className="size-4" />}>
              Cadastrar suas contas e cartões
            </Passo>
            <Passo feito={temTransacoes} para="/importar" icone={<Upload className="size-4" />}>
              Importar o primeiro extrato
            </Passo>
            <Passo feito={categoriasOk} para="/gastos?sem_categoria=1" icone={<Tags className="size-4" />}>
              Revisar categorias
            </Passo>
            <Passo
              feito={(metas.data?.metas.length ?? 0) > 0}
              para="/metas"
              icone={<Target className="size-4" />}
            >
              Definir uma meta
            </Passo>
          </ol>
        </Cartao>
      ) : null}

      {casas.data && casas.data.length === 0 ? (
        <Cartao>
          <Vazio
            icone={<Building2 className="size-8" />}
            titulo="Você ainda não está em uma casa"
            texto="Crie uma casa para compartilhar contas e metas com quem mora com você."
            acao={
              <Link to="/casa" className="text-sm font-semibold text-primaria">
                Criar casa
              </Link>
            }
          />
        </Cartao>
      ) : null}
    </Pagina>
  );
}

/** Número do mês no mesmo padrão dos blocos: ícone e rótulo em maiúsculas, valor, nota no pé. */
function CartaoNumero({
  titulo,
  icone,
  valor,
  cor,
  carregando,
  children,
}: {
  titulo: string;
  icone: ReactNode;
  valor?: number;
  cor: string;
  carregando: boolean;
  pequeno?: boolean;
  children?: ReactNode;
}) {
  return (
    <section className="flex h-full flex-col rounded-cartao border border-borda bg-superficie p-5">
      <h2 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-texto-2">
        <span className="text-destaque [&>svg]:size-4" aria-hidden>
          {icone}
        </span>
        {titulo}
      </h2>
      {carregando ? (
        <Esqueleto className="mt-3 h-8 w-36" />
      ) : (
        <p className={`valor num mt-2 text-2xl font-bold tracking-tight ${cor}`}>
          {formatarMoeda(valor ?? 0)}
        </p>
      )}
      <p className="mt-auto pt-2 text-xs text-texto-2">{children}</p>
    </section>
  );
}

function Comparacao({ atual, anterior }: { atual: number; anterior: number }) {
  if (anterior === 0) return <>Sem gastos no mesmo período do mês passado.</>;
  const variacao = (atual - anterior) / anterior;
  return (
    <>
      <span className={`valor num font-semibold ${variacao > 0 ? "text-saida" : "text-entrada"}`}>
        {variacao > 0 ? "▲" : "▼"} {formatarPercentual(Math.abs(variacao), 0)}
      </span>{" "}
      em relação ao mesmo período do mês passado (<span className="valor num">{formatarMoeda(anterior)}</span>
      )
    </>
  );
}

/** Barras ordenadas (nada de pizza com mais de 5 fatias, docs/SPEC.md §7). */
function GastosPorCategoria({
  itens,
  total,
  mes,
}: {
  itens: Resumo["gastos_por_categoria"];
  total: number;
  mes: string;
}) {
  const principais = itens.slice(0, 6);
  const resto = itens.slice(6).reduce((s, i) => s + i.total_centavos, 0);
  const lista =
    resto > 0
      ? [...principais, { categoria_id: null, nome: "Outras", cor: null, total_centavos: resto }]
      : principais;
  const maior = Math.max(...lista.map((i) => i.total_centavos), 1);
  return (
    <ul className="flex flex-col gap-3">
      {lista.map((i) => (
        <li key={i.categoria_id ?? i.nome}>
          <Link
            to={
              i.categoria_id
                ? `/gastos?mes=${mes}&categoria=${i.categoria_id}`
                : i.nome === "Sem categoria"
                  ? `/gastos?mes=${mes}&sem_categoria=1`
                  : `/gastos?mes=${mes}`
            }
            className="block rounded-lg hover:bg-superficie-2"
          >
            <span className="flex items-baseline justify-between gap-3 text-sm">
              <span className="truncate">{i.nome}</span>
              <span className="valor num shrink-0 font-medium">
                {formatarMoeda(i.total_centavos)}
                <span className="ml-1.5 text-xs text-texto-2">
                  {total > 0 ? formatarPercentual(i.total_centavos / total, 0) : ""}
                </span>
              </span>
            </span>
            <span className="mt-1 block h-2 rounded-full bg-superficie-2">
              <span
                className="block h-2 rounded-full"
                style={{
                  width: `${Math.max(2, (i.total_centavos / maior) * 100)}%`,
                  backgroundColor: i.cor ?? "var(--texto-2)",
                }}
              />
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

function Passo({
  feito,
  para,
  icone,
  fase,
  children,
}: {
  feito: boolean;
  para?: string;
  icone: ReactNode;
  fase?: string;
  children: ReactNode;
}) {
  const conteudo = (
    <span className="flex min-h-11 items-center gap-3">
      {feito ? (
        <CheckCircle2 className="size-5 text-entrada" aria-label="feito" />
      ) : (
        <Circle className="size-5 text-texto-2" aria-label="pendente" />
      )}
      <span className="text-texto-2">{icone}</span>
      <span className={feito ? "text-texto-2 line-through" : ""}>{children}</span>
      {fase ? <Etiqueta>{fase}</Etiqueta> : null}
    </span>
  );
  return (
    <li>
      {para && !feito ? (
        <Link to={para} className="block rounded-lg hover:bg-superficie-2">
          {conteudo}
        </Link>
      ) : (
        conteudo
      )}
    </li>
  );
}

/** Os números que o painel sempre mostra (SPEC §2). */
function PainelIndicadores({
  i,
  carregando,
  orcamento: o,
  mes,
}: {
  i?: Indicadores;
  carregando: boolean;
  orcamento?: Orcamento;
  mes: string;
}) {
  const custo = i?.custo_de_vida.medio_6_meses_centavos ?? 0;
  const proximo = i?.comprometido[1];
  const t = o?.totais;
  const gasto = gastoOrcado(o?.itens ?? []);
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Link to="/patrimonio" className="block h-full rounded-cartao hover:brightness-110">
        <CartaoNumero
          titulo="Patrimônio líquido"
          icone={<Scale />}
          carregando={carregando}
          valor={i?.patrimonio.hoje.liquido_centavos}
          cor={(i?.patrimonio.hoje.liquido_centavos ?? 0) < 0 ? "text-saida" : "text-texto"}
          pequeno
        >
          {i ? (
            <>
              <span
                className={`valor num font-semibold ${i.patrimonio.variacao_mes_centavos < 0 ? "text-saida" : "text-entrada"}`}
              >
                {i.patrimonio.variacao_mes_centavos >= 0 ? "+" : ""}
                {formatarMoeda(i.patrimonio.variacao_mes_centavos)}
              </span>{" "}
              no mês
            </>
          ) : null}
        </CartaoNumero>
      </Link>
      <Link to="/metas" className="block h-full rounded-cartao hover:brightness-110">
        <CartaoNumero
          titulo="Custo de vida"
          icone={<Wallet />}
          carregando={carregando}
          valor={custo}
          cor="text-texto"
          pequeno
        >
          Média dos últimos 6 meses.{" "}
          {i?.meses_de_reserva != null ? (
            <>
              Reserva cobre{" "}
              <strong className="valor num">
                {i.meses_de_reserva.toLocaleString("pt-BR", { maximumFractionDigits: 1 })} meses
              </strong>
            </>
          ) : custo === 0 ? (
            "Aparece depois do primeiro mês fechado."
          ) : (
            "Crie uma meta de reserva para ver quantos meses ela cobre."
          )}
        </CartaoNumero>
      </Link>
      <Link to="/agenda" className="block h-full rounded-cartao hover:brightness-110">
        <CartaoNumero
          titulo="Saldo em 30 dias"
          icone={<CalendarDays />}
          carregando={carregando}
          valor={i?.saldo_projetado_30_dias_centavos}
          cor={(i?.saldo_projetado_30_dias_centavos ?? 0) < 0 ? "text-saida" : "text-texto"}
          pequeno
        >
          {i ? (
            <>
              Menor saldo:{" "}
              <span
                className={`valor num font-semibold ${i.menor_saldo_30_dias.saldo_centavos < 0 ? "text-saida" : ""}`}
              >
                {formatarMoeda(i.menor_saldo_30_dias.saldo_centavos)}
              </span>{" "}
              em {formatarData(i.menor_saldo_30_dias.data)}
            </>
          ) : null}
        </CartaoNumero>
      </Link>
      <Link to="/cartoes" className="block h-full rounded-cartao hover:brightness-110">
        <CartaoNumero
          icone={<CreditCard />}
          titulo={
            proximo ? `Comprometido em ${nomeMes(proximo.mes).split(" ")[0]?.toLowerCase()}` : "Comprometido"
          }
          carregando={carregando}
          valor={proximo?.total_centavos}
          cor="text-texto"
          pequeno
        >
          {proximo ? "Parcelas, financiamentos e contas fixas já assumidos." : null}
        </CartaoNumero>
      </Link>
      {t && t.disponivel_centavos > 0 && o ? (
        <Link
          to="/orcamento"
          className="block rounded-cartao hover:brightness-110 sm:col-span-2 lg:col-span-4"
        >
          <Cartao>
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <p className="text-sm text-texto-2">Categorias orçadas em {nomeMes(mes).toLowerCase()}</p>
              <p className="valor num text-sm">
                <strong>{formatarMoeda(gasto)}</strong>
                <span className="text-texto-2"> / {formatarMoeda(t.disponivel_centavos)}</span>
              </p>
            </div>
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
              rotulo="Gasto do mês em relação ao orçamento"
            />
          </Cartao>
        </Link>
      ) : null}
    </div>
  );
}
