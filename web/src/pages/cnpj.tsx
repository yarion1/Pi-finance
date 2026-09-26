import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Briefcase,
  CalendarClock,
  Download,
  FileText,
  Gauge,
  Plus,
  Receipt,
  Scale,
  Users,
  Wallet,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import {
  DialogoNFSe,
  DialogoReceita,
  FormFolha,
  FormLucro,
  Simulador,
  useInvalidarCNPJ,
} from "../components/cnpj";
import { AvisoSimulacao, Barra } from "../components/extras";
import {
  Aviso,
  Botao,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { Bloco } from "../components/visao";
import { api, mensagemDe, obter } from "../lib/api";
import { nomeMes, useEntidades } from "../lib/dados";
import { formatarData, formatarMoeda, formatarPercentual, hojeSP } from "../lib/formato";
import { type DASMes, type NotaCNPJ, nomesRegime, nomesSituacaoMEI, type PainelCNPJ } from "../lib/tipos";

export function CNPJ() {
  const entidades = useEntidades();
  const pjs = (entidades.data ?? []).filter((e) => e.tipo === "PJ");
  const [escolhida, setEscolhida] = useState("");
  const id = escolhida || pjs[0]?.id || "";

  const painel = useQuery({
    queryKey: ["cnpj-painel", id],
    queryFn: () => obter<PainelCNPJ>(`/api/cnpj/${id}/painel`),
    enabled: !!id,
  });
  const p = painel.data;

  if (!entidades.isPending && pjs.length === 0) {
    return (
      <Pagina titulo="CNPJ" subtitulo="Como está a empresa?">
        <Vazio
          icone={<Briefcase className="size-8" />}
          titulo="Nenhum CNPJ cadastrado"
          texto="Cadastre o CNPJ (MEI ou Simples) em Entidades para acompanhar o teto, o DAS, as notas e o lucro."
          acao={
            <Link to="/entidades" className="text-sm font-semibold text-primaria">
              Cadastrar CNPJ
            </Link>
          }
        />
      </Pagina>
    );
  }

  return (
    <Pagina
      titulo="CNPJ"
      subtitulo={
        p
          ? `${p.entidade.nome} · ${nomesRegime[p.entidade.regime] ?? p.entidade.regime}`
          : "Como está a empresa?"
      }
      acao={
        pjs.length > 1 ? (
          <select
            aria-label="CNPJ"
            value={id}
            onChange={(e) => setEscolhida(e.target.value)}
            className="min-h-11 rounded-xl border border-borda bg-superficie-2 px-3 text-base"
          >
            {pjs.map((e) => (
              <option key={e.id} value={e.id}>
                {e.nome}
              </option>
            ))}
          </select>
        ) : null
      }
    >
      {painel.error ? <Aviso tipo="erro">{mensagemDe(painel.error)}</Aviso> : null}
      {painel.isPending ? <CarregandoLista linhas={4} /> : null}
      {p ? <Conteudo p={p} /> : null}
      <AvisoSimulacao />
    </Pagina>
  );
}

function Conteudo({ p }: { p: PainelCNPJ }) {
  const [dialogo, setDialogo] = useState<"receita" | "xml" | null>(null);
  const ent = p.entidade.id;
  const editar = p.entidade.pode_editar;
  return (
    <>
      {p.alertas.map((a) => (
        <Aviso key={a} tipo="alerta">
          {a}
        </Aviso>
      ))}
      <DialogoReceita entidade={ent} aberto={dialogo === "receita"} aoFechar={() => setDialogo(null)} />
      <DialogoNFSe entidade={ent} aberto={dialogo === "xml"} aoFechar={() => setDialogo(null)} />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {p.mei ? <BlocoTeto p={p} /> : <BlocoSimples p={p} />}
        <Bloco
          icone={<Wallet className="size-4" />}
          rotulo={`Receita de ${nomeMes(p.hoje.slice(0, 7))}`}
          valor={p.receita_mes_centavos}
          cor="text-entrada"
          sub={`No ano: ${formatarMoeda(p.receita_ano_centavos)} · despesas ${formatarMoeda(p.despesas_ano_centavos)}`}
        >
          <MesesDoAno p={p} />
        </Bloco>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Cartao>
          <TituloCartao>
            <span className="flex items-center gap-2">
              <CalendarClock className="size-4 text-destaque" aria-hidden />
              {p.mei ? "DAS-MEI" : "DAS do Simples"}
            </span>
          </TituloCartao>
          <ListaDAS entidade={ent} meses={p.mei?.das_meses ?? p.simples?.das_meses ?? []} editar={editar} />
        </Cartao>
        {p.mei ? <BlocoLucroMEI p={p} /> : <BlocoTributos p={p} />}
      </div>

      <Cartao>
        <TituloCartao
          acao={
            editar ? (
              <div className="flex flex-wrap gap-2">
                <Botao variante="secundario" onClick={() => setDialogo("xml")}>
                  <FileText className="size-4" aria-hidden /> NFS-e (XML)
                </Botao>
                <a
                  href={`/api/cnpj/${ent}/pacote?mes=${p.hoje.slice(0, 7)}`}
                  download
                  className="inline-flex min-h-11 items-center gap-2 rounded-xl border border-borda bg-superficie-2 px-4 text-sm font-semibold hover:border-texto-2"
                >
                  <Download className="size-4" aria-hidden /> Contador
                </a>
                <Botao onClick={() => setDialogo("receita")}>
                  <Plus className="size-4" aria-hidden /> Receita
                </Botao>
              </div>
            ) : null
          }
        >
          <span className="flex items-center gap-2">
            <Receipt className="size-4 text-destaque" aria-hidden /> Receitas e notas
          </span>
        </TituloCartao>
        <ListaNotas entidade={ent} editar={editar} />
      </Cartao>

      {p.clientes.length ? (
        <Cartao>
          <TituloCartao>
            <span className="flex items-center gap-2">
              <Users className="size-4 text-destaque" aria-hidden /> Clientes (12 meses)
            </span>
          </TituloCartao>
          <ul className="flex flex-col gap-3">
            {p.clientes.map((c) => (
              <li key={c.id ?? c.nome}>
                <div className="flex justify-between gap-3 text-sm">
                  <span>
                    {c.nome} {c.pais !== "BR" ? <Etiqueta>{c.pais}</Etiqueta> : null}
                  </span>
                  <span className="valor num">
                    <span className="text-texto-2">{formatarPercentual(c.percentual, 0)}</span>{" "}
                    {formatarMoeda(c.receita_centavos)}
                  </span>
                </div>
                <Barra
                  fracao={c.percentual}
                  cor={c.percentual > 0.7 ? "var(--alerta)" : "var(--destaque)"}
                  rotulo={c.nome}
                />
              </li>
            ))}
          </ul>
        </Cartao>
      ) : null}

      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <Scale className="size-4 text-destaque" aria-hidden />
            {p.mei ? "Continuar MEI ou virar ME?" : "Simulador de regime"}
          </span>
        </TituloCartao>
        <Simulador
          atividade={p.entidade.atividade}
          receitaInicial={Math.round(p.receita_ano_centavos / Math.max(p.meses.length, 1))}
        />
      </Cartao>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {p.simples && editar ? (
          <Cartao>
            <TituloCartao>Folha e pró-labore</TituloCartao>
            <FormFolha entidade={ent} />
          </Cartao>
        ) : null}
        {editar ? (
          <Cartao>
            <TituloCartao>Retirada de lucro</TituloCartao>
            <p className="mb-3 text-sm text-texto-2">
              Lucro estimado deste mês: {formatarMoeda(p.lucro_distribuivel_mes_centavos)} (receita menos o
              DAS; tire também as despesas e deixe uma reserva no caixa). Acima de R$ 50 mil no mês para a
              mesma pessoa, há IRRF de 10 % sobre o total.
            </p>
            <FormLucro entidade={ent} />
          </Cartao>
        ) : null}
      </div>
    </>
  );
}

function BlocoTeto({ p }: { p: PainelCNPJ }) {
  const m = p.mei;
  if (!m) return null;
  const cor =
    m.situacao === "normal"
      ? "var(--entrada)"
      : m.situacao === "atencao" || m.situacao === "planejar"
        ? "var(--alerta)"
        : "var(--saida)";
  return (
    <Bloco
      icone={<Gauge className="size-4" />}
      rotulo={`Teto do MEI em ${p.hoje.slice(0, 4)}`}
      valor={m.projecao.faturado_centavos}
      cor={m.situacao === "normal" ? "text-entrada" : "text-texto"}
      sub={`${formatarPercentual(m.projecao.percentual, 0)} de ${formatarMoeda(m.projecao.teto_centavos)} · ${nomesSituacaoMEI[m.situacao]}`}
    >
      <div className="flex flex-col gap-3 p-5 text-sm">
        <Barra
          fracao={m.projecao.percentual}
          marca={0.7}
          cor={cor}
          rotulo="Faturamento contra o teto do MEI"
        />
        <dl className="grid grid-cols-2 gap-3">
          <Info rotulo="Falta para o teto" valor={formatarMoeda(m.projecao.falta_centavos)} />
          <Info
            rotulo={`Média máxima por mês (${m.projecao.meses_restantes})`}
            valor={formatarMoeda(m.projecao.media_maxima_centavos)}
          />
          <Info
            rotulo="No ritmo, termina o ano com"
            valor={formatarMoeda(m.projecao.projecao_ano_centavos)}
          />
          <Info
            rotulo="Bate o teto em"
            valor={
              m.projecao.data_teto ? formatarData(m.projecao.data_teto.slice(0, 10)) : "não bate este ano"
            }
          />
          {m.exportacao_ano_centavos ? (
            <Info rotulo="Receita do exterior no ano" valor={formatarMoeda(m.exportacao_ano_centavos)} />
          ) : null}
          <Info rotulo="Últimos 12 meses" valor={formatarMoeda(m.receita_12m_centavos)} />
        </dl>
      </div>
    </Bloco>
  );
}

function BlocoSimples({ p }: { p: PainelCNPJ }) {
  const s = p.simples;
  if (!s) return null;
  return (
    <Bloco
      icone={<Gauge className="size-4" />}
      rotulo={`DAS de ${nomeMes(p.hoje.slice(0, 7))} (Anexo ${s.anexo})`}
      valor={s.das.das_centavos}
      cor="text-imposto"
      sub={`Alíquota efetiva ${formatarPercentual(Number(s.das.aliquota_efetiva), 2)} · faixa ${s.das.faixa}`}
    >
      <dl className="grid grid-cols-2 gap-3 p-5 text-sm">
        <Info rotulo="Receita dos últimos 12 meses (RBT12)" valor={formatarMoeda(s.rbt12_centavos)} />
        <Info rotulo="Folha dos 12 meses" valor={formatarMoeda(s.folha_12m_centavos)} />
        <Info
          rotulo={`Fator R (mínimo ${formatarPercentual(Number(s.fator_r_minimo), 0)})`}
          valor={formatarPercentual(Number(s.fator_r), 2)}
        />
        <Info
          rotulo="Pró-labore para o Anexo III"
          valor={`${formatarMoeda(s.prolabore_para_fator_r_centavos)}/mês`}
        />
        {s.das.exportacao_centavos ? (
          <Info
            rotulo="Economia da exportação"
            valor={formatarMoeda(s.das.das_sem_exportacao_centavos - s.das.das_centavos)}
          />
        ) : null}
      </dl>
    </Bloco>
  );
}

function BlocoTributos({ p }: { p: PainelCNPJ }) {
  const s = p.simples;
  if (!s) return null;
  const nomes: Record<string, string> = {
    irpj: "IRPJ",
    csll: "CSLL",
    cofins: "COFINS",
    pis: "PIS",
    cpp: "INSS patronal (CPP)",
    iss: "ISS",
  };
  return (
    <Cartao>
      <TituloCartao>Para onde vai o DAS</TituloCartao>
      <ul className="divide-y divide-borda text-sm">
        {s.das.tributos.map((t) => (
          <li key={t.tributo} className="flex justify-between gap-2 py-2">
            <span>
              {nomes[t.tributo] ?? t.tributo}{" "}
              <span className="text-xs text-texto-2">{formatarPercentual(Number(t.aliquota), 2)}</span>
            </span>
            <span className="valor num">{formatarMoeda(t.valor_centavos)}</span>
          </li>
        ))}
      </ul>
    </Cartao>
  );
}

function BlocoLucroMEI({ p }: { p: PainelCNPJ }) {
  const m = p.mei;
  if (!m) return null;
  return (
    <Cartao>
      <TituloCartao>Lucro do MEI no seu IR</TituloCartao>
      <dl className="grid grid-cols-2 gap-3 text-sm">
        <Info rotulo="Receita no ano" valor={formatarMoeda(m.lucro.receita_centavos)} />
        <Info rotulo="Despesas do CNPJ" valor={formatarMoeda(m.lucro.despesas_centavos)} />
        <Info
          rotulo="Lucro isento (32 % serviços / 8 % comércio)"
          valor={formatarMoeda(m.lucro.isento_centavos)}
        />
        <Info
          rotulo="Parte tributável na sua declaração"
          valor={formatarMoeda(m.lucro.tributavel_centavos)}
        />
      </dl>
      <p className="mt-4 text-sm text-texto-2">
        DASN-SIMEI de {m.dasn_ano}: faturamento de {formatarMoeda(m.dasn_receita_centavos)}, entregar até{" "}
        {formatarData(m.dasn_prazo)}.
      </p>
      <p className="mt-2 text-xs text-texto-2">
        As despesas são os gastos lançados nas contas do CNPJ. Sem escrituração, só a parte isenta presumida
        vale.
      </p>
    </Cartao>
  );
}

function MesesDoAno({ p }: { p: PainelCNPJ }) {
  const maior = Math.max(...p.meses.map((m) => m.receita_centavos), 1);
  const media = p.mei?.projecao.media_maxima_centavos;
  return (
    <ul className="flex flex-col gap-2 p-5 text-sm">
      {p.meses
        .slice()
        .reverse()
        .map((m) => (
          <li key={m.mes}>
            <div className="flex justify-between gap-3">
              <span className="capitalize">{nomeMes(m.mes)}</span>
              <span className="valor num">{formatarMoeda(m.receita_centavos)}</span>
            </div>
            <Barra
              fracao={m.receita_centavos / maior}
              cor="var(--destaque)"
              rotulo={`Receita de ${nomeMes(m.mes)}`}
            />
          </li>
        ))}
      {media !== undefined ? (
        <li className="text-xs text-texto-2">
          Para não estourar o teto: até {formatarMoeda(media)} por mês.
        </li>
      ) : null}
    </ul>
  );
}

function ListaDAS({ entidade, meses, editar }: { entidade: string; meses: DASMes[]; editar: boolean }) {
  const invalidar = useInvalidarCNPJ();
  const marcar = useMutation({
    mutationFn: (x: { competencia: string; pago: boolean }) =>
      api("PUT", `/api/cnpj/${entidade}/das/${x.competencia}`, { pago_em: x.pago ? hojeSP() : null }),
    onSuccess: invalidar,
  });
  const lista = meses.slice().reverse();
  if (!lista.length) return <p className="text-sm text-texto-2">Nenhuma competência ainda.</p>;
  return (
    <>
      {marcar.error ? <Aviso tipo="erro">{mensagemDe(marcar.error)}</Aviso> : null}
      <ul className="divide-y divide-borda text-sm">
        {lista.map((d) => (
          <li key={d.competencia} className="flex min-h-12 items-center justify-between gap-2 py-1.5">
            <span>
              <span className="block capitalize">{nomeMes(d.competencia)}</span>
              <span className="block text-xs text-texto-2">
                vence {formatarData(d.vencimento)}
                {d.valor_centavos ? ` · ${formatarMoeda(d.valor_centavos)}` : ""}
              </span>
            </span>
            {d.pago_em ? (
              <button
                type="button"
                disabled={!editar}
                onClick={() => marcar.mutate({ competencia: d.competencia, pago: false })}
                className="min-h-11 px-2"
                aria-label={`Desmarcar pagamento de ${nomeMes(d.competencia)}`}
              >
                <Etiqueta cor="entrada">pago</Etiqueta>
              </button>
            ) : editar ? (
              <Botao
                variante={d.atrasado ? "perigo" : "secundario"}
                onClick={() => marcar.mutate({ competencia: d.competencia, pago: true })}
                carregando={marcar.isPending && marcar.variables?.competencia === d.competencia}
              >
                {d.atrasado ? "Atrasado · paguei" : "Paguei"}
              </Botao>
            ) : d.atrasado ? (
              <Etiqueta cor="alerta">atrasado</Etiqueta>
            ) : null}
          </li>
        ))}
      </ul>
    </>
  );
}

function ListaNotas({ entidade, editar }: { entidade: string; editar: boolean }) {
  const invalidar = useInvalidarCNPJ();
  const notas = useQuery({
    queryKey: ["cnpj-notas", entidade],
    queryFn: () => obter<NotaCNPJ[]>(`/api/cnpj/${entidade}/notas`),
  });
  const cancelar = useMutation({
    mutationFn: (n: NotaCNPJ) => api("PATCH", `/api/cnpj/notas/${n.id}`, { cancelada: !n.cancelada }),
    onSuccess: invalidar,
  });
  if (notas.isPending) return <CarregandoLista linhas={2} />;
  if (!notas.data?.length) {
    return (
      <p className="text-sm text-texto-2">
        Nenhuma receita neste ano. Lance à mão ou importe o XML da NFS-e.
      </p>
    );
  }
  return (
    <ul className="divide-y divide-borda text-sm">
      {notas.data.map((n) => (
        <li key={n.id} className={`flex min-h-14 items-center gap-3 py-2 ${n.cancelada ? "opacity-50" : ""}`}>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">
              {n.cliente ?? "Sem cliente"}
              {n.numero ? <span className="text-texto-2"> · nota {n.numero}</span> : null}
            </span>
            <span className="flex flex-wrap items-center gap-1 text-xs text-texto-2">
              {formatarData(n.data_emissao)}
              {n.moeda !== "BRL" && n.valor_moeda_centavos !== null
                ? ` · ${n.moeda} ${(n.valor_moeda_centavos / 100).toLocaleString("pt-BR", { minimumFractionDigits: 2 })} a ${n.taxa_cambio?.replace(".", ",")}`
                : ""}
              {n.exportacao ? <Etiqueta>exportação</Etiqueta> : null}
              {n.origem === "nfse" ? <Etiqueta>NFS-e</Etiqueta> : null}
              {n.cancelada ? <Etiqueta cor="alerta">cancelada</Etiqueta> : null}
            </span>
          </span>
          <span className="valor num shrink-0 font-semibold text-entrada">
            {formatarMoeda(n.valor_centavos)}
          </span>
          {editar ? (
            <button
              type="button"
              onClick={() => cancelar.mutate(n)}
              className="min-h-11 shrink-0 px-2 text-xs text-texto-2 hover:text-texto"
            >
              {n.cancelada ? "Reativar" : "Cancelar"}
            </button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function Info({ rotulo, valor }: { rotulo: string; valor: string }) {
  return (
    <div>
      <dt className="text-xs text-texto-2">{rotulo}</dt>
      <dd className="valor num font-semibold">{valor}</dd>
    </div>
  );
}
