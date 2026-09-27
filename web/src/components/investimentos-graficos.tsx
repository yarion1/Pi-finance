import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, LineChart, PieChart } from "lucide-react";
import { useState } from "react";
import { api, mensagemDe, obter } from "../lib/api";
import { decimalBR, formatarData, formatarMoeda, hojeSP } from "../lib/formato";
import { percentual } from "../lib/graficos";
import type { Carteira, HistoricoInvestimentos, RespostaAlvo } from "../lib/tipos";
import { nomesClasse } from "../lib/tipos";
import { Dialogo } from "./extras";
import { GraficoAlocacao, GraficoMensal, GraficoRentabilidade, SeletorPeriodo } from "./graficos-painel";
import { Aviso, Botao, Campo, Cartao, Esqueleto, TituloCartao, Vazio } from "./ui";

/** Gráficos da tela Investimentos: rentabilidade × índices, alocação × alvo, proventos e vencimentos. */
export function GraficosInvestimentos({ carteira }: { carteira: Carteira }) {
  const [meses, setMeses] = useState(12);
  const historico = useQuery({
    queryKey: ["investimentos", "historico", meses],
    queryFn: () => obter<HistoricoInvestimentos>(`/api/investimentos/historico?meses=${meses}`),
    placeholderData: (anterior) => anterior,
  });
  const h = historico.data;
  const comHistorico = h?.valores_centavos.some((v) => v > 0);
  const proventos = (h?.proventos ?? []).filter((p) => p.valor_centavos > 0).length
    ? (h?.proventos ?? [])
    : [];
  const hoje = hojeSP();
  const umAno = `${Number(hoje.slice(0, 4)) + 1}${hoje.slice(4)}`;
  const vencimentos = carteira.ativos
    .filter((a) => !a.encerrado && a.vencimento && a.vencimento >= hoje && a.vencimento <= umAno)
    .sort((a, b) => (a.vencimento ?? "").localeCompare(b.vencimento ?? ""));

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Cartao className="lg:col-span-2">
        <TituloCartao
          acao={<SeletorPeriodo valor={meses} onChange={setMeses} nome="periodo-rentabilidade" />}
        >
          <span className="flex items-center gap-2">
            <LineChart className="size-4 text-destaque" aria-hidden /> Rentabilidade acumulada
          </span>
        </TituloCartao>
        {historico.error ? <Aviso tipo="erro">{mensagemDe(historico.error)}</Aviso> : null}
        {!h ? (
          <Esqueleto className="h-64 w-full" />
        ) : comHistorico ? (
          <>
            <GraficoRentabilidade
              datas={h.datas}
              series={[
                { nome: "Carteira", valores: h.carteira, principal: true, cor: 0 },
                { nome: "CDI", valores: h.cdi, cor: 1 },
                { nome: "IPCA", valores: h.ipca, cor: 2 },
                { nome: "Ibovespa", valores: h.ibovespa, cor: 3 },
              ].filter((x) => x.principal || x.valores.filter((v) => v !== null).length > 1)}
            />
            <p className="mt-2 text-xs text-texto-2">
              Retorno ponderado pelo tempo (aportes e resgates não distorcem), dos ativos com operações.
              {h.fora_do_historico
                ? ` ${h.fora_do_historico} investimentos do Open Finance ficam de fora: o banco só informa o saldo de hoje.`
                : ""}
            </p>
          </>
        ) : (
          <Vazio
            titulo="Sem histórico para comparar"
            texto="Importe o extrato da B3 ou lance as operações dos ativos para ver a carteira contra o CDI e a inflação."
          />
        )}
      </Cartao>

      <Alocacao carteira={carteira} />

      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <CalendarClock className="size-4 text-destaque" aria-hidden /> Vencimentos em 12 meses
          </span>
        </TituloCartao>
        {vencimentos.length ? (
          <ul className="flex flex-col divide-y divide-borda" aria-label="Vencimentos">
            {vencimentos.map((a) => (
              <li key={a.id} className="flex min-h-12 items-center gap-3 py-2 text-sm">
                <span className="w-20 shrink-0 text-xs text-texto-2">{formatarData(a.vencimento ?? "")}</span>
                <span className="min-w-0 flex-1 truncate">{a.nome}</span>
                <span className="valor num">{formatarMoeda(a.valor_centavos)}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-texto-2">Nenhum título vence nos próximos 12 meses.</p>
        )}
      </Cartao>

      {proventos.length ? (
        <Cartao className="lg:col-span-2">
          <TituloCartao>Proventos e juros por mês</TituloCartao>
          <GraficoMensal
            meses={proventos.map((p) => p.mes)}
            valores={proventos.map((p) => p.valor_centavos)}
            cor={(c) => c.entrada}
            nome="Proventos"
            rotulo={`Proventos e juros recebidos por mês: ${formatarMoeda(proventos.reduce((s, p) => s + p.valor_centavos, 0))} no período`}
          />
        </Cartao>
      ) : null}
    </div>
  );
}

function Alocacao({ carteira }: { carteira: Carteira }) {
  const alvo = useQuery({
    queryKey: ["investimentos", "alvo"],
    queryFn: () => obter<RespostaAlvo>("/api/investimentos/alvo"),
  });
  const [editando, setEditando] = useState(false);
  const a = alvo.data?.alvo ?? {};
  const classes = [...new Set([...carteira.classes.map((c) => c.classe), ...Object.keys(a)])].map(
    (classe) => ({
      classe,
      nome: nomesClasse[classe] ?? classe,
      atual: carteira.classes.find((c) => c.classe === classe)?.percentual ?? 0,
      alvo: a[classe] ? Number(a[classe]) : Object.keys(a).length ? 0 : null,
    }),
  );
  return (
    <Cartao>
      <TituloCartao
        acao={
          <Botao variante="secundario" onClick={() => setEditando(true)} disabled={!alvo.data}>
            {Object.keys(a).length ? "Mudar alvo" : "Definir alvo"}
          </Botao>
        }
      >
        <span className="flex items-center gap-2">
          <PieChart className="size-4 text-destaque" aria-hidden /> Alocação
        </span>
      </TituloCartao>
      {classes.length ? (
        <GraficoAlocacao classes={classes} />
      ) : (
        <p className="text-sm text-texto-2">A alocação aparece quando houver investimentos.</p>
      )}
      <p className="mt-1 text-xs text-texto-2">
        {Object.keys(a).length
          ? "Rosa: como está hoje; azul: o alvo."
          : "Defina um alvo por classe para comparar."}
      </p>
      <Dialogo aberto={editando} aoFechar={() => setEditando(false)} titulo="Alocação alvo">
        {editando && alvo.data ? (
          <FormAlvo
            entidade={alvo.data.entidade_id}
            atual={a}
            classes={classes.map((c) => c.classe)}
            aoFechar={() => setEditando(false)}
          />
        ) : null}
      </Dialogo>
    </Cartao>
  );
}

const todasClasses = [
  "acao",
  "fii",
  "etf",
  "bdr",
  "renda_fixa",
  "tesouro",
  "fundo",
  "cripto",
  "exterior",
  "previdencia",
  "outro",
];

function FormAlvo({
  entidade,
  atual,
  classes,
  aoFechar,
}: {
  entidade: string;
  atual: Record<string, string>;
  classes: string[];
  aoFechar: () => void;
}) {
  const cliente = useQueryClient();
  const [valores, setValores] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      todasClasses.map((c) => [c, atual[c] ? String(Number(atual[c]) * 100).replace(".", ",") : ""]),
    ),
  );
  const soma = Object.values(valores).reduce((s, v) => s + (Number(decimalBR(v) ?? 0) || 0), 0);
  const salvar = useMutation({
    mutationFn: () => {
      const alvo: Record<string, string> = {};
      for (const [classe, texto] of Object.entries(valores)) {
        if (!texto.trim()) continue;
        const v = decimalBR(texto);
        if (v === null || Number(v) <= 0 || Number(v) > 100) throw new Error("Percentuais entre 0 e 100.");
        alvo[classe] = (Number(v) / 100).toFixed(4);
      }
      return api("PUT", "/api/investimentos/alvo", { entidade_id: entidade, alvo });
    },
    onSuccess: () => {
      cliente.invalidateQueries({ queryKey: ["investimentos", "alvo"] });
      aoFechar();
    },
  });
  const ordem = [...classes, ...todasClasses.filter((c) => !classes.includes(c))];
  return (
    <form
      className="flex flex-col gap-3"
      onSubmit={(e) => {
        e.preventDefault();
        salvar.mutate();
      }}
    >
      <div className="grid grid-cols-2 gap-3">
        {ordem.map((c) => (
          <Campo
            key={c}
            rotulo={`${nomesClasse[c] ?? c} (%)`}
            inputMode="decimal"
            value={valores[c] ?? ""}
            onChange={(e) => setValores({ ...valores, [c]: e.target.value })}
          />
        ))}
      </div>
      <p className={`text-sm ${Math.abs(soma - 100) < 0.01 || soma === 0 ? "text-texto-2" : "text-saida"}`}>
        Soma: {percentual(soma / 100)} {soma === 0 ? "(vazio apaga o alvo)" : "(precisa dar 100 %)"}
      </p>
      {salvar.error ? <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso> : null}
      <div className="flex gap-2">
        <Botao type="submit" carregando={salvar.isPending}>
          Salvar
        </Botao>
        <Botao variante="secundario" onClick={aoFechar}>
          Cancelar
        </Botao>
      </div>
    </form>
  );
}
