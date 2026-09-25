import { useMutation, useQuery } from "@tanstack/react-query";
import { Pencil, Plus, Target, Trash2 } from "lucide-react";
import { useState } from "react";
import { AvisoSimulacao, Barra, Dialogo, SeletorEntidade } from "../components/extras";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  Selecao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { useContas, useEditaveis, useInvalidarPlanejamento } from "../lib/dados";
import {
  centavosParaTexto,
  formatarData,
  formatarMoeda,
  formatarPercentual,
  lerCentavos,
} from "../lib/formato";
import { type CustoDeVida, type Meta, nomesTipoMeta } from "../lib/tipos";

type RespostaMetas = { metas: Meta[]; custo_de_vida: CustoDeVida };

export function Metas() {
  const dados = useQuery({ queryKey: ["metas"], queryFn: () => obter<RespostaMetas>("/api/metas") });
  const editaveis = useEditaveis();
  const [editando, setEditando] = useState<Meta | "nova" | null>(null);
  const metas = dados.data?.metas ?? [];
  const custo = dados.data?.custo_de_vida.medio_6_meses_centavos ?? 0;

  return (
    <Pagina
      titulo="Metas"
      subtitulo="Quanto falta e quanto guardar por mês."
      acao={
        <Botao onClick={() => setEditando("nova")} disabled={editaveis.lista.length === 0}>
          <Plus className="size-4" aria-hidden /> Nova meta
        </Botao>
      }
    >
      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}
      {dados.isPending ? (
        <Cartao>
          <CarregandoLista />
        </Cartao>
      ) : metas.length === 0 ? (
        <Cartao>
          <Vazio
            icone={<Target className="size-8" />}
            titulo="Nenhuma meta ainda"
            texto="Comece pela reserva de emergência: 6 meses do seu custo de vida."
            acao={
              <Botao onClick={() => setEditando("nova")} disabled={editaveis.lista.length === 0}>
                Criar meta
              </Botao>
            }
          />
        </Cartao>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {metas.map((m) => (
            <CartaoMeta key={m.id} meta={m} aoEditar={() => setEditando(m)} />
          ))}
        </div>
      )}
      {custo > 0 ? (
        <p className="text-sm text-texto-2">
          Custo de vida médio (6 meses): <span className="valor num">{formatarMoeda(custo)}</span> por mês.
        </p>
      ) : null}

      <Dialogo
        aberto={editando !== null}
        aoFechar={() => setEditando(null)}
        titulo={editando === "nova" ? "Nova meta" : "Editar meta"}
      >
        {editando ? (
          <FormMeta
            meta={editando === "nova" ? undefined : editando}
            custo={custo}
            aoFechar={() => setEditando(null)}
          />
        ) : null}
      </Dialogo>
      <AvisoSimulacao />
    </Pagina>
  );
}

function CartaoMeta({ meta: m, aoEditar }: { meta: Meta; aoEditar: () => void }) {
  const falta = Math.max(m.alvo_centavos - m.atual_centavos, 0);
  return (
    <Cartao>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="truncate font-semibold">{m.nome}</h2>
          <p className="text-xs text-texto-2">
            {nomesTipoMeta[m.tipo]}
            {m.data_alvo ? ` · até ${formatarData(m.data_alvo)}` : ""}
          </p>
        </div>
        <Botao variante="fantasma" aria-label={`Editar ${m.nome}`} onClick={aoEditar}>
          <Pencil className="size-4" />
        </Botao>
      </div>
      <p className="mt-3 flex flex-wrap items-baseline justify-between gap-2">
        <span className="valor num text-2xl font-bold">{formatarMoeda(m.atual_centavos)}</span>
        <span className="valor num text-sm text-texto-2">de {formatarMoeda(m.alvo_centavos)}</span>
      </p>
      <Barra
        fracao={m.percentual}
        cor={m.concluida ? "var(--entrada)" : "var(--invest)"}
        rotulo={`Progresso de ${m.nome}`}
      />
      <dl className="mt-3 grid grid-cols-2 gap-3 text-sm">
        <div>
          <dt className="text-xs text-texto-2">Progresso</dt>
          <dd className="valor num font-medium">{formatarPercentual(m.percentual, 0)}</dd>
        </div>
        <div>
          <dt className="text-xs text-texto-2">Falta</dt>
          <dd className="valor num font-medium">{formatarMoeda(falta)}</dd>
        </div>
        {m.data_alvo && !m.concluida ? (
          <div>
            <dt className="text-xs text-texto-2">Guardar por mês</dt>
            <dd className="valor num font-medium">{formatarMoeda(m.aporte_necessario_centavos)}</dd>
            <dd className="text-xs text-texto-2">
              {m.meses_restantes} {m.meses_restantes === 1 ? "mês" : "meses"} restantes
            </dd>
          </div>
        ) : null}
        {m.data_prevista && !m.concluida ? (
          <div>
            <dt className="text-xs text-texto-2">No ritmo planejado</dt>
            <dd className="font-medium">{formatarData(m.data_prevista)}</dd>
          </div>
        ) : null}
        {m.tipo === "reserva" && m.meses_de_custo_de_vida !== null ? (
          <div>
            <dt className="text-xs text-texto-2">Cobre</dt>
            <dd className="valor num font-medium">
              {m.meses_de_custo_de_vida.toLocaleString("pt-BR", { maximumFractionDigits: 1 })} meses
            </dd>
          </div>
        ) : null}
      </dl>
      {m.concluida ? (
        <div className="mt-3">
          <Etiqueta cor="entrada">Meta alcançada</Etiqueta>
        </div>
      ) : null}
    </Cartao>
  );
}

function FormMeta({ meta, custo, aoFechar }: { meta?: Meta; custo: number; aoFechar: () => void }) {
  const invalidar = useInvalidarPlanejamento();
  const editaveis = useEditaveis();
  const contas = useContas();
  const [d, setD] = useState({
    entidade_id: meta?.entidade_id ?? editaveis.lista[0]?.id ?? "",
    nome: meta?.nome ?? "",
    tipo: meta?.tipo ?? "reserva",
    alvo: meta ? centavosParaTexto(meta.alvo_centavos) : custo > 0 ? centavosParaTexto(custo * 6) : "",
    data_alvo: meta?.data_alvo ?? "",
    contas: meta?.contas_vinculadas ?? [],
    manual: meta ? centavosParaTexto(meta.valor_manual_centavos) : "",
    aporte: meta?.aporte_planejado_centavos != null ? centavosParaTexto(meta.aporte_planejado_centavos) : "",
  });
  const disponiveis = (contas.data ?? []).filter((c) => !c.arquivada && c.tipo !== "cartao");

  const salvar = useMutation({
    mutationFn: () => {
      const alvo = lerCentavos(d.alvo);
      const manual = d.manual.trim() ? lerCentavos(d.manual) : 0;
      const aporte = d.aporte.trim() ? lerCentavos(d.aporte) : null;
      if (alvo === null || alvo <= 0 || manual === null || (d.aporte.trim() && aporte === null))
        throw new Error("Valor inválido. Use o formato 1.234,56.");
      const corpo = {
        entidade_id: d.entidade_id,
        nome: d.nome,
        tipo: d.tipo,
        alvo_centavos: alvo,
        data_alvo: d.data_alvo || null,
        contas_vinculadas: d.contas,
        valor_manual_centavos: manual,
        aporte_planejado_centavos: aporte,
      };
      return meta ? api("PATCH", `/api/metas/${meta.id}`, corpo) : api("POST", "/api/metas", corpo);
    },
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const apagar = useMutation({
    mutationFn: () => api("DELETE", `/api/metas/${meta?.id}`),
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
      {!meta ? (
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
        placeholder="Reserva de emergência"
      />
      <div className="grid grid-cols-2 gap-3">
        <Selecao
          rotulo="Tipo"
          value={d.tipo}
          onChange={(e) => setD({ ...d, tipo: e.target.value as Meta["tipo"] })}
        >
          {Object.entries(nomesTipoMeta).map(([v, n]) => (
            <option key={v} value={v}>
              {n}
            </option>
          ))}
        </Selecao>
        <Campo
          rotulo="Até quando (opcional)"
          type="date"
          value={d.data_alvo}
          onChange={(e) => setD({ ...d, data_alvo: e.target.value })}
        />
      </div>
      <Campo
        rotulo="Quanto quer juntar"
        inputMode="decimal"
        value={d.alvo}
        onChange={(e) => setD({ ...d, alvo: e.target.value })}
        required
        placeholder="15.000,00"
        dica={
          d.tipo === "reserva" && custo > 0
            ? `6 meses do seu custo de vida: ${formatarMoeda(custo * 6)}`
            : undefined
        }
      />
      <fieldset className="flex flex-col gap-1.5">
        <legend className="mb-1.5 text-sm font-medium">Contas que guardam este dinheiro</legend>
        {disponiveis.length === 0 ? <p className="text-sm text-texto-2">Nenhuma conta cadastrada.</p> : null}
        {disponiveis.map((c) => (
          <label key={c.id} className="flex min-h-11 items-center gap-3 text-sm">
            <input
              type="checkbox"
              className="size-5 accent-[var(--primaria)]"
              checked={d.contas.includes(c.id)}
              onChange={(e) =>
                setD({
                  ...d,
                  contas: e.target.checked ? [...d.contas, c.id] : d.contas.filter((x) => x !== c.id),
                })
              }
            />
            <span className="min-w-0 flex-1 truncate">{c.nome}</span>
            {c.saldo_centavos !== null ? (
              <span className="valor num text-texto-2">{formatarMoeda(c.saldo_centavos, c.moeda)}</span>
            ) : null}
          </label>
        ))}
      </fieldset>
      <div className="grid grid-cols-2 gap-3">
        <Campo
          rotulo="Valor fora das contas"
          inputMode="decimal"
          value={d.manual}
          onChange={(e) => setD({ ...d, manual: e.target.value })}
          placeholder="0,00"
        />
        <Campo
          rotulo="Aporte por mês"
          inputMode="decimal"
          value={d.aporte}
          onChange={(e) => setD({ ...d, aporte: e.target.value })}
          placeholder="500,00"
          dica="Para prever quando chega lá."
        />
      </div>
      <div className="flex flex-wrap justify-between gap-2">
        {meta ? (
          <Botao
            variante="perigo"
            carregando={apagar.isPending}
            onClick={() => confirm(`Apagar a meta "${meta.nome}"?`) && apagar.mutate()}
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
