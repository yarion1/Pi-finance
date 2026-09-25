import { useMutation, useQuery } from "@tanstack/react-query";
import { CalendarDays, Check, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { AvisoSimulacao, Dialogo, SeletorCategoria, SeletorEntidade, Valor } from "../components/extras";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Esqueleto,
  Etiqueta,
  Pagina,
  Selecao,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { useCategorias, useContas, useEditaveis, useInvalidarPlanejamento } from "../lib/dados";
import { centavosParaTexto, formatarData, formatarMoeda, hojeSP, lerCentavos } from "../lib/formato";
import type { Compromisso, ItemAgenda, Agenda as TAgenda } from "../lib/tipos";

const nomesOrigem: Record<ItemAgenda["origem"], string> = {
  recorrencia: "Recorrente",
  fatura: "Fatura",
  divida: "Parcela de dívida",
  compromisso: "A pagar/receber",
  lancamento: "Lançamento futuro",
};

const diaSemana = new Intl.DateTimeFormat("pt-BR", {
  weekday: "short",
  day: "2-digit",
  month: "2-digit",
  timeZone: "UTC",
});

export function Agenda() {
  const [dias, setDias] = useState(30);
  const agenda = useQuery({
    queryKey: ["agenda", dias],
    queryFn: () => obter<TAgenda>(`/api/agenda?dias=${dias}`),
  });
  const compromissos = useQuery({
    queryKey: ["compromissos"],
    queryFn: () => obter<Compromisso[]>("/api/compromissos"),
  });
  const [editando, setEditando] = useState<Compromisso | "novo" | null>(null);
  const invalidar = useInvalidarPlanejamento();
  const pagar = useMutation({
    mutationFn: (c: Compromisso) => api("PATCH", `/api/compromissos/${c.id}`, { ...c, pago_em: hojeSP() }),
    onSuccess: invalidar,
  });

  const a = agenda.data;
  const itens = a?.itens ?? [];
  const saldoDoDia = new Map((a?.saldos ?? []).map((s) => [s.data, s.saldo_centavos]));
  const porDia = new Map<string, ItemAgenda[]>();
  for (const i of itens) porDia.set(i.data, [...(porDia.get(i.data) ?? []), i]);
  const pendentes = (compromissos.data ?? []).filter((c) => !c.pago_em);
  const erro = agenda.error ?? compromissos.error ?? pagar.error;

  return (
    <Pagina
      titulo="Agenda"
      subtitulo="O que vence e quanto sobra na conta."
      acao={
        <Botao onClick={() => setEditando("novo")}>
          <Plus className="size-4" aria-hidden /> A pagar ou receber
        </Botao>
      }
    >
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Numero titulo="Saldo hoje" valor={a?.saldo_inicial_centavos} carregando={agenda.isPending} />
        <Numero
          titulo={`Saldo em ${dias} dias`}
          valor={a?.saldo_final_centavos}
          carregando={agenda.isPending}
        />
        <Numero
          titulo="Menor saldo no período"
          valor={a?.menor_saldo.saldo_centavos}
          carregando={agenda.isPending}
          dica={a ? `em ${formatarData(a.menor_saldo.data)}` : undefined}
        />
      </div>
      {a && a.menor_saldo.saldo_centavos < 0 ? (
        <Aviso tipo="alerta">
          O saldo projetado fica negativo em {formatarData(a.menor_saldo.data)}. Antecipe uma receita ou adie
          um pagamento.
        </Aviso>
      ) : null}
      <p className="text-xs text-texto-2">
        Saldo das contas correntes, poupança, carteiras e benefícios. Compras no cartão entram pela fatura.
      </p>

      <fieldset className="flex gap-2">
        <legend className="sr-only">Período</legend>
        {[30, 60, 90].map((n) => (
          <Botao
            key={n}
            variante={n === dias ? "primario" : "secundario"}
            onClick={() => setDias(n)}
            aria-pressed={n === dias}
          >
            {n} dias
          </Botao>
        ))}
      </fieldset>

      <Cartao>
        <TituloCartao>Próximos vencimentos</TituloCartao>
        {agenda.isPending ? (
          <CarregandoLista linhas={5} />
        ) : itens.length === 0 ? (
          <Vazio
            icone={<CalendarDays className="size-8" />}
            titulo="Nada previsto"
            texto={
              <>
                Faturas, assinaturas e parcelas aparecem aqui. Veja as{" "}
                <Link to="/recorrencias" className="font-semibold text-primaria">
                  recorrências
                </Link>
                .
              </>
            }
          />
        ) : (
          <ol className="flex flex-col gap-4" aria-label="Agenda por dia">
            {[...porDia.entries()].map(([data, lista]) => {
              const saldo = saldoDoDia.get(data);
              return (
                <li key={data}>
                  <h3 className="flex items-baseline justify-between gap-3 border-b border-borda pb-1 text-sm font-semibold">
                    <span className="capitalize">{diaSemana.format(new Date(`${data}T00:00:00Z`))}</span>
                    {saldo !== undefined ? (
                      <span className="text-xs font-normal text-texto-2">
                        saldo <Valor centavos={saldo} saldo />
                      </span>
                    ) : null}
                  </h3>
                  <ul className="flex flex-col">
                    {lista.map((i) => {
                      const comp =
                        i.origem === "compromisso" ? pendentes.find((c) => c.id === i.id) : undefined;
                      return (
                        <li
                          key={`${i.origem}-${i.id}-${i.data}`}
                          className="flex min-h-12 items-center gap-3 py-1"
                        >
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-sm">{i.descricao}</span>
                            <span className="flex flex-wrap items-center gap-1 text-xs text-texto-2">
                              {nomesOrigem[i.origem]}
                              {i.conta ? ` · ${i.conta}` : ""}
                              {i.no_cartao ? <Etiqueta>no cartão</Etiqueta> : null}
                              {i.atrasado ? <Etiqueta cor="alerta">atrasado</Etiqueta> : null}
                            </span>
                          </span>
                          <Valor centavos={i.valor_centavos} className="text-sm font-medium" />
                          {comp ? (
                            <Botao
                              variante="fantasma"
                              aria-label={`Marcar ${i.descricao} como pago`}
                              title="Marcar como pago"
                              onClick={() => pagar.mutate(comp)}
                            >
                              <Check className="size-4" />
                            </Botao>
                          ) : null}
                        </li>
                      );
                    })}
                  </ul>
                </li>
              );
            })}
          </ol>
        )}
      </Cartao>

      {pendentes.length ? (
        <Cartao>
          <TituloCartao>Contas a pagar e a receber</TituloCartao>
          <ul className="flex flex-col divide-y divide-borda">
            {pendentes.map((c) => (
              <li key={c.id} className="flex min-h-12 items-center gap-3 py-1.5">
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm">{c.descricao}</span>
                  <span className="block text-xs text-texto-2">vence {formatarData(c.vencimento)}</span>
                </span>
                <Valor centavos={c.valor_centavos} className="text-sm" />
                <Botao
                  variante="fantasma"
                  aria-label={`Editar ${c.descricao}`}
                  onClick={() => setEditando(c)}
                >
                  <Pencil className="size-4" />
                </Botao>
              </li>
            ))}
          </ul>
        </Cartao>
      ) : null}

      <Dialogo
        aberto={editando !== null}
        aoFechar={() => setEditando(null)}
        titulo={editando === "novo" ? "Conta a pagar ou receber" : "Editar"}
      >
        {editando ? (
          <FormCompromisso
            compromisso={editando === "novo" ? undefined : editando}
            aoFechar={() => setEditando(null)}
          />
        ) : null}
      </Dialogo>
      <AvisoSimulacao />
    </Pagina>
  );
}

function Numero({
  titulo,
  valor,
  carregando,
  dica,
}: {
  titulo: string;
  valor?: number;
  carregando: boolean;
  dica?: string;
}) {
  return (
    <Cartao>
      <p className="text-sm text-texto-2">{titulo}</p>
      {carregando ? (
        <Esqueleto className="mt-2 h-8 w-32" />
      ) : (
        <p className={`valor num mt-1 text-2xl font-bold ${(valor ?? 0) < 0 ? "text-saida" : ""}`}>
          {formatarMoeda(valor ?? 0)}
        </p>
      )}
      {dica ? <p className="mt-1 text-xs text-texto-2">{dica}</p> : null}
    </Cartao>
  );
}

function FormCompromisso({ compromisso, aoFechar }: { compromisso?: Compromisso; aoFechar: () => void }) {
  const invalidar = useInvalidarPlanejamento();
  const editaveis = useEditaveis();
  const contas = useContas();
  const categorias = useCategorias();
  const [d, setD] = useState({
    entidade_id: compromisso?.entidade_id ?? editaveis.lista[0]?.id ?? "",
    descricao: compromisso?.descricao ?? "",
    valor: compromisso ? centavosParaTexto(Math.abs(compromisso.valor_centavos)) : "",
    pagar: compromisso ? compromisso.valor_centavos < 0 : true,
    vencimento: compromisso?.vencimento ?? "",
    conta_id: compromisso?.conta_id ?? "",
    categoria_id: compromisso?.categoria_id ?? "",
  });
  const daEntidade = (contas.data ?? []).filter((c) => c.entidade_id === d.entidade_id && !c.arquivada);
  const salvar = useMutation({
    mutationFn: () => {
      const v = lerCentavos(d.valor);
      if (v === null || v === 0) throw new Error("Valor inválido. Use o formato 1.234,56.");
      const corpo = {
        entidade_id: d.entidade_id,
        descricao: d.descricao,
        valor_centavos: d.pagar ? -Math.abs(v) : Math.abs(v),
        vencimento: d.vencimento,
        conta_id: d.conta_id || null,
        categoria_id: d.categoria_id || null,
        pago_em: compromisso?.pago_em ?? null,
      };
      return compromisso
        ? api("PATCH", `/api/compromissos/${compromisso.id}`, corpo)
        : api("POST", "/api/compromissos", corpo);
    },
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const apagar = useMutation({
    mutationFn: () => api("DELETE", `/api/compromissos/${compromisso?.id}`),
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
      {!compromisso ? (
        <SeletorEntidade
          entidades={editaveis.lista}
          valor={d.entidade_id}
          onChange={(v) => setD({ ...d, entidade_id: v, conta_id: "" })}
        />
      ) : null}
      <Selecao
        rotulo="É para"
        value={d.pagar ? "pagar" : "receber"}
        onChange={(e) => setD({ ...d, pagar: e.target.value === "pagar" })}
      >
        <option value="pagar">Pagar</option>
        <option value="receber">Receber</option>
      </Selecao>
      <Campo
        rotulo="Descrição"
        value={d.descricao}
        onChange={(e) => setD({ ...d, descricao: e.target.value })}
        required
        maxLength={200}
        placeholder="IPVA"
      />
      <div className="grid grid-cols-2 gap-3">
        <Campo
          rotulo="Valor"
          inputMode="decimal"
          value={d.valor}
          onChange={(e) => setD({ ...d, valor: e.target.value })}
          required
          placeholder="1.200,00"
        />
        <Campo
          rotulo="Vencimento"
          type="date"
          value={d.vencimento}
          onChange={(e) => setD({ ...d, vencimento: e.target.value })}
          required
        />
      </div>
      <Selecao rotulo="Conta" value={d.conta_id} onChange={(e) => setD({ ...d, conta_id: e.target.value })}>
        <option value="">—</option>
        {daEntidade.map((c) => (
          <option key={c.id} value={c.id}>
            {c.nome}
          </option>
        ))}
      </Selecao>
      <SeletorCategoria
        categorias={categorias.data ?? []}
        entidadeId={d.entidade_id}
        valor={d.categoria_id}
        onChange={(v) => setD({ ...d, categoria_id: v })}
      />
      <div className="flex flex-wrap justify-between gap-2">
        {compromisso ? (
          <Botao
            variante="perigo"
            carregando={apagar.isPending}
            onClick={() => confirm(`Apagar "${compromisso.descricao}"?`) && apagar.mutate()}
          >
            <Trash2 className="size-4" aria-hidden /> Apagar
          </Botao>
        ) : (
          <span />
        )}
        <div className="flex gap-2">
          <Botao variante="fantasma" onClick={aoFechar}>
            Cancelar
          </Botao>
          <Botao type="submit" carregando={salvar.isPending}>
            Salvar
          </Botao>
        </div>
      </div>
    </form>
  );
}
