import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2, Wand2 } from "lucide-react";
import { useState } from "react";
import { SeletorCategoria } from "../components/extras";
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
import { arvoreCategorias, useCategorias, useEntidades } from "../lib/dados";
import type { Categoria, Regra } from "../lib/tipos";

const nomesTipo = { gasto: "Gastos", receita: "Receitas", transferencia: "Transferências" } as const;

export function Categorias() {
  const entidades = useEntidades();
  const editaveis = (entidades.data ?? []).filter((e) => e.papel === "dono" || e.papel === "membro");
  const [entidadeId, setEntidadeId] = useState("");
  const entidade = entidadeId || editaveis[0]?.id || "";

  return (
    <Pagina
      titulo="Categorias e regras"
      subtitulo="Regras categorizam cada importação; o que você corrige vira histórico."
    >
      {editaveis.length > 1 ? (
        <Selecao
          rotulo="Entidade"
          value={entidade}
          onChange={(e) => setEntidadeId(e.target.value)}
          className="max-w-xs"
        >
          {editaveis.map((e) => (
            <option key={e.id} value={e.id}>
              {e.nome}
            </option>
          ))}
        </Selecao>
      ) : null}
      {entidade ? (
        <>
          <Regras entidadeId={entidade} />
          <ArvoreCategorias entidadeId={entidade} />
        </>
      ) : entidades.isPending ? (
        <CarregandoLista />
      ) : (
        <Aviso>Crie sua entidade pessoa física antes.</Aviso>
      )}
    </Pagina>
  );
}

function Regras({ entidadeId }: { entidadeId: string }) {
  const cliente = useQueryClient();
  const categorias = useCategorias();
  const regras = useQuery({
    queryKey: ["regras", entidadeId],
    queryFn: () => obter<Regra[]>(`/api/regras?entidade_id=${entidadeId}`),
  });
  const [texto, setTexto] = useState("");
  const [categoria, setCategoria] = useState("");
  const invalidar = () => cliente.invalidateQueries({ queryKey: ["regras", entidadeId] });
  const criar = useMutation({
    mutationFn: () => api("POST", "/api/regras", { entidade_id: entidadeId, texto, categoria_id: categoria }),
    onSuccess: () => {
      setTexto("");
      return invalidar();
    },
  });
  const apagar = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/regras/${id}`),
    onSuccess: invalidar,
  });
  const aplicar = useMutation({
    mutationFn: () =>
      api<{ categorizadas: number }>("POST", "/api/regras/aplicar", { entidade_id: entidadeId }),
    onSuccess: () =>
      Promise.all(["transacoes", "resumo"].map((k) => cliente.invalidateQueries({ queryKey: [k] }))),
  });
  const erro = criar.error ?? apagar.error ?? aplicar.error;

  return (
    <Cartao>
      <TituloCartao
        acao={
          <Botao variante="secundario" carregando={aplicar.isPending} onClick={() => aplicar.mutate()}>
            <Wand2 className="size-4" aria-hidden /> Aplicar nas sem categoria
          </Botao>
        }
      >
        Regras
      </TituloCartao>
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      {aplicar.data ? (
        <Aviso tipo="sucesso">
          {aplicar.data.categorizadas}{" "}
          {aplicar.data.categorizadas === 1 ? "transação categorizada" : "transações categorizadas"}.
        </Aviso>
      ) : null}
      {regras.isPending ? (
        <CarregandoLista linhas={2} />
      ) : regras.data?.length ? (
        <ul className="my-2 flex flex-col divide-y divide-borda">
          {regras.data.map((r) => (
            <li key={r.id} className="flex min-h-12 items-center gap-3 text-sm">
              <span className="min-w-0 flex-1">
                contém <strong className="font-semibold">“{r.texto}”</strong> → {r.categoria}
              </span>
              <Botao
                variante="fantasma"
                aria-label={`Apagar regra ${r.texto}`}
                onClick={() => apagar.mutate(r.id)}
              >
                <Trash2 className="size-4" />
              </Botao>
            </li>
          ))}
        </ul>
      ) : (
        <Vazio
          titulo="Nenhuma regra"
          texto="Ex.: descrição contém “UBER” → Transporte › App. Também dá para criar ao categorizar várias transações de uma vez."
        />
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          criar.mutate();
        }}
        className="mt-3 grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
      >
        <Campo
          rotulo="Descrição contém"
          value={texto}
          onChange={(e) => setTexto(e.target.value)}
          required
          maxLength={100}
          placeholder="UBER"
        />
        <SeletorCategoria
          categorias={categorias.data ?? []}
          entidadeId={entidadeId}
          valor={categoria}
          onChange={setCategoria}
          vazio="Escolha…"
        />
        <Botao type="submit" disabled={!categoria || !texto.trim()} carregando={criar.isPending}>
          <Plus className="size-4" aria-hidden /> Criar
        </Botao>
      </form>
    </Cartao>
  );
}

function ArvoreCategorias({ entidadeId }: { entidadeId: string }) {
  const cliente = useQueryClient();
  const categorias = useCategorias();
  const [nova, setNova] = useState<{ pai: string; tipo: Categoria["tipo"] } | null>(null);
  const [nome, setNome] = useState("");
  const invalidar = () => cliente.invalidateQueries({ queryKey: ["categorias"] });
  const criar = useMutation({
    mutationFn: () =>
      api("POST", "/api/categorias", {
        entidade_id: entidadeId,
        nome,
        pai_id: nova?.pai || null,
        tipo: nova?.tipo,
      }),
    onSuccess: () => {
      setNome("");
      setNova(null);
      return invalidar();
    },
  });
  const apagar = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/categorias/${id}`),
    onSuccess: invalidar,
  });
  const arvore = arvoreCategorias(categorias.data ?? [], entidadeId);

  return (
    <Cartao>
      <TituloCartao>Categorias</TituloCartao>
      {(criar.error ?? apagar.error) ? (
        <Aviso tipo="erro">{mensagemDe(criar.error ?? apagar.error)}</Aviso>
      ) : null}
      {(Object.keys(nomesTipo) as Categoria["tipo"][]).map((tipo) => (
        <section key={tipo} className="mt-3">
          <div className="mb-1 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-texto-2">{nomesTipo[tipo]}</h3>
            <Botao variante="fantasma" onClick={() => setNova({ pai: "", tipo })}>
              <Plus className="size-4" aria-hidden /> Categoria
            </Botao>
          </div>
          <ul className="flex flex-col gap-1">
            {arvore
              .filter((c) => c.tipo === tipo)
              .map((m) => (
                <li key={m.id} className="rounded-xl border border-borda px-3 py-2">
                  <div className="flex min-h-9 items-center gap-2">
                    <span
                      className="size-3 rounded-full"
                      style={{ backgroundColor: m.cor ?? "var(--borda)" }}
                      aria-hidden
                    />
                    <span className="flex-1 font-medium">{m.nome}</span>
                    {m.entidade_id ? <Etiqueta>sua</Etiqueta> : null}
                    <Botao
                      variante="fantasma"
                      aria-label={`Nova subcategoria de ${m.nome}`}
                      onClick={() => setNova({ pai: m.id, tipo })}
                    >
                      <Plus className="size-4" />
                    </Botao>
                    {m.entidade_id ? (
                      <Botao
                        variante="fantasma"
                        aria-label={`Apagar ${m.nome}`}
                        onClick={() => confirm(`Apagar "${m.nome}"?`) && apagar.mutate(m.id)}
                      >
                        <Trash2 className="size-4" />
                      </Botao>
                    ) : null}
                  </div>
                  {m.filhas.length ? (
                    <ul className="mt-1 flex flex-wrap gap-1.5 pl-5">
                      {m.filhas.map((f) => (
                        <li
                          key={f.id}
                          className="inline-flex items-center gap-1 rounded-full border border-borda px-2.5 py-1 text-xs"
                        >
                          {f.nome}
                          {f.entidade_id ? (
                            <button
                              type="button"
                              aria-label={`Apagar ${f.nome}`}
                              className="text-texto-2 hover:text-saida"
                              onClick={() => confirm(`Apagar "${f.nome}"?`) && apagar.mutate(f.id)}
                            >
                              ×
                            </button>
                          ) : null}
                        </li>
                      ))}
                    </ul>
                  ) : null}
                  {nova?.pai === m.id ? (
                    <FormNome
                      nome={nome}
                      setNome={setNome}
                      carregando={criar.isPending}
                      aoSalvar={() => criar.mutate()}
                      aoCancelar={() => setNova(null)}
                      rotulo={`Nova em ${m.nome}`}
                    />
                  ) : null}
                </li>
              ))}
          </ul>
          {nova && nova.pai === "" && nova.tipo === tipo ? (
            <FormNome
              nome={nome}
              setNome={setNome}
              carregando={criar.isPending}
              aoSalvar={() => criar.mutate()}
              aoCancelar={() => setNova(null)}
              rotulo="Nova categoria"
            />
          ) : null}
        </section>
      ))}
    </Cartao>
  );
}

function FormNome({
  nome,
  setNome,
  carregando,
  aoSalvar,
  aoCancelar,
  rotulo,
}: {
  nome: string;
  setNome: (s: string) => void;
  carregando: boolean;
  aoSalvar: () => void;
  aoCancelar: () => void;
  rotulo: string;
}) {
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        aoSalvar();
      }}
      className="mt-2 flex flex-wrap items-end gap-2"
    >
      <Campo
        rotulo={rotulo}
        className="flex-1"
        value={nome}
        onChange={(e) => setNome(e.target.value)}
        required
        maxLength={100}
        autoFocus
      />
      <Botao variante="fantasma" onClick={aoCancelar}>
        Cancelar
      </Botao>
      <Botao type="submit" carregando={carregando}>
        Criar
      </Botao>
    </form>
  );
}
