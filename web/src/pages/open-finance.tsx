import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Landmark, Link2, Plug, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { SeletorEntidade } from "../components/extras";
import { useComReautenticacao } from "../components/reautenticacao";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { useContas, useEditaveis } from "../lib/dados";
import { formatarDataHora, formatarMoeda } from "../lib/formato";
import type {
  ContaPluggy,
  ItemPluggy,
  RelatorioSincronizacao,
  OpenFinance as TOpenFinance,
} from "../lib/tipos";

function useInvalidarTudo() {
  const cliente = useQueryClient();
  return () => cliente.invalidateQueries();
}

export function OpenFinance() {
  const dados = useQuery({
    queryKey: ["open-finance"],
    queryFn: () => obter<TOpenFinance>("/api/open-finance"),
  });
  const invalidar = useInvalidarTudo();
  const sincronizar = useMutation({
    mutationFn: () => api<RelatorioSincronizacao>("POST", "/api/open-finance/sincronizar"),
    onSuccess: invalidar,
  });
  const c = dados.data?.conexao;
  const itens = dados.data?.itens ?? [];

  return (
    <Pagina
      titulo="Open Finance"
      subtitulo="Extratos e saldos direto do banco, pelo Meu Pluggy."
      acao={
        c ? (
          <Botao
            carregando={sincronizar.isPending}
            onClick={() => sincronizar.mutate()}
            disabled={itens.length === 0}
          >
            <RefreshCw className="size-4" aria-hidden /> Sincronizar agora
          </Botao>
        ) : null
      }
    >
      {dados.error ? <Aviso tipo="erro">{mensagemDe(dados.error)}</Aviso> : null}
      {sincronizar.error ? <Aviso tipo="erro">{mensagemDe(sincronizar.error)}</Aviso> : null}
      {sincronizar.data ? <Resumo r={sincronizar.data} /> : null}

      {dados.isPending ? (
        <Cartao>
          <CarregandoLista />
        </Cartao>
      ) : !c ? (
        <PassoAPasso />
      ) : (
        <>
          <Credenciais conexao={c} />
          {itens.map((it) => (
            <CartaoItem key={it.id} item={it} />
          ))}
          <NovoBanco primeiro={itens.length === 0} />
        </>
      )}
      <p className="text-center text-xs text-texto-2">
        O painel só lê: nenhuma senha de banco passa por aqui. As credenciais do Meu Pluggy ficam cifradas e
        só você as usa. A sincronização automática roda uma vez por dia.
      </p>
    </Pagina>
  );
}

function Resumo({ r }: { r: RelatorioSincronizacao }) {
  const partes = [
    `${r.novas} ${r.novas === 1 ? "transação nova" : "transações novas"}`,
    r.casadas ? `${r.casadas} já estavam no painel (arquivo ou manual)` : "",
    r.transferencias ? `${r.transferencias} transferências entre contas` : "",
    r.contas_sem_vinculo
      ? `${r.contas_sem_vinculo} ${r.contas_sem_vinculo === 1 ? "conta espera" : "contas esperam"} vínculo`
      : "",
  ].filter(Boolean);
  return (
    <>
      <Aviso tipo={r.divergencias ? "alerta" : "sucesso"}>
        Sincronizado: {partes.join(" · ")}.
        {r.divergencias
          ? ` ${r.divergencias} ${r.divergencias === 1 ? "conta ficou" : "contas ficaram"} com saldo diferente do banco.`
          : ""}
      </Aviso>
      {r.erros?.map((e) => (
        <Aviso key={e} tipo="erro">
          {e}
        </Aviso>
      ))}
    </>
  );
}

function PassoAPasso() {
  return (
    <>
      <Cartao>
        <TituloCartao>Como conectar</TituloCartao>
        <ol className="flex list-decimal flex-col gap-2 pl-5 text-sm">
          <li>
            Crie sua conta gratuita em <strong>meu.pluggy.ai</strong> e conecte seus bancos lá (até 5
            conexões, só contas em seu nome).
          </li>
          <li>
            No Meu Pluggy, copie o <strong>Client ID</strong> e o <strong>Client Secret</strong> e cole
            abaixo.
          </li>
          <li>
            Copie o <strong>id de cada banco conectado</strong> (o item) e adicione aqui, escolhendo de quem
            são as contas.
          </li>
          <li>Escolha, para cada conta, se liga a uma conta que já existe no painel ou cria uma nova.</li>
        </ol>
      </Cartao>
      <FormCredenciais />
    </>
  );
}

function FormCredenciais({ aoSalvar }: { aoSalvar?: () => void }) {
  const invalidar = useInvalidarTudo();
  const comReautenticacao = useComReautenticacao();
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const salvar = useMutation({
    mutationFn: () =>
      comReautenticacao(() =>
        api("PUT", "/api/open-finance/credenciais", { client_id: clientId, client_secret: secret }).then(
          () => true,
        ),
      ),
    onSuccess: async (feito) => {
      if (!feito) return;
      setSecret("");
      await invalidar();
      aoSalvar?.();
    },
  });
  return (
    <Cartao>
      <TituloCartao>Credenciais do Meu Pluggy</TituloCartao>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          salvar.mutate();
        }}
        className="flex flex-col gap-4"
      >
        {salvar.error ? <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso> : null}
        <Campo
          rotulo="Client ID"
          value={clientId}
          onChange={(e) => setClientId(e.target.value)}
          required
          maxLength={200}
          autoComplete="off"
          spellCheck={false}
        />
        <Campo
          rotulo="Client Secret"
          type="password"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          required
          maxLength={200}
          autoComplete="off"
          dica="Conferimos com o Meu Pluggy antes de guardar. Pede sua senha de novo."
        />
        <div className="flex justify-end">
          <Botao type="submit" carregando={salvar.isPending}>
            <Plug className="size-4" aria-hidden /> Conectar
          </Botao>
        </div>
      </form>
    </Cartao>
  );
}

function Credenciais({ conexao: c }: { conexao: NonNullable<TOpenFinance["conexao"]> }) {
  const invalidar = useInvalidarTudo();
  const comReautenticacao = useComReautenticacao();
  const [trocando, setTrocando] = useState(false);
  const desconectar = useMutation({
    mutationFn: () => comReautenticacao(() => api("DELETE", "/api/open-finance").then(() => true)),
    onSuccess: async (feito) => {
      if (feito) await invalidar();
    },
  });
  if (trocando) return <FormCredenciais aoSalvar={() => setTrocando(false)} />;
  return (
    <Cartao>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-semibold">Meu Pluggy conectado</h2>
          <p className="text-sm text-texto-2">
            Client ID terminado em <span className="font-mono">{c.client_id_fim}</span>
            {c.ultima_sync
              ? ` · última sincronização ${formatarDataHora(c.ultima_sync)}`
              : " · ainda não sincronizado"}
          </p>
        </div>
        <div className="flex gap-2">
          <Botao variante="secundario" onClick={() => setTrocando(true)}>
            Trocar
          </Botao>
          <Botao
            variante="perigo"
            carregando={desconectar.isPending}
            onClick={() =>
              confirm("Desconectar o Meu Pluggy? As transações já importadas continuam no painel.") &&
              desconectar.mutate()
            }
          >
            Desconectar
          </Botao>
        </div>
      </div>
      {c.ultimo_erro ? (
        <div className="mt-3">
          <Aviso tipo="erro">Última sincronização falhou: {c.ultimo_erro}</Aviso>
        </div>
      ) : null}
      {desconectar.error ? (
        <div className="mt-3">
          <Aviso tipo="erro">{mensagemDe(desconectar.error)}</Aviso>
        </div>
      ) : null}
    </Cartao>
  );
}

function NovoBanco({ primeiro }: { primeiro: boolean }) {
  const invalidar = useInvalidarTudo();
  const editaveis = useEditaveis();
  const [item, setItem] = useState("");
  const [entidade, setEntidade] = useState("");
  const entidadeId = entidade || editaveis.lista[0]?.id || "";
  const adicionar = useMutation({
    mutationFn: async () => {
      await api("POST", "/api/open-finance/itens", { item_id: item, entidade_id: entidadeId });
      // já traz as contas desse banco para escolher o vínculo
      return api<RelatorioSincronizacao>("POST", "/api/open-finance/sincronizar").catch(() => undefined);
    },
    onSuccess: async () => {
      setItem("");
      await invalidar();
    },
  });
  if (editaveis.lista.length === 0) {
    return (
      <Aviso>
        Crie sua entidade pessoa física em{" "}
        <Link to="/entidades" className="font-semibold underline">
          Entidades
        </Link>{" "}
        antes de adicionar um banco.
      </Aviso>
    );
  }
  return (
    <Cartao>
      <TituloCartao>{primeiro ? "Adicione o primeiro banco" : "Adicionar banco"}</TituloCartao>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          adicionar.mutate();
        }}
        className="flex flex-col gap-4"
      >
        {adicionar.error ? <Aviso tipo="erro">{mensagemDe(adicionar.error)}</Aviso> : null}
        <Campo
          rotulo="Id do item no Meu Pluggy"
          value={item}
          onChange={(e) => setItem(e.target.value)}
          required
          maxLength={100}
          autoComplete="off"
          spellCheck={false}
          placeholder="ex.: 5a1c1b2e-…"
        />
        <SeletorEntidade entidades={editaveis.lista} valor={entidadeId} onChange={setEntidade} />
        <div className="flex justify-end">
          <Botao type="submit" carregando={adicionar.isPending}>
            <Landmark className="size-4" aria-hidden /> Adicionar banco
          </Botao>
        </div>
      </form>
    </Cartao>
  );
}

function CartaoItem({ item: it }: { item: ItemPluggy }) {
  const invalidar = useInvalidarTudo();
  const remover = useMutation({
    mutationFn: () => api("DELETE", `/api/open-finance/itens/${it.id}`),
    onSuccess: invalidar,
  });
  return (
    <Cartao>
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-lg font-semibold">{it.instituicao ?? it.item_id}</h2>
          <p className="text-xs text-texto-2">
            {it.entidade ? `Contas de ${it.entidade}` : ""}
            {it.atualizado_em ? ` · banco lido em ${formatarDataHora(it.atualizado_em)}` : ""}
          </p>
        </div>
        <Botao
          variante="fantasma"
          aria-label={`Remover ${it.instituicao ?? it.item_id}`}
          carregando={remover.isPending}
          onClick={() =>
            confirm("Remover este banco? As transações já importadas continuam no painel.") &&
            remover.mutate()
          }
        >
          <Trash2 className="size-4" />
        </Botao>
      </div>
      {it.ultimo_erro ? (
        <div className="mb-3">
          <Aviso tipo="erro">{it.ultimo_erro}</Aviso>
        </div>
      ) : null}
      {it.contas.length === 0 ? (
        <Vazio titulo="Nenhuma conta ainda" texto="Sincronize para buscar as contas deste banco." />
      ) : (
        <ul
          className="flex flex-col divide-y divide-borda"
          aria-label={`Contas de ${it.instituicao ?? it.item_id}`}
        >
          {it.contas.map((c) => (
            <LinhaConta key={c.id} conta={c} entidadeId={it.entidade_id} />
          ))}
        </ul>
      )}
    </Cartao>
  );
}

function LinhaConta({ conta: c, entidadeId }: { conta: ContaPluggy; entidadeId: string }) {
  const invalidar = useInvalidarTudo();
  const contas = useContas();
  const cartao = c.tipo === "CREDIT";
  const candidatas = (contas.data ?? []).filter(
    (x) =>
      x.entidade_id === entidadeId &&
      x.pode_editar &&
      !x.arquivada &&
      !x.open_finance &&
      (x.tipo === "cartao") === cartao,
  );
  const [destino, setDestino] = useState("criar");
  const decidir = useMutation({
    mutationFn: (corpo: { acao: string; conta_id?: string }) =>
      api("PATCH", `/api/open-finance/contas/${c.id}`, corpo),
    onSuccess: invalidar,
  });
  return (
    <li className="flex flex-col gap-2 py-3">
      <div className="flex items-baseline justify-between gap-3">
        <span className="min-w-0">
          <span className="block truncate font-medium">{c.nome}</span>
          <span className="block text-xs text-texto-2">
            {cartao ? "Cartão de crédito" : "Conta"}
            {c.numero ? ` · ${c.numero}` : ""}
          </span>
        </span>
        {c.saldo_centavos !== null ? (
          <span className="shrink-0 text-right">
            <span
              className={`valor num block text-sm font-semibold ${c.saldo_centavos < 0 ? "text-saida" : ""}`}
            >
              {formatarMoeda(c.saldo_centavos, c.moeda)}
            </span>
            <span className="block text-xs text-texto-2">
              {cartao ? "fatura no banco" : "saldo no banco"}
            </span>
          </span>
        ) : null}
      </div>
      {decidir.error ? <Aviso tipo="erro">{mensagemDe(decidir.error)}</Aviso> : null}
      {c.conta_id ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="flex items-center gap-1.5 text-sm">
            <Link2 className="size-4 text-entrada" aria-hidden />
            Ligada a{" "}
            <Link to={`/gastos?conta=${c.conta_id}`} className="font-semibold text-primaria">
              {c.conta}
            </Link>
          </span>
          <Botao variante="fantasma" onClick={() => decidir.mutate({ acao: "desvincular" })}>
            Desfazer vínculo
          </Botao>
        </div>
      ) : c.ignorada ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Etiqueta>Ignorada</Etiqueta>
          <Botao variante="fantasma" onClick={() => decidir.mutate({ acao: "desvincular" })}>
            Voltar a considerar
          </Botao>
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (destino === "criar" || destino === "ignorar") decidir.mutate({ acao: destino });
            else decidir.mutate({ acao: "vincular", conta_id: destino });
          }}
          className="flex flex-wrap items-end gap-2"
        >
          <label className="flex min-w-0 flex-1 flex-col gap-1.5">
            <span className="text-sm font-medium">O que fazer com esta conta</span>
            <select
              value={destino}
              onChange={(e) => setDestino(e.target.value)}
              className="min-h-11 rounded-xl border border-borda bg-superficie-2 px-3 text-base text-texto focus:border-primaria focus:outline-none"
            >
              <option value="criar">Criar conta nova no painel</option>
              {candidatas.map((x) => (
                <option key={x.id} value={x.id}>
                  Ligar a “{x.nome}”
                </option>
              ))}
              <option value="ignorar">Ignorar</option>
            </select>
          </label>
          <Botao type="submit" carregando={decidir.isPending}>
            Confirmar
          </Botao>
        </form>
      )}
    </li>
  );
}
