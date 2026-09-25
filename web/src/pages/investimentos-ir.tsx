import { useQuery } from "@tanstack/react-query";
import { ChevronDown, Receipt } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { AvisoSimulacao, SeletorEntidade } from "../components/extras";
import { Aviso, CarregandoLista, Etiqueta, Pagina, Vazio } from "../components/ui";
import { mensagemDe, obter } from "../lib/api";
import { nomeMes, useEditaveis } from "../lib/dados";
import { formatarData, formatarMoeda, soData } from "../lib/formato";
import { type ApuracaoIR, type MesIR, nomesGrupoIR } from "../lib/tipos";

export function InvestimentosIR() {
  const editaveis = useEditaveis();
  const [entidade, setEntidade] = useState("");
  const ent = entidade || editaveis.lista[0]?.id || "";
  const dados = useQuery({
    queryKey: ["investimentos-ir", ent],
    queryFn: () => obter<ApuracaoIR>(`/api/investimentos/ir${ent ? `?entidade_id=${ent}` : ""}`),
    enabled: !editaveis.isPending,
  });
  const ap = dados.data;
  const aPagar = (ap?.meses ?? []).filter((m) => m.darf_centavos > 0);

  return (
    <Pagina
      titulo="IR dos investimentos"
      subtitulo="Ganho de capital em ações, ETFs, BDRs e FIIs, mês a mês, e o DARF de cada mês."
      acao={
        <Link to="/investimentos" className="text-sm font-semibold text-primaria">
          Voltar à carteira
        </Link>
      }
    >
      <SeletorEntidade entidades={editaveis.lista} valor={ent} onChange={setEntidade} />
      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}
      {dados.isPending ? (
        <CarregandoLista />
      ) : !ap?.meses.length ? (
        <Vazio
          icone={<Receipt className="size-8" />}
          titulo="Nenhuma venda de renda variável"
          texto="Importe a negociação da B3 ou lance as vendas no ativo: a apuração aparece aqui, com isenção de R$ 20 mil em ações, compensação de prejuízo e DARF."
        />
      ) : (
        <>
          {aPagar.length ? (
            <Aviso tipo="alerta">
              {aPagar.length === 1 ? "1 mês com DARF" : `${aPagar.length} meses com DARF`}; o mais recente
              vence em {formatarData(soData(aPagar[0]?.vencimento ?? ""))} (código {aPagar[0]?.codigo_darf}).
            </Aviso>
          ) : null}
          <ul className="flex flex-col gap-3">
            {ap.meses.map((m) => (
              <MesApurado key={m.mes} m={m} vendas={ap.vendas.filter((v) => v.mes === m.mes)} />
            ))}
          </ul>
          {ap.fonte ? <p className="text-xs text-texto-2">Regras: {ap.fonte}</p> : null}
        </>
      )}
      <p className="text-xs text-texto-2">
        Não inclui opções, termo e renda fixa (o banco já retém o IR dela). Confira com o seu contador antes
        de pagar.
      </p>
      <AvisoSimulacao />
    </Pagina>
  );
}

function MesApurado({ m, vendas }: { m: MesIR; vendas: ApuracaoIR["vendas"] }) {
  return (
    <li className="overflow-hidden rounded-cartao border border-borda bg-superficie">
      <div className="flex flex-wrap items-center justify-between gap-2 p-4">
        <div>
          <p className="font-semibold capitalize">{nomeMes(m.mes)}</p>
          <p className="text-xs text-texto-2">
            Vendas de ações: {formatarMoeda(m.vendas_acoes_centavos)}
            {m.isento_acoes ? " · isentas (até R$ 20 mil)" : ""}
          </p>
        </div>
        <div className="text-right">
          {m.darf_centavos > 0 ? (
            <>
              <p className="valor num text-lg font-bold text-imposto">{formatarMoeda(m.darf_centavos)}</p>
              <p className="text-xs text-texto-2">
                DARF {m.codigo_darf} até {formatarData(soData(m.vencimento))}
              </p>
            </>
          ) : m.darf_acumulado_centavos > 0 ? (
            <>
              <p className="valor num font-semibold">{formatarMoeda(m.darf_acumulado_centavos)}</p>
              <p className="text-xs text-texto-2">abaixo de R$ 10: soma no próximo mês</p>
            </>
          ) : (
            <Etiqueta cor="entrada">sem imposto</Etiqueta>
          )}
        </div>
      </div>
      <details className="border-t border-borda">
        <summary className="flex min-h-11 cursor-pointer items-center gap-2 px-4 text-sm text-texto-2">
          <ChevronDown className="size-4" aria-hidden /> Detalhes
        </summary>
        <div className="flex flex-col gap-3 px-4 pb-4 text-sm">
          {m.grupos.map((g) => (
            <dl key={g.grupo} className="grid grid-cols-2 gap-x-3 gap-y-1">
              <dt className="col-span-2 font-semibold">
                {nomesGrupoIR[g.grupo]} ({(Number(g.aliquota) * 100).toLocaleString("pt-BR")} %)
              </dt>
              <dt className="text-texto-2">Resultado</dt>
              <dd className={`valor num text-right ${g.ganho_centavos < 0 ? "text-saida" : ""}`}>
                {formatarMoeda(g.ganho_centavos)}
              </dd>
              {g.isento_centavos ? (
                <>
                  <dt className="text-texto-2">Isento</dt>
                  <dd className="valor num text-right">{formatarMoeda(g.isento_centavos)}</dd>
                </>
              ) : null}
              {g.prejuizo_anterior_centavos ? (
                <>
                  <dt className="text-texto-2">Prejuízo a compensar</dt>
                  <dd className="valor num text-right">{formatarMoeda(g.prejuizo_anterior_centavos)}</dd>
                </>
              ) : null}
              <dt className="text-texto-2">Base</dt>
              <dd className="valor num text-right">{formatarMoeda(g.base_centavos)}</dd>
              <dt className="text-texto-2">Imposto</dt>
              <dd className="valor num text-right">{formatarMoeda(g.imposto_centavos)}</dd>
              {g.prejuizo_acumulado_centavos ? (
                <>
                  <dt className="text-texto-2">Prejuízo para os próximos meses</dt>
                  <dd className="valor num text-right">{formatarMoeda(g.prejuizo_acumulado_centavos)}</dd>
                </>
              ) : null}
            </dl>
          ))}
          {m.ir_retido_centavos ? (
            <p className="text-texto-2">
              IR retido na fonte (dedo-duro) abatido: {formatarMoeda(m.ir_retido_centavos)}
            </p>
          ) : null}
          {vendas.length ? (
            <ul className="divide-y divide-borda rounded-xl border border-borda">
              {vendas.map((v) => (
                <li
                  key={`${v.data}-${v.ativo}-${v.valor_centavos}`}
                  className="flex justify-between gap-2 px-3 py-2"
                >
                  <span>
                    <span className="font-medium">{v.ativo}</span>{" "}
                    <span className="text-xs text-texto-2">
                      {formatarData(v.data)}
                      {v.day_trade ? " · day trade" : ""}
                    </span>
                  </span>
                  <span className="text-right">
                    <span className="valor num block">{formatarMoeda(v.valor_centavos)}</span>
                    <span
                      className={`valor num block text-xs ${v.ganho_centavos < 0 ? "text-saida" : "text-entrada"}`}
                    >
                      {v.ganho_centavos >= 0 ? "+" : ""}
                      {formatarMoeda(v.ganho_centavos)}
                    </span>
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      </details>
    </li>
  );
}
