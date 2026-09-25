// Blocos da visão geral no estilo do Meu Pluggy: cabeçalho com ícone e rótulo em
// maiúsculas, número grande, lista por banco, barra do limite e gráfico de área.
import { CreditCard, Landmark, TrendingUp, Wallet } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { formatarMoeda, formatarPercentual } from "../lib/formato";
import { type Conta, nomesTipoConta, type PontoPatrimonio } from "../lib/tipos";
import { Barra } from "./extras";
import { Esqueleto } from "./ui";

/** Cartão com cabeçalho separado por uma linha, como na referência. */
export function Bloco({
  icone,
  rotulo,
  valor,
  cor = "text-texto",
  sub,
  carregando,
  acao,
  children,
  className = "",
}: {
  icone: ReactNode;
  rotulo: string;
  valor?: number;
  cor?: string;
  sub?: ReactNode;
  carregando?: boolean;
  acao?: ReactNode;
  children?: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={`flex flex-col overflow-hidden rounded-cartao border border-borda bg-superficie ${className}`}
    >
      <div className="p-5 pb-4">
        <div className="flex items-center justify-between gap-3">
          <h2 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-texto-2">
            <span className="text-destaque">{icone}</span>
            {rotulo}
          </h2>
          {acao}
        </div>
        {carregando ? (
          <Esqueleto className="mt-3 h-9 w-40" />
        ) : valor !== undefined ? (
          <p className={`valor num mt-2 text-3xl font-bold tracking-tight ${cor}`}>{formatarMoeda(valor)}</p>
        ) : null}
        {sub ? <p className="mt-1 text-sm text-texto-2">{sub}</p> : null}
      </div>
      {children ? <div className="flex-1 border-t border-borda">{children}</div> : null}
    </section>
  );
}

// Cores de marca dos bancos mais comuns (o resto fica neutro).
const marcas: [RegExp, string, string, string][] = [
  [/nubank|nu pagamentos/i, "#820ad1", "#ffffff", "nu"],
  [/\binter\b|banco inter/i, "#ff7a00", "#ffffff", "in"],
  [/ita[uú]/i, "#ec7000", "#ffffff", "it"],
  [/picpay/i, "#21c25e", "#ffffff", "pp"],
  [/mercado ?pago/i, "#00b1ea", "#ffffff", "mp"],
  [/\bc6\b/i, "#242424", "#ffffff", "c6"],
  [/bradesco/i, "#cc092f", "#ffffff", "br"],
  [/santander/i, "#ec0000", "#ffffff", "sa"],
  [/banco do brasil|\bbb\b/i, "#fcfc30", "#0038a8", "bb"],
  [/caixa/i, "#005ca9", "#ffffff", "cx"],
  [/btg/i, "#0b2a4a", "#ffffff", "bt"],
  [/\bxp\b/i, "#111111", "#f5f5f5", "xp"],
  [/neon/i, "#00a5ff", "#ffffff", "ne"],
  [/pagbank|pagseguro/i, "#ffc801", "#111111", "pb"],
  [/sicoob/i, "#003641", "#ffffff", "sc"],
  [/sicredi/i, "#3fa110", "#ffffff", "si"],
];

/** Nome do banco da conta: a instituição cadastrada ou, sem ela, o nome da conta. */
export function nomeDoBanco(c: Conta): string {
  if (c.instituicao) return c.instituicao;
  return /nu pagamentos/i.test(c.nome) ? "Nubank" : c.nome;
}

export function IconeBanco({ nome, tamanho = "size-9" }: { nome: string; tamanho?: string }) {
  const marca = marcas.find(([re]) => re.test(nome));
  const [, fundo, texto, sigla] = marca ?? [null, "var(--superficie-2)", "var(--texto-2)", nome.slice(0, 2)];
  return (
    <span
      className={`flex ${tamanho} shrink-0 items-center justify-center rounded-xl text-xs font-bold lowercase`}
      style={{ backgroundColor: fundo, color: texto }}
      aria-hidden
    >
      {sigla}
    </span>
  );
}

const bancarias: Conta["tipo"][] = ["corrente", "poupanca", "carteira", "dinheiro", "beneficio"];

/** Contas bancárias agrupadas por banco, com a participação de cada um. */
export function ContasBancarias({ contas, carregando }: { contas: Conta[]; carregando: boolean }) {
  const lista = contas.filter((c) => bancarias.includes(c.tipo) && !c.arquivada && c.moeda === "BRL");
  const porBanco = new Map<string, Conta[]>();
  for (const c of lista) {
    const k = nomeDoBanco(c);
    porBanco.set(k, [...(porBanco.get(k) ?? []), c]);
  }
  const total = lista.reduce((s, c) => s + (c.saldo_centavos ?? 0), 0);
  const positivos = lista.reduce((s, c) => s + Math.max(c.saldo_centavos ?? 0, 0), 0);
  const bancos = [...porBanco.entries()]
    .map(([nome, cs]) => ({ nome, contas: cs, saldo: cs.reduce((s, c) => s + (c.saldo_centavos ?? 0), 0) }))
    .sort((a, b) => b.saldo - a.saldo);
  return (
    <Bloco
      icone={<Landmark className="size-4" />}
      rotulo="Contas bancárias"
      valor={total}
      cor={total < 0 ? "text-saida" : "text-texto"}
      carregando={carregando}
    >
      {bancos.length ? (
        <ul className="divide-y divide-borda">
          {bancos.map((b) => (
            <li key={b.nome}>
              <Link
                to={b.contas.length === 1 ? `/gastos?conta=${b.contas[0]?.id}` : "/contas"}
                className="flex min-h-16 items-center gap-3 px-5 py-2.5 hover:bg-superficie-2"
              >
                <IconeBanco nome={b.nome} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{b.nome}</span>
                  <span className="block text-xs text-texto-2">
                    {b.contas.length} {b.contas.length === 1 ? "conta" : "contas"}
                    {positivos > 0 && b.saldo > 0 ? ` · ${formatarPercentual(b.saldo / positivos)}` : ""}
                  </span>
                </span>
                <span className={`valor num font-semibold ${b.saldo < 0 ? "text-saida" : "text-entrada"}`}>
                  {formatarMoeda(b.saldo)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      ) : carregando ? null : (
        <p className="p-5 text-sm text-texto-2">
          Nenhuma conta.{" "}
          <Link to="/contas" className="font-semibold text-primaria">
            Cadastrar
          </Link>
        </p>
      )}
    </Bloco>
  );
}

/** Cartões: quanto está usado (segundo o banco, quando há Open Finance) e de quanto de limite. */
export function Cartoes({ contas, carregando }: { contas: Conta[]; carregando: boolean }) {
  const cartoes = contas.filter((c) => c.tipo === "cartao" && !c.arquivada);
  const usado = (c: Conta) => c.limite_usado_banco_centavos ?? Math.max(-(c.saldo_centavos ?? 0), 0);
  const total = cartoes.reduce((s, c) => s + usado(c), 0);
  const limite = cartoes.reduce((s, c) => s + (c.limite_centavos ?? 0), 0);
  const fracao = limite > 0 ? total / limite : 0;
  return (
    <Bloco
      icone={<CreditCard className="size-4" />}
      rotulo="Cartões de crédito"
      valor={total}
      cor={total > 0 ? "text-saida" : "text-texto"}
      carregando={carregando}
    >
      {limite > 0 ? (
        <div className="border-b border-borda px-5 py-3">
          <div className="flex justify-between text-sm text-texto-2">
            <span>{formatarPercentual(fracao, 0)} utilizado</span>
            <span>
              Limite: <span className="valor num">{formatarMoeda(limite)}</span>
            </span>
          </div>
          <Barra
            fracao={fracao}
            cor={fracao > 0.9 ? "var(--saida)" : fracao > 0.7 ? "var(--alerta)" : "var(--entrada)"}
            rotulo="Limite dos cartões utilizado"
          />
        </div>
      ) : null}
      {cartoes.length ? (
        <ul className="divide-y divide-borda">
          {cartoes.map((c) => (
            <li key={c.id}>
              <Link
                to={`/cartoes?id=${c.id}`}
                className="flex min-h-16 items-center gap-3 px-5 py-2.5 hover:bg-superficie-2"
              >
                <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-superficie-2 text-texto-2">
                  <CreditCard className="size-4" aria-hidden />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{c.nome}</span>
                  <span className="block text-xs text-texto-2">
                    {c.numero_final ? `•••• ${c.numero_final}` : (c.instituicao ?? "Cartão")}
                  </span>
                </span>
                <span className="valor num font-semibold text-saida">{formatarMoeda(usado(c))}</span>
              </Link>
            </li>
          ))}
        </ul>
      ) : carregando ? null : (
        <p className="p-5 text-sm text-texto-2">Nenhum cartão.</p>
      )}
    </Bloco>
  );
}

/** Investimentos por classe (fase 4: a carteira completa entra aqui). */
export function Investimentos({ contas, carregando }: { contas: Conta[]; carregando: boolean }) {
  const lista = contas.filter((c) => c.tipo === "investimento" && !c.arquivada && c.moeda === "BRL");
  const total = lista.reduce((s, c) => s + (c.saldo_centavos ?? 0), 0);
  return (
    <Bloco
      icone={<TrendingUp className="size-4" />}
      rotulo="Investimentos"
      valor={total}
      cor="text-entrada"
      carregando={carregando}
      sub={lista.length ? `${lista.length} ${lista.length === 1 ? "conta" : "contas"}` : undefined}
    >
      {lista.length ? (
        <ul className="flex flex-col gap-4 p-5">
          {lista.map((c) => (
            <li key={c.id}>
              <div className="flex items-baseline justify-between gap-3 text-sm">
                <span className="truncate">{c.nome}</span>
                <span className="valor num shrink-0">
                  <span className="text-texto-2">
                    {total > 0 ? formatarPercentual((c.saldo_centavos ?? 0) / total) : ""}
                  </span>{" "}
                  {formatarMoeda(c.saldo_centavos ?? 0)}
                </span>
              </div>
              <Barra
                fracao={total > 0 ? (c.saldo_centavos ?? 0) / total : 0}
                cor="var(--destaque)"
                rotulo={c.nome}
              />
            </li>
          ))}
        </ul>
      ) : carregando ? null : (
        <p className="p-5 text-sm text-texto-2">
          Cadastre uma conta do tipo “{nomesTipoConta.investimento}”. A carteira completa (ações, FIIs, renda
          fixa) chega na próxima versão.
        </p>
      )}
    </Bloco>
  );
}

/** Evolução do patrimônio líquido: gráfico de área em SVG (sem biblioteca). */
export function EvolucaoSaldo({ serie, carregando }: { serie: PontoPatrimonio[]; carregando: boolean }) {
  const valores = serie.map((p) => p.liquido_centavos);
  const ultimo = valores[valores.length - 1];
  const largura = 1000;
  const altura = 200;
  const min = Math.min(...valores, 0);
  const max = Math.max(...valores, 1);
  const x = (i: number) => (serie.length > 1 ? (i / (serie.length - 1)) * largura : largura / 2);
  const y = (v: number) => altura - ((v - min) / (max - min || 1)) * (altura - 16) - 8;
  const linha = valores
    .map((v, i) => `${i === 0 ? "M" : "L"}${x(i).toFixed(1)},${y(v).toFixed(1)}`)
    .join(" ");
  const area = `${linha} L${largura},${altura} L0,${altura} Z`;
  const rotulo = (d: string) => d.slice(0, 7);
  return (
    <Bloco
      icone={<Wallet className="size-4" />}
      rotulo="Evolução do patrimônio"
      valor={ultimo}
      cor={(ultimo ?? 0) < 0 ? "text-saida" : "text-texto"}
      carregando={carregando}
    >
      {serie.length > 1 ? (
        <div className="px-5 pt-4 pb-3">
          <svg
            viewBox={`0 0 ${largura} ${altura}`}
            preserveAspectRatio="none"
            className="h-40 w-full"
            role="img"
            aria-label={`Patrimônio líquido de ${rotulo(serie[0]?.data ?? "")} a ${rotulo(serie[serie.length - 1]?.data ?? "")}`}
          >
            <defs>
              <linearGradient id="grad-evolucao" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="var(--destaque)" stopOpacity="0.35" />
                <stop offset="100%" stopColor="var(--destaque)" stopOpacity="0" />
              </linearGradient>
            </defs>
            <path d={area} fill="url(#grad-evolucao)" />
            <path
              d={linha}
              fill="none"
              stroke="var(--destaque)"
              strokeWidth="2.5"
              vectorEffect="non-scaling-stroke"
            />
          </svg>
          <div className="mt-2 flex justify-between text-xs text-texto-2">
            <span>{rotulo(serie[0]?.data ?? "")}</span>
            <span>{rotulo(serie[Math.floor(serie.length / 2)]?.data ?? "")}</span>
            <span>{rotulo(serie[serie.length - 1]?.data ?? "")}</span>
          </div>
        </div>
      ) : null}
    </Bloco>
  );
}
