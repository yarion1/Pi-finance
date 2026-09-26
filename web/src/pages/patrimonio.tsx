import { useMutation, useQuery } from "@tanstack/react-query";
import { Car, Home as Casa, Landmark, Package, Pencil, Plus, Table2, Trash2 } from "lucide-react";
import { useState } from "react";
import { AvisoSimulacao, Dialogo, SeletorEntidade } from "../components/extras";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Esqueleto,
  Pagina,
  Selecao,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { useContas, useEditaveis, useInvalidarPlanejamento } from "../lib/dados";
import {
  centavosParaTexto,
  formatarData,
  formatarMoeda,
  fracaoParaPercentual,
  hojeSP,
  lerCentavos,
  percentualParaFracao,
  soData,
} from "../lib/formato";
import type {
  Bem,
  Divida,
  EmprestimoBanco,
  ParcelaDivida,
  PontoPatrimonio,
  RespostaPatrimonio,
} from "../lib/tipos";

const nomesBem: Record<Bem["tipo"], string> = { imovel: "Imóvel", veiculo: "Veículo", outro: "Outro" };
const iconesBem = { imovel: Casa, veiculo: Car, outro: Package };
const nomesDivida: Record<Divida["tipo"], string> = {
  financiamento: "Financiamento",
  emprestimo: "Empréstimo",
  outro: "Outra",
};

function Variacao({ centavos, rotulo }: { centavos: number; rotulo: string }) {
  return (
    <span className="text-sm text-texto-2">
      <span className={`valor num font-semibold ${centavos < 0 ? "text-saida" : "text-entrada"}`}>
        {centavos >= 0 ? "+" : ""}
        {formatarMoeda(centavos)}
      </span>{" "}
      {rotulo}
    </span>
  );
}

export function Patrimonio() {
  const dados = useQuery({
    queryKey: ["patrimonio"],
    queryFn: () => obter<RespostaPatrimonio>("/api/patrimonio"),
  });
  const [bem, setBem] = useState<Bem | "novo" | null>(null);
  const [divida, setDivida] = useState<Divida | "nova" | null>(null);
  const [tabela, setTabela] = useState<Divida | null>(null);
  const p = dados.data;

  const composicao = p
    ? [
        { nome: "Contas", valor: p.contas_centavos },
        { nome: "Investimentos", valor: p.investimentos_centavos },
        { nome: "Bens", valor: p.bens_centavos },
        { nome: "Cartões", valor: -p.cartoes_centavos },
        { nome: "Contas no negativo", valor: -p.contas_negativas_centavos },
        { nome: "Dívidas", valor: -p.dividas_centavos },
        { nome: "Empréstimos do banco", valor: -(p.dividas_banco_centavos ?? 0) },
      ].filter((c) => (c.nome !== "Contas no negativo" && c.nome !== "Empréstimos do banco") || c.valor !== 0)
    : [];

  return (
    <Pagina titulo="Patrimônio" subtitulo="Quanto tenho, descontado o que devo.">
      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}
      <Cartao>
        <p className="text-sm text-texto-2">Patrimônio líquido</p>
        {dados.isPending ? (
          <Esqueleto className="mt-2 h-9 w-48" />
        ) : p ? (
          <>
            <p
              className={`valor num mt-1 text-3xl font-bold tracking-tight ${p.hoje.liquido_centavos < 0 ? "text-saida" : ""}`}
            >
              {formatarMoeda(p.hoje.liquido_centavos)}
            </p>
            <p className="mt-1 flex flex-wrap gap-x-4 gap-y-1">
              <Variacao centavos={p.variacao_mes_centavos} rotulo="no mês" />
              <Variacao centavos={p.variacao_ano_centavos} rotulo="no ano" />
            </p>
            <dl className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
              {composicao.map((c) => (
                <div key={c.nome}>
                  <dt className="text-xs text-texto-2">{c.nome}</dt>
                  <dd className={`valor num text-sm font-medium ${c.valor < 0 ? "text-saida" : ""}`}>
                    {formatarMoeda(c.valor)}
                  </dd>
                </div>
              ))}
            </dl>
          </>
        ) : null}
      </Cartao>

      {p?.serie?.length ? (
        <Cartao>
          <TituloCartao>Últimos 12 meses</TituloCartao>
          <Serie pontos={p.serie} />
        </Cartao>
      ) : null}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Cartao>
          <TituloCartao
            acao={
              <Botao variante="secundario" onClick={() => setBem("novo")}>
                <Plus className="size-4" aria-hidden /> Bem
              </Botao>
            }
          >
            Bens
          </TituloCartao>
          {dados.isPending ? (
            <CarregandoLista />
          ) : p?.bens.length ? (
            <ul className="flex flex-col divide-y divide-borda">
              {p.bens.map((b) => {
                const Icone = iconesBem[b.tipo];
                return (
                  <li key={b.id} className="flex min-h-14 items-center gap-3 py-2">
                    <Icone className="size-5 shrink-0 text-texto-2" aria-hidden />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">{b.nome}</span>
                      <span className="block text-xs text-texto-2">
                        {nomesBem[b.tipo]} · atualizado em {formatarData(b.atualizado_em)}
                      </span>
                    </span>
                    <span className="valor num text-sm font-semibold">{formatarMoeda(b.valor_centavos)}</span>
                    <Botao variante="fantasma" aria-label={`Editar ${b.nome}`} onClick={() => setBem(b)}>
                      <Pencil className="size-4" />
                    </Botao>
                  </li>
                );
              })}
            </ul>
          ) : (
            <Vazio titulo="Nenhum bem" texto="Imóvel, carro, moto: o valor entra no patrimônio." />
          )}
        </Cartao>

        <Cartao>
          <TituloCartao
            acao={
              <Botao variante="secundario" onClick={() => setDivida("nova")}>
                <Plus className="size-4" aria-hidden /> Dívida
              </Botao>
            }
          >
            Dívidas
          </TituloCartao>
          {dados.isPending ? (
            <CarregandoLista />
          ) : p?.dividas.length ? (
            <ul className="flex flex-col divide-y divide-borda">
              {p.dividas.map((d) => (
                <li key={d.id} className="flex min-h-14 items-center gap-3 py-2">
                  <Landmark className="size-5 shrink-0 text-texto-2" aria-hidden />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{d.nome}</span>
                    <span className="block text-xs text-texto-2">
                      {d.sistema.toUpperCase()} · {fracaoParaPercentual(d.taxa_mensal)} % a.m. ·{" "}
                      {d.parcelas_restantes} de {d.prazo_meses} parcelas
                      {d.proxima_parcela
                        ? ` · próxima ${formatarData(soData(d.proxima_parcela.vencimento))}: ${formatarMoeda(d.proxima_parcela.prestacao_centavos)}`
                        : ""}
                    </span>
                  </span>
                  <span className="valor num text-sm font-semibold text-saida">
                    {formatarMoeda(d.saldo_devedor_centavos)}
                  </span>
                  <Botao variante="fantasma" aria-label={`Tabela de ${d.nome}`} onClick={() => setTabela(d)}>
                    <Table2 className="size-4" />
                  </Botao>
                  <Botao variante="fantasma" aria-label={`Editar ${d.nome}`} onClick={() => setDivida(d)}>
                    <Pencil className="size-4" />
                  </Botao>
                </li>
              ))}
            </ul>
          ) : (
            <Vazio
              titulo="Nenhuma dívida"
              texto="Financiamento ou empréstimo: o saldo devedor sai do patrimônio."
            />
          )}
        </Cartao>
      </div>

      {p?.emprestimos_banco?.length ? <EmprestimosDoBanco lista={p.emprestimos_banco} /> : null}

      <Dialogo
        aberto={bem !== null}
        aoFechar={() => setBem(null)}
        titulo={bem === "novo" ? "Novo bem" : "Editar bem"}
      >
        {bem ? <FormBem bem={bem === "novo" ? undefined : bem} aoFechar={() => setBem(null)} /> : null}
      </Dialogo>
      <Dialogo
        aberto={divida !== null}
        aoFechar={() => setDivida(null)}
        titulo={divida === "nova" ? "Nova dívida" : "Editar dívida"}
      >
        {divida ? (
          <FormDivida
            divida={divida === "nova" ? undefined : divida}
            bens={p?.bens ?? []}
            aoFechar={() => setDivida(null)}
          />
        ) : null}
      </Dialogo>
      <Dialogo
        aberto={tabela !== null}
        aoFechar={() => setTabela(null)}
        titulo={`Parcelas — ${tabela?.nome ?? ""}`}
      >
        {tabela ? <TabelaDivida divida={tabela} /> : null}
      </Dialogo>
      <AvisoSimulacao />
    </Pagina>
  );
}

const nomesModalidade: Record<string, string> = {
  LOAN: "Empréstimo",
  FINANCING: "Financiamento",
  INVOICE_FINANCING: "Parcelamento de fatura",
  UNARRANGED_ACCOUNT_OVERDRAFT: "Cheque especial",
};

/** Empréstimos e financiamentos que o banco informa pelo Open Finance (só leitura). */
function EmprestimosDoBanco({ lista }: { lista: EmprestimoBanco[] }) {
  return (
    <Cartao>
      <TituloCartao>Empréstimos segundo o banco</TituloCartao>
      <ul className="flex flex-col divide-y divide-borda" aria-label="Empréstimos segundo o banco">
        {lista.map((e) => {
          const detalhes = [
            nomesModalidade[e.modalidade] ?? e.modalidade,
            e.instituicao,
            e.parcelas_total !== null && e.parcelas_pagas !== null
              ? `${e.parcelas_pagas} de ${e.parcelas_total} parcelas pagas`
              : e.parcelas_restantes !== null
                ? `${e.parcelas_restantes} parcelas a vencer`
                : null,
            e.taxa !== null
              ? `${fracaoParaPercentual(e.taxa)} % ${e.taxa_periodicidade === "YEARLY" ? "a.a." : "a.m."}`
              : null,
            e.cet !== null ? `CET ${fracaoParaPercentual(e.cet)} %` : null,
            e.vencimento_final ? `até ${formatarData(soData(e.vencimento_final))}` : null,
          ].filter(Boolean);
          return (
            <li key={e.id} className="flex min-h-14 items-center gap-3 py-2">
              <Landmark className="size-5 shrink-0 text-texto-2" aria-hidden />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{e.nome}</span>
                <span className="block text-xs text-texto-2">{detalhes.join(" · ")}</span>
                {e.parcelas_atrasadas ? (
                  <span className="block text-xs font-semibold text-saida">
                    {e.parcelas_atrasadas}{" "}
                    {e.parcelas_atrasadas === 1 ? "parcela atrasada" : "parcelas atrasadas"}
                  </span>
                ) : null}
                {!e.conta_no_passivo ? (
                  <span className="block text-xs text-texto-2">Já está no saldo da conta ou do cartão.</span>
                ) : null}
              </span>
              {e.saldo_devedor_centavos !== null ? (
                <span className="valor num text-sm font-semibold text-saida">
                  {formatarMoeda(e.saldo_devedor_centavos)}
                </span>
              ) : null}
            </li>
          );
        })}
      </ul>
      <p className="mt-2 text-xs text-texto-2">
        Vem do banco pelo Open Finance. Se você também cadastrou o mesmo contrato em Dívidas, apague um dos
        dois para não contar duas vezes.
      </p>
    </Cartao>
  );
}

/** Barras simples do patrimônio líquido (gráficos completos na fase 7). */
function Serie({ pontos }: { pontos: PontoPatrimonio[] }) {
  const maior = Math.max(...pontos.map((p) => Math.abs(p.liquido_centavos)), 1);
  return (
    <ol className="flex h-32 items-end gap-1" aria-label="Patrimônio líquido por mês">
      {pontos.map((p) => (
        <li
          key={p.data}
          className="flex h-full min-w-0 flex-1 flex-col justify-end"
          title={`${formatarData(p.data)}: ${formatarMoeda(p.liquido_centavos)}`}
        >
          <span
            className={`block rounded-t ${p.liquido_centavos < 0 ? "bg-saida" : "bg-invest"}`}
            style={{ height: `${Math.max(2, (Math.abs(p.liquido_centavos) / maior) * 100)}%` }}
          />
          <span className="sr-only">
            {formatarData(p.data)}: {formatarMoeda(p.liquido_centavos)}
          </span>
        </li>
      ))}
    </ol>
  );
}

function FormBem({ bem, aoFechar }: { bem?: Bem; aoFechar: () => void }) {
  const invalidar = useInvalidarPlanejamento();
  const editaveis = useEditaveis();
  const [d, setD] = useState({
    entidade_id: bem?.entidade_id ?? editaveis.lista[0]?.id ?? "",
    nome: bem?.nome ?? "",
    tipo: bem?.tipo ?? "imovel",
    valor: bem ? centavosParaTexto(bem.valor_centavos) : "",
    fipe: bem?.fipe_codigo ?? "",
    notas: bem?.notas ?? "",
  });
  const salvar = useMutation({
    mutationFn: () => {
      const valor = lerCentavos(d.valor);
      if (valor === null || valor < 0) throw new Error("Valor inválido. Use o formato 1.234,56.");
      const corpo = {
        entidade_id: d.entidade_id,
        nome: d.nome,
        tipo: d.tipo,
        valor_centavos: valor,
        fipe_codigo: d.tipo === "veiculo" && d.fipe ? d.fipe : null,
        notas: d.notas || null,
      };
      return bem ? api("PATCH", `/api/bens/${bem.id}`, corpo) : api("POST", "/api/bens", corpo);
    },
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const apagar = useMutation({
    mutationFn: () => api("DELETE", `/api/bens/${bem?.id}`),
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
      {!bem ? (
        <SeletorEntidade
          entidades={editaveis.lista}
          valor={d.entidade_id}
          onChange={(v) => setD({ ...d, entidade_id: v })}
        />
      ) : null}
      <Campo
        rotulo="Nome"
        value={d.nome}
        onChange={(e) => setD({ ...d, nome: e.target.value })}
        required
        maxLength={100}
        placeholder="Apartamento"
      />
      <div className="grid grid-cols-2 gap-3">
        <Selecao
          rotulo="Tipo"
          value={d.tipo}
          onChange={(e) => setD({ ...d, tipo: e.target.value as Bem["tipo"] })}
        >
          {Object.entries(nomesBem).map(([v, n]) => (
            <option key={v} value={v}>
              {n}
            </option>
          ))}
        </Selecao>
        <Campo
          rotulo="Valor estimado"
          inputMode="decimal"
          value={d.valor}
          onChange={(e) => setD({ ...d, valor: e.target.value })}
          required
          placeholder="350.000,00"
        />
      </div>
      {d.tipo === "veiculo" ? (
        <Campo
          rotulo="Código FIPE (opcional)"
          value={d.fipe}
          onChange={(e) => setD({ ...d, fipe: e.target.value })}
          maxLength={20}
          dica="A atualização automática pela FIPE chega numa próxima versão."
        />
      ) : null}
      <Campo
        rotulo="Notas"
        value={d.notas}
        onChange={(e) => setD({ ...d, notas: e.target.value })}
        maxLength={500}
      />
      <Rodape
        apagar={bem ? () => confirm(`Apagar "${bem.nome}"?`) && apagar.mutate() : undefined}
        apagando={apagar.isPending}
        salvando={salvar.isPending}
        aoFechar={aoFechar}
      />
    </form>
  );
}

function FormDivida({ divida, bens, aoFechar }: { divida?: Divida; bens: Bem[]; aoFechar: () => void }) {
  const invalidar = useInvalidarPlanejamento();
  const editaveis = useEditaveis();
  const contas = useContas();
  const [d, setD] = useState({
    entidade_id: divida?.entidade_id ?? editaveis.lista[0]?.id ?? "",
    nome: divida?.nome ?? "",
    tipo: divida?.tipo ?? "financiamento",
    sistema: divida?.sistema ?? "price",
    principal: divida ? centavosParaTexto(divida.principal_centavos) : "",
    taxa: divida ? fracaoParaPercentual(divida.taxa_mensal) : "",
    prazo: divida?.prazo_meses.toString() ?? "",
    primeiro: divida?.primeiro_vencimento ?? "",
    conta_id: divida?.conta_id ?? "",
    bem_id: divida?.bem_id ?? "",
  });
  const salvar = useMutation({
    mutationFn: () => {
      const principal = lerCentavos(d.principal);
      const taxa = percentualParaFracao(d.taxa);
      if (principal === null || principal <= 0) throw new Error("Valor inválido. Use o formato 1.234,56.");
      if (taxa === null) throw new Error("Taxa inválida. Use o formato 0,99 (% ao mês).");
      const corpo = {
        entidade_id: d.entidade_id,
        nome: d.nome,
        tipo: d.tipo,
        sistema: d.sistema,
        principal_centavos: principal,
        taxa_mensal: taxa,
        prazo_meses: Number(d.prazo),
        primeiro_vencimento: d.primeiro,
        conta_id: d.conta_id || null,
        bem_id: d.bem_id || null,
      };
      return divida ? api("PATCH", `/api/dividas/${divida.id}`, corpo) : api("POST", "/api/dividas", corpo);
    },
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const apagar = useMutation({
    mutationFn: () => api("DELETE", `/api/dividas/${divida?.id}`),
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const erro = salvar.error ?? apagar.error;
  const pagadoras = (contas.data ?? []).filter((c) => c.pode_editar && !c.arquivada && c.tipo !== "cartao");
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        salvar.mutate();
      }}
      className="grid grid-cols-2 gap-3"
    >
      {erro ? (
        <div className="col-span-2">
          <Aviso tipo="erro">{mensagemDe(erro)}</Aviso>
        </div>
      ) : null}
      {!divida && editaveis.lista.length > 1 ? (
        <div className="col-span-2">
          <SeletorEntidade
            entidades={editaveis.lista}
            valor={d.entidade_id}
            onChange={(v) => setD({ ...d, entidade_id: v })}
          />
        </div>
      ) : null}
      <Campo
        className="col-span-2"
        rotulo="Nome"
        value={d.nome}
        onChange={(e) => setD({ ...d, nome: e.target.value })}
        required
        maxLength={100}
        placeholder="Financiamento do apartamento"
      />
      <Selecao
        rotulo="Tipo"
        value={d.tipo}
        onChange={(e) => setD({ ...d, tipo: e.target.value as Divida["tipo"] })}
      >
        {Object.entries(nomesDivida).map(([v, n]) => (
          <option key={v} value={v}>
            {n}
          </option>
        ))}
      </Selecao>
      <Selecao
        rotulo="Sistema"
        value={d.sistema}
        onChange={(e) => setD({ ...d, sistema: e.target.value as Divida["sistema"] })}
      >
        <option value="price">Price (parcela fixa)</option>
        <option value="sac">SAC (parcela cai)</option>
      </Selecao>
      <Campo
        rotulo="Valor financiado"
        inputMode="decimal"
        value={d.principal}
        onChange={(e) => setD({ ...d, principal: e.target.value })}
        required
        placeholder="200.000,00"
      />
      <Campo
        rotulo="Juros ao mês (%)"
        inputMode="decimal"
        value={d.taxa}
        onChange={(e) => setD({ ...d, taxa: e.target.value })}
        required
        placeholder="0,99"
      />
      <Campo
        rotulo="Prazo (meses)"
        type="number"
        min={1}
        max={600}
        value={d.prazo}
        onChange={(e) => setD({ ...d, prazo: e.target.value })}
        required
      />
      <Campo
        rotulo="1ª parcela"
        type="date"
        value={d.primeiro}
        onChange={(e) => setD({ ...d, primeiro: e.target.value })}
        required
      />
      <Selecao
        rotulo="Paga pela conta"
        value={d.conta_id}
        onChange={(e) => setD({ ...d, conta_id: e.target.value })}
      >
        <option value="">—</option>
        {pagadoras.map((c) => (
          <option key={c.id} value={c.id}>
            {c.nome}
          </option>
        ))}
      </Selecao>
      <Selecao rotulo="Bem ligado" value={d.bem_id} onChange={(e) => setD({ ...d, bem_id: e.target.value })}>
        <option value="">—</option>
        {bens.map((b) => (
          <option key={b.id} value={b.id}>
            {b.nome}
          </option>
        ))}
      </Selecao>
      <div className="col-span-2">
        <Rodape
          apagar={divida ? () => confirm(`Apagar "${divida.nome}"?`) && apagar.mutate() : undefined}
          apagando={apagar.isPending}
          salvando={salvar.isPending}
          aoFechar={aoFechar}
        />
      </div>
    </form>
  );
}

function TabelaDivida({ divida }: { divida: Divida }) {
  const dados = useQuery({
    queryKey: ["patrimonio", "tabela", divida.id],
    queryFn: () => obter<ParcelaDivida[]>(`/api/dividas/${divida.id}/tabela`),
  });
  if (dados.isPending) return <CarregandoLista />;
  if (dados.error) return <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso>;
  const hoje = hojeSP();
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-right text-xs">
        <thead className="text-texto-2">
          <tr>
            <th className="py-2 text-left font-medium">Nº</th>
            <th className="py-2 text-left font-medium">Vence</th>
            <th className="py-2 font-medium">Parcela</th>
            <th className="py-2 font-medium">Juros</th>
            <th className="py-2 font-medium">Saldo</th>
          </tr>
        </thead>
        <tbody className="valor num">
          {(dados.data ?? []).map((p) => (
            <tr
              key={p.numero}
              className={`border-t border-borda ${soData(p.vencimento) < hoje ? "text-texto-2" : ""}`}
            >
              <td className="py-1.5 text-left">{p.numero}</td>
              <td className="py-1.5 text-left">{formatarData(soData(p.vencimento))}</td>
              <td className="py-1.5">{formatarMoeda(p.prestacao_centavos)}</td>
              <td className="py-1.5">{formatarMoeda(p.juros_centavos)}</td>
              <td className="py-1.5">{formatarMoeda(p.saldo_devedor_centavos)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Rodape({
  apagar,
  apagando,
  salvando,
  aoFechar,
}: {
  apagar?: () => void;
  apagando: boolean;
  salvando: boolean;
  aoFechar: () => void;
}) {
  return (
    <div className="flex flex-wrap justify-between gap-2">
      {apagar ? (
        <Botao variante="perigo" carregando={apagando} onClick={apagar}>
          <Trash2 className="size-4" aria-hidden /> Apagar
        </Botao>
      ) : (
        <span />
      )}
      <div className="flex gap-2">
        <Botao variante="fantasma" onClick={aoFechar}>
          Cancelar
        </Botao>
        <Botao type="submit" carregando={salvando}>
          Salvar
        </Botao>
      </div>
    </div>
  );
}
