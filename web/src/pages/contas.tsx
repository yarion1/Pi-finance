import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Landmark, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Dialogo, Valor } from "../components/extras";
import { useComReautenticacao } from "../components/reautenticacao";
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
import { api, mensagemDe } from "../lib/api";
import { useCasas, useContas, useEntidades, useInstituicoes } from "../lib/dados";
import { centavosParaTexto, formatarMoeda, lerCentavos } from "../lib/formato";
import { type Conta, nomesTipoConta, nomesVisibilidade } from "../lib/tipos";

export function Contas() {
  const contas = useContas();
  const entidades = useEntidades();
  const [editando, setEditando] = useState<Conta | "nova" | null>(null);

  const minhas = (contas.data ?? []).filter((c) => c.entidade_nome !== null);
  const daCasa = (contas.data ?? []).filter((c) => c.entidade_nome === null);
  const grupos = new Map<string, Conta[]>();
  for (const c of minhas)
    grupos.set(c.entidade_nome ?? "", [...(grupos.get(c.entidade_nome ?? "") ?? []), c]);
  const semEntidade = entidades.data?.length === 0;

  return (
    <Pagina
      titulo="Contas e cartões"
      subtitulo="Quanto tenho em cada banco."
      acao={
        <Botao onClick={() => setEditando("nova")} disabled={semEntidade}>
          <Plus className="size-4" aria-hidden /> Nova conta
        </Botao>
      }
    >
      {semEntidade ? (
        <Aviso>
          Crie sua entidade pessoa física antes, em{" "}
          <Link to="/entidades" className="font-semibold underline">
            Entidades
          </Link>
          .
        </Aviso>
      ) : null}
      {contas.error ? <Aviso tipo="erro">{mensagemDe(contas.error)}</Aviso> : null}
      {contas.isPending ? (
        <Cartao>
          <CarregandoLista />
        </Cartao>
      ) : minhas.length === 0 && daCasa.length === 0 ? (
        <Cartao>
          <Vazio
            icone={<Landmark className="size-8" />}
            titulo="Nenhuma conta ainda"
            texto="Cadastre a conta do banco, o cartão ou o dinheiro na carteira. Depois é só importar o extrato."
          />
        </Cartao>
      ) : null}

      {[...grupos.entries()].map(([nome, lista]) => (
        <Cartao key={nome}>
          <div className="mb-2 flex items-baseline justify-between gap-3">
            <h2 className="font-semibold">{nome}</h2>
            <span className="text-sm text-texto-2">
              <span className="valor num">
                {formatarMoeda(
                  lista
                    .filter((c) => c.moeda === "BRL" && !c.arquivada)
                    .reduce((s, c) => s + (c.saldo_centavos ?? 0), 0),
                )}
              </span>
            </span>
          </div>
          <ListaContas contas={lista} aoEditar={setEditando} />
        </Cartao>
      ))}

      {daCasa.length ? (
        <Cartao>
          <h2 className="mb-2 font-semibold">Compartilhadas pela casa</h2>
          <ListaContas contas={daCasa} aoEditar={setEditando} />
        </Cartao>
      ) : null}

      <Dialogo
        aberto={editando !== null}
        aoFechar={() => setEditando(null)}
        titulo={editando === "nova" ? "Nova conta" : "Editar conta"}
      >
        {editando ? (
          <FormConta conta={editando === "nova" ? undefined : editando} aoFechar={() => setEditando(null)} />
        ) : null}
      </Dialogo>
    </Pagina>
  );
}

function ListaContas({ contas, aoEditar }: { contas: Conta[]; aoEditar: (c: Conta) => void }) {
  return (
    <ul className="flex flex-col divide-y divide-borda">
      {contas.map((c) => (
        <li key={c.id} className={`flex min-h-16 items-center gap-3 py-2 ${c.arquivada ? "opacity-60" : ""}`}>
          <span
            className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-superficie-2 text-sm font-bold text-texto-2"
            aria-hidden
          >
            {(c.instituicao ?? c.nome).slice(0, 2).toUpperCase()}
          </span>
          <Link to={`/gastos?conta=${c.id}`} className="min-w-0 flex-1">
            <span className="block truncate font-medium">{c.nome}</span>
            <span className="block truncate text-xs text-texto-2">
              {nomesTipoConta[c.tipo]}
              {c.instituicao ? ` · ${c.instituicao}` : ""}
              {c.entidade_nome === null && c.dono ? ` · de ${c.dono}` : ""}
            </span>
            <span className="mt-1 flex flex-wrap gap-1">
              {c.visibilidade !== "privada" ? (
                <Etiqueta cor="primaria">{nomesVisibilidade[c.visibilidade]}</Etiqueta>
              ) : null}
              {c.arquivada ? <Etiqueta>Arquivada</Etiqueta> : null}
            </span>
          </Link>
          {c.saldo_centavos !== null ? (
            <Valor centavos={c.saldo_centavos} moeda={c.moeda} saldo className="font-semibold" />
          ) : null}
          {c.pode_editar ? (
            <Botao variante="fantasma" aria-label={`Editar ${c.nome}`} onClick={() => aoEditar(c)}>
              <Pencil className="size-4" />
            </Botao>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function FormConta({ conta, aoFechar }: { conta?: Conta; aoFechar: () => void }) {
  const cliente = useQueryClient();
  const entidades = useEntidades();
  const instituicoes = useInstituicoes();
  const casas = useCasas();
  const comReautenticacao = useComReautenticacao();
  const editaveis = (entidades.data ?? []).filter((e) => e.papel === "dono" || e.papel === "membro");
  const [d, setD] = useState({
    entidade_id: conta?.entidade_id ?? editaveis[0]?.id ?? "",
    instituicao_id: conta?.instituicao_id ?? "",
    nome: conta?.nome ?? "",
    tipo: conta?.tipo ?? "corrente",
    moeda: conta?.moeda ?? "BRL",
    visibilidade: conta?.visibilidade ?? "privada",
    casa_id: conta?.casa_id ?? casas.data?.[0]?.id ?? "",
    saldo_inicial: conta ? centavosParaTexto(conta.saldo_inicial_centavos) : "0,00",
    fechamento: conta?.fechamento?.toString() ?? "",
    vencimento: conta?.vencimento?.toString() ?? "",
    limite: conta?.limite_centavos != null ? centavosParaTexto(conta.limite_centavos) : "",
    arquivada: conta?.arquivada ?? false,
  });
  const mudar = (k: keyof typeof d) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setD((v) => ({
      ...v,
      [k]: e.target.type === "checkbox" ? (e.target as HTMLInputElement).checked : e.target.value,
    }));

  const salvar = useMutation({
    mutationFn: () => {
      const saldo = lerCentavos(d.saldo_inicial);
      const limite = d.limite ? lerCentavos(d.limite) : null;
      if (saldo === null || (d.limite && limite === null))
        throw new Error("Valor inválido. Use o formato 1.234,56.");
      const corpo = {
        entidade_id: d.entidade_id,
        instituicao_id: d.instituicao_id || null,
        nome: d.nome,
        tipo: d.tipo,
        moeda: d.moeda,
        visibilidade: d.visibilidade,
        casa_id: d.visibilidade === "privada" ? null : d.casa_id || null,
        saldo_inicial_centavos: saldo,
        fechamento: d.tipo === "cartao" && d.fechamento ? Number(d.fechamento) : null,
        vencimento: d.tipo === "cartao" && d.vencimento ? Number(d.vencimento) : null,
        limite_centavos: d.tipo === "cartao" ? limite : null,
        arquivada: d.arquivada,
      };
      return conta ? api("PATCH", `/api/contas/${conta.id}`, corpo) : api("POST", "/api/contas", corpo);
    },
    onSuccess: async () => {
      await Promise.all([
        cliente.invalidateQueries({ queryKey: ["contas"] }),
        cliente.invalidateQueries({ queryKey: ["resumo"] }),
      ]);
      aoFechar();
    },
  });

  const apagar = useMutation({
    mutationFn: () => comReautenticacao(() => api("DELETE", `/api/contas/${conta?.id}`).then(() => true)),
    onSuccess: async (feito) => {
      if (!feito) return;
      await cliente.invalidateQueries();
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
      className="grid gap-4 sm:grid-cols-2"
    >
      {erro ? (
        <div className="sm:col-span-2">
          <Aviso tipo="erro">{mensagemDe(erro)}</Aviso>
        </div>
      ) : null}
      {!conta ? (
        <Selecao rotulo="De quem é" value={d.entidade_id} onChange={mudar("entidade_id")} required>
          {editaveis.map((e) => (
            <option key={e.id} value={e.id}>
              {e.nome}
            </option>
          ))}
        </Selecao>
      ) : null}
      <Campo
        rotulo="Nome"
        value={d.nome}
        onChange={mudar("nome")}
        required
        maxLength={100}
        placeholder="Nubank"
      />
      <Selecao rotulo="Tipo" value={d.tipo} onChange={mudar("tipo")}>
        {Object.entries(nomesTipoConta).map(([v, n]) => (
          <option key={v} value={v}>
            {n}
          </option>
        ))}
      </Selecao>
      <Selecao rotulo="Banco (opcional)" value={d.instituicao_id} onChange={mudar("instituicao_id")}>
        <option value="">—</option>
        {(instituicoes.data ?? []).map((i) => (
          <option key={i.id} value={i.id}>
            {i.nome}
          </option>
        ))}
      </Selecao>
      <Selecao rotulo="Moeda" value={d.moeda} onChange={mudar("moeda")}>
        <option value="BRL">Real (BRL)</option>
        <option value="USD">Dólar (USD)</option>
        <option value="EUR">Euro (EUR)</option>
      </Selecao>
      <Campo
        rotulo={d.tipo === "cartao" ? "Saldo inicial da fatura" : "Saldo inicial"}
        inputMode="decimal"
        value={d.saldo_inicial}
        onChange={mudar("saldo_inicial")}
        dica="Saldo antes da primeira transação importada. Negativo para dívida."
      />
      {d.tipo === "cartao" ? (
        <>
          <Campo
            rotulo="Limite"
            inputMode="decimal"
            value={d.limite}
            onChange={mudar("limite")}
            placeholder="5.000,00"
          />
          <Campo
            rotulo="Dia do fechamento"
            type="number"
            min={1}
            max={31}
            value={d.fechamento}
            onChange={mudar("fechamento")}
          />
          <Campo
            rotulo="Dia do vencimento"
            type="number"
            min={1}
            max={31}
            value={d.vencimento}
            onChange={mudar("vencimento")}
          />
        </>
      ) : null}
      <Selecao rotulo="Visibilidade" value={d.visibilidade} onChange={mudar("visibilidade")}>
        <option value="privada">Privada — só eu</option>
        <option value="saldo" disabled={!casas.data?.length}>
          Só saldo — a casa vê o total
        </option>
        <option value="compartilhada" disabled={!casas.data?.length}>
          Compartilhada — a casa vê tudo
        </option>
      </Selecao>
      {d.visibilidade !== "privada" ? (
        <Selecao rotulo="Casa" value={d.casa_id} onChange={mudar("casa_id")}>
          {(casas.data ?? []).map((c) => (
            <option key={c.id} value={c.id}>
              {c.nome}
            </option>
          ))}
        </Selecao>
      ) : null}
      {conta ? (
        <label className="flex min-h-11 items-center gap-3 text-sm sm:col-span-2">
          <input
            type="checkbox"
            className="size-5 accent-[var(--primaria)]"
            checked={d.arquivada}
            onChange={mudar("arquivada")}
          />
          Arquivada (some das listas e do total, mas guarda o histórico)
        </label>
      ) : null}
      <div className="flex flex-wrap justify-between gap-2 sm:col-span-2">
        {conta ? (
          <Botao
            variante="perigo"
            carregando={apagar.isPending}
            onClick={() => confirm(`Apagar "${conta.nome}" com todas as transações?`) && apagar.mutate()}
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
