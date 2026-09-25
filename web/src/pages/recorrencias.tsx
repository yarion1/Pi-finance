import { useMutation, useQuery } from "@tanstack/react-query";
import { Pencil, Plus, Repeat, Search, Trash2 } from "lucide-react";
import { useState } from "react";
import { Dialogo, SeletorCategoria, SeletorEntidade, Valor } from "../components/extras";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  Selecao,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { useCategorias, useContas, useEditaveis, useInvalidarPlanejamento } from "../lib/dados";
import { centavosParaTexto, formatarData, formatarMoeda, lerCentavos } from "../lib/formato";
import { nomesFrequencia, nomesTipoRecorrencia, type Recorrencia } from "../lib/tipos";

/** Quanto a recorrência pesa por mês (anual ÷ 12, semanal × 52 ÷ 12). */
function porMes(r: Recorrencia): number {
  if (r.frequencia === "anual") return Math.round(r.valor_centavos / 12);
  if (r.frequencia === "semanal") return Math.round((r.valor_centavos * 52) / 12);
  return r.valor_centavos;
}

export function Recorrencias() {
  const dados = useQuery({
    queryKey: ["recorrencias"],
    queryFn: () => obter<Recorrencia[]>("/api/recorrencias"),
  });
  const editaveis = useEditaveis();
  const invalidar = useInvalidarPlanejamento();
  const [editando, setEditando] = useState<Recorrencia | "nova" | null>(null);
  const detectar = useMutation({
    mutationFn: async () => {
      let novas = 0;
      for (const e of editaveis.lista) {
        const r = await api<{ novas: number }>("POST", "/api/recorrencias/detectar", { entidade_id: e.id });
        novas += r.novas;
      }
      return novas;
    },
    onSuccess: invalidar,
  });

  const lista = dados.data ?? [];
  const ativas = lista.filter((r) => r.ativa);
  const inativas = lista.filter((r) => !r.ativa);
  const assinaturas = ativas.filter((r) => r.tipo === "assinatura").reduce((s, r) => s + porMes(r), 0);
  const fixas = ativas.filter((r) => r.tipo === "conta_fixa").reduce((s, r) => s + porMes(r), 0);
  const erro = dados.error ?? detectar.error;

  return (
    <Pagina
      titulo="Recorrências"
      subtitulo="Assinaturas, contas fixas e receitas que se repetem."
      acao={
        <div className="flex flex-wrap gap-2">
          <Botao variante="secundario" carregando={detectar.isPending} onClick={() => detectar.mutate()}>
            <Search className="size-4" aria-hidden /> Procurar no histórico
          </Botao>
          <Botao onClick={() => setEditando("nova")} disabled={editaveis.lista.length === 0}>
            <Plus className="size-4" aria-hidden /> Nova
          </Botao>
        </div>
      }
    >
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      {detectar.data !== undefined ? (
        <Aviso tipo="sucesso">
          {detectar.data === 0
            ? "Nenhuma recorrência nova no histórico."
            : `${detectar.data} ${detectar.data === 1 ? "recorrência nova encontrada" : "recorrências novas encontradas"}.`}
        </Aviso>
      ) : null}

      {ativas.length ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Cartao>
            <p className="text-sm text-texto-2">Assinaturas por mês</p>
            <p className="valor num mt-1 text-2xl font-bold">{formatarMoeda(-assinaturas)}</p>
          </Cartao>
          <Cartao>
            <p className="text-sm text-texto-2">Contas fixas por mês</p>
            <p className="valor num mt-1 text-2xl font-bold">{formatarMoeda(-fixas)}</p>
          </Cartao>
        </div>
      ) : null}

      <Cartao>
        <TituloCartao>Ativas</TituloCartao>
        {dados.isPending ? (
          <CarregandoLista />
        ) : ativas.length ? (
          <ListaRecorrencias lista={ativas} aoEditar={setEditando} />
        ) : (
          <Vazio
            icone={<Repeat className="size-8" />}
            titulo="Nenhuma recorrência"
            texto="Depois de 3 meses de extrato, o painel encontra sozinho as cobranças que se repetem."
          />
        )}
      </Cartao>

      {inativas.length ? (
        <Cartao>
          <TituloCartao>Canceladas ou pausadas</TituloCartao>
          <ListaRecorrencias lista={inativas} aoEditar={setEditando} />
        </Cartao>
      ) : null}

      <Dialogo
        aberto={editando !== null}
        aoFechar={() => setEditando(null)}
        titulo={editando === "nova" ? "Nova recorrência" : "Editar recorrência"}
      >
        {editando ? (
          <FormRecorrencia
            recorrencia={editando === "nova" ? undefined : editando}
            aoFechar={() => setEditando(null)}
          />
        ) : null}
      </Dialogo>
    </Pagina>
  );
}

function ListaRecorrencias({
  lista,
  aoEditar,
}: {
  lista: Recorrencia[];
  aoEditar: (r: Recorrencia) => void;
}) {
  return (
    <ul className="flex flex-col divide-y divide-borda">
      {lista.map((r) => (
        <li key={r.id} className={`flex min-h-16 items-center gap-3 py-2 ${r.ativa ? "" : "opacity-60"}`}>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{r.descricao}</span>
            <span className="block truncate text-xs text-texto-2">
              {nomesFrequencia[r.frequencia]} · próxima {formatarData(r.proxima)}
              {r.conta ? ` · ${r.conta}` : ""}
            </span>
            <span className="mt-1 flex flex-wrap gap-1">
              <Etiqueta>{nomesTipoRecorrencia[r.tipo]}</Etiqueta>
              {r.detectada ? <Etiqueta cor="primaria">Detectada</Etiqueta> : null}
              {r.subiu && r.valor_anterior_centavos !== null ? (
                <Etiqueta cor="alerta">
                  Subiu de {formatarMoeda(Math.abs(r.valor_anterior_centavos))}
                </Etiqueta>
              ) : null}
              {r.atrasada && r.ativa ? <Etiqueta cor="alerta">Não apareceu — cancelou?</Etiqueta> : null}
            </span>
          </span>
          <Valor centavos={r.valor_centavos} className="text-sm font-semibold" />
          <Botao variante="fantasma" aria-label={`Editar ${r.descricao}`} onClick={() => aoEditar(r)}>
            <Pencil className="size-4" />
          </Botao>
        </li>
      ))}
    </ul>
  );
}

function FormRecorrencia({ recorrencia: r, aoFechar }: { recorrencia?: Recorrencia; aoFechar: () => void }) {
  const invalidar = useInvalidarPlanejamento();
  const editaveis = useEditaveis();
  const contas = useContas();
  const categorias = useCategorias();
  const [d, setD] = useState({
    entidade_id: r?.entidade_id ?? editaveis.lista[0]?.id ?? "",
    descricao: r?.descricao ?? "",
    tipo: r?.tipo ?? "assinatura",
    valor: r ? centavosParaTexto(Math.abs(r.valor_centavos)) : "",
    frequencia: r?.frequencia ?? "mensal",
    proxima: r?.proxima ?? "",
    conta_id: r?.conta_id ?? "",
    categoria_id: r?.categoria_id ?? "",
    ativa: r?.ativa ?? true,
  });
  const daEntidade = (contas.data ?? []).filter((c) => c.entidade_id === d.entidade_id && !c.arquivada);
  const salvar = useMutation({
    mutationFn: () => {
      const v = lerCentavos(d.valor);
      if (v === null || v === 0) throw new Error("Valor inválido. Use o formato 1.234,56.");
      const corpo = {
        entidade_id: d.entidade_id,
        descricao: d.descricao,
        tipo: d.tipo,
        valor_centavos: d.tipo === "receita" ? Math.abs(v) : -Math.abs(v),
        frequencia: d.frequencia,
        proxima: d.proxima,
        conta_id: d.conta_id || null,
        categoria_id: d.categoria_id || null,
        ativa: d.ativa,
      };
      return r ? api("PATCH", `/api/recorrencias/${r.id}`, corpo) : api("POST", "/api/recorrencias", corpo);
    },
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });
  const apagar = useMutation({
    mutationFn: () => api("DELETE", `/api/recorrencias/${r?.id}`),
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
      {!r ? (
        <SeletorEntidade
          entidades={editaveis.lista}
          valor={d.entidade_id}
          onChange={(v) => setD({ ...d, entidade_id: v, conta_id: "" })}
        />
      ) : null}
      <Campo
        rotulo="Descrição"
        value={d.descricao}
        onChange={(e) => setD({ ...d, descricao: e.target.value })}
        required
        maxLength={100}
        placeholder="Netflix"
      />
      <div className="grid grid-cols-2 gap-3">
        <Selecao
          rotulo="Tipo"
          value={d.tipo}
          onChange={(e) => setD({ ...d, tipo: e.target.value as Recorrencia["tipo"] })}
        >
          {Object.entries(nomesTipoRecorrencia).map(([v, n]) => (
            <option key={v} value={v}>
              {n}
            </option>
          ))}
        </Selecao>
        <Campo
          rotulo="Valor"
          inputMode="decimal"
          value={d.valor}
          onChange={(e) => setD({ ...d, valor: e.target.value })}
          required
          placeholder="44,90"
        />
        <Selecao
          rotulo="Frequência"
          value={d.frequencia}
          onChange={(e) => setD({ ...d, frequencia: e.target.value as Recorrencia["frequencia"] })}
        >
          {Object.entries(nomesFrequencia).map(([v, n]) => (
            <option key={v} value={v}>
              {n}
            </option>
          ))}
        </Selecao>
        <Campo
          rotulo="Próxima cobrança"
          type="date"
          value={d.proxima}
          onChange={(e) => setD({ ...d, proxima: e.target.value })}
          required
        />
      </div>
      <Selecao
        rotulo="Conta ou cartão"
        value={d.conta_id}
        onChange={(e) => setD({ ...d, conta_id: e.target.value })}
      >
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
      {r ? (
        <label className="flex min-h-11 items-center gap-3 text-sm">
          <input
            type="checkbox"
            className="size-5 accent-[var(--primaria)]"
            checked={d.ativa}
            onChange={(e) => setD({ ...d, ativa: e.target.checked })}
          />
          Ativa (desmarque quando cancelar a assinatura)
        </label>
      ) : null}
      <div className="flex flex-wrap justify-between gap-2">
        {r ? (
          <Botao
            variante="perigo"
            carregando={apagar.isPending}
            onClick={() => confirm(`Apagar "${r.descricao}"?`) && apagar.mutate()}
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
