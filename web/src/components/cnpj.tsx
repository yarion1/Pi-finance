import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileUp } from "lucide-react";
import { useState } from "react";
import { api, mensagemDe, obter } from "../lib/api";
import { lerBase64 } from "../lib/dados";
import {
  centavosParaTexto,
  decimalBR,
  formatarMoeda,
  formatarPercentual,
  hojeSP,
  lerCentavos,
} from "../lib/formato";
import type { AvisoDistribuicao, FolhaMes, SimulacaoCNPJ } from "../lib/tipos";
import { Dialogo } from "./extras";
import { Aviso, Botao, Campo, Etiqueta, Selecao } from "./ui";

/** Tudo que muda quando o CNPJ muda. */
export function useInvalidarCNPJ() {
  const cliente = useQueryClient();
  return () =>
    Promise.all(
      ["cnpj-painel", "cnpj-notas", "cnpj-folha", "cnpj-distribuicoes", "agenda"].map((k) =>
        cliente.invalidateQueries({ queryKey: [k] }),
      ),
    );
}

export function DialogoReceita({
  entidade,
  aberto,
  aoFechar,
}: {
  entidade: string;
  aberto: boolean;
  aoFechar: () => void;
}) {
  const invalidar = useInvalidarCNPJ();
  const vazio = {
    data: hojeSP(),
    valor: "",
    cliente: "",
    numero: "",
    descricao: "",
    moeda: "BRL",
    valorMoeda: "",
    cambio: "",
    pais: "BR",
    exportacao: false,
  };
  const [f, setF] = useState(vazio);
  const estrangeira = f.moeda !== "BRL";
  const valor = lerCentavos(f.valor);
  const valorMoeda = lerCentavos(f.valorMoeda);
  const cambio = f.cambio ? decimalBR(f.cambio) : "";
  const valido = estrangeira
    ? valorMoeda !== null && valorMoeda > 0 && cambio !== null
    : valor !== null && valor > 0;

  const salvar = useMutation({
    mutationFn: () =>
      api("POST", `/api/cnpj/${entidade}/notas`, {
        data_emissao: f.data,
        cliente: f.cliente,
        numero: f.numero,
        descricao: f.descricao,
        moeda: f.moeda,
        pais: f.pais,
        exportacao: estrangeira ? f.exportacao : false,
        ...(estrangeira
          ? { valor_moeda_centavos: valorMoeda ?? 0, taxa_cambio: cambio ?? "" }
          : { valor_centavos: valor ?? 0 }),
      }),
    onSuccess: async () => {
      await invalidar();
      setF(vazio);
      aoFechar();
    },
  });

  return (
    <Dialogo aberto={aberto} aoFechar={aoFechar} titulo="Nova receita">
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          salvar.mutate();
        }}
      >
        <div className="grid grid-cols-2 gap-3">
          <Campo
            rotulo="Data"
            type="date"
            value={f.data}
            onChange={(e) => setF({ ...f, data: e.target.value })}
            required
          />
          <Selecao
            rotulo="Moeda"
            value={f.moeda}
            onChange={(e) =>
              setF({
                ...f,
                moeda: e.target.value,
                pais: e.target.value === "BRL" ? "BR" : f.pais === "BR" ? "US" : f.pais,
                exportacao: e.target.value !== "BRL",
              })
            }
          >
            <option value="BRL">Real</option>
            <option value="USD">Dólar</option>
            <option value="EUR">Euro</option>
            <option value="GBP">Libra</option>
          </Selecao>
          {estrangeira ? (
            <>
              <Campo
                rotulo={`Valor em ${f.moeda}`}
                inputMode="decimal"
                value={f.valorMoeda}
                onChange={(e) => setF({ ...f, valorMoeda: e.target.value })}
              />
              <Campo
                rotulo="Câmbio do recebimento"
                inputMode="decimal"
                placeholder={f.moeda === "USD" ? "vazio = PTAX" : "5,40"}
                value={f.cambio}
                onChange={(e) => setF({ ...f, cambio: e.target.value })}
                erro={f.cambio && !cambio ? "Número inválido" : undefined}
              />
            </>
          ) : (
            <Campo
              rotulo="Valor (R$)"
              inputMode="decimal"
              value={f.valor}
              onChange={(e) => setF({ ...f, valor: e.target.value })}
              className="col-span-2"
            />
          )}
          <Campo
            rotulo="Cliente (opcional)"
            value={f.cliente}
            onChange={(e) => setF({ ...f, cliente: e.target.value })}
            maxLength={200}
            className="col-span-2"
          />
          <Campo
            rotulo="Nº da nota (opcional)"
            value={f.numero}
            onChange={(e) => setF({ ...f, numero: e.target.value })}
            maxLength={60}
          />
          <Campo
            rotulo="País do cliente"
            value={f.pais}
            maxLength={2}
            onChange={(e) => setF({ ...f, pais: e.target.value.toUpperCase() })}
          />
          <Campo
            rotulo="Descrição (opcional)"
            value={f.descricao}
            onChange={(e) => setF({ ...f, descricao: e.target.value })}
            maxLength={500}
            className="col-span-2"
          />
        </div>
        {estrangeira ? (
          <label className="flex min-h-11 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={f.exportacao}
              onChange={(e) => setF({ ...f, exportacao: e.target.checked })}
              className="size-5 accent-[var(--primaria)]"
            />
            Exportação de serviço (cliente e resultado no exterior, com entrada de divisas)
          </label>
        ) : null}
        {salvar.error ? <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso> : null}
        <div className="flex justify-end">
          <Botao type="submit" carregando={salvar.isPending} disabled={!valido}>
            Salvar receita
          </Botao>
        </div>
      </form>
    </Dialogo>
  );
}

export function DialogoNFSe({
  entidade,
  aberto,
  aoFechar,
}: {
  entidade: string;
  aberto: boolean;
  aoFechar: () => void;
}) {
  const invalidar = useInvalidarCNPJ();
  const [arquivos, setArquivos] = useState<{ nome: string; conteudo: string }[]>([]);
  const enviar = useMutation({
    mutationFn: () =>
      api<{ novas: number; duplicadas: number; erros: string[]; avisos: string[] }>(
        "POST",
        `/api/cnpj/${entidade}/notas/xml`,
        { arquivos },
      ),
    onSuccess: async () => {
      setArquivos([]);
      await invalidar();
    },
  });
  const r = enviar.data;
  return (
    <Dialogo
      aberto={aberto}
      aoFechar={() => {
        enviar.reset();
        setArquivos([]);
        aoFechar();
      }}
      titulo="Importar NFS-e"
    >
      <div className="flex flex-col gap-4">
        <p className="text-sm text-texto-2">
          No Emissor Nacional (nfse.gov.br), em Notas emitidas, baixe o XML de cada nota. Pode escolher vários
          de uma vez; nota já importada não duplica.
        </p>
        <label className="flex min-h-24 cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-borda bg-superficie-2 p-4 text-center text-sm hover:border-texto-2">
          <FileUp className="size-6 text-texto-2" aria-hidden />
          <span>
            {arquivos.length
              ? `${arquivos.length} ${arquivos.length === 1 ? "arquivo" : "arquivos"}`
              : "Escolher arquivos .xml"}
          </span>
          <input
            type="file"
            accept=".xml,text/xml,application/xml"
            multiple
            className="sr-only"
            onChange={async (e) => {
              enviar.reset();
              const lista = [...(e.target.files ?? [])].slice(0, 100);
              setArquivos(
                await Promise.all(lista.map(async (f) => ({ nome: f.name, conteudo: await lerBase64(f) }))),
              );
            }}
          />
        </label>
        {enviar.error ? <Aviso tipo="erro">{mensagemDe(enviar.error)}</Aviso> : null}
        {r ? (
          <Aviso tipo={r.erros.length ? "alerta" : "sucesso"}>
            <span className="block">
              {r.novas} {r.novas === 1 ? "nota importada" : "notas importadas"}
              {r.duplicadas ? `, ${r.duplicadas} já estavam` : ""}.
            </span>
            {[...r.avisos, ...r.erros].map((m) => (
              <span key={m} className="block text-xs">
                {m}
              </span>
            ))}
          </Aviso>
        ) : null}
        <div className="flex justify-end">
          <Botao onClick={() => enviar.mutate()} carregando={enviar.isPending} disabled={!arquivos.length}>
            Importar
          </Botao>
        </div>
      </div>
    </Dialogo>
  );
}

/** Continuar MEI ou migrar para ME, com a receita mensal projetada. */
export function Simulador({ atividade, receitaInicial }: { atividade: string; receitaInicial: number }) {
  const [receita, setReceita] = useState(receitaInicial > 0 ? centavosParaTexto(receitaInicial) : "");
  const [contador, setContador] = useState("300,00");
  const r = lerCentavos(receita);
  const c = lerCentavos(contador);
  const pronto = r !== null && r >= 0 && c !== null && c >= 0;
  const sim = useQuery({
    queryKey: ["cnpj-simulacao", atividade, r, c],
    queryFn: () =>
      obter<SimulacaoCNPJ>(
        `/api/cnpj/simulacao?atividade=${atividade}&receita_mensal_centavos=${r}&contador_centavos=${c}`,
      ),
    enabled: pronto,
  });
  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-3">
        <Campo
          rotulo="Receita por mês (R$)"
          inputMode="decimal"
          value={receita}
          onChange={(e) => setReceita(e.target.value)}
        />
        <Campo
          rotulo="Contador por mês (R$)"
          inputMode="decimal"
          value={contador}
          onChange={(e) => setContador(e.target.value)}
        />
      </div>
      {sim.error ? <Aviso tipo="erro">{mensagemDe(sim.error)}</Aviso> : null}
      {sim.data ? (
        <>
          <p className="text-sm text-texto-2">
            {formatarMoeda(sim.data.receita_anual_centavos)} em 12 meses. O MEI aceita até{" "}
            {formatarMoeda(sim.data.teto_mei_centavos)} por ano (
            {formatarMoeda(sim.data.receita_mensal_maxima_mei_centavos)} por mês).
          </p>
          <ul className="grid gap-3 sm:grid-cols-3">
            {sim.data.cenarios.map((x) => (
              <li
                key={x.nome}
                className={`flex flex-col gap-1 rounded-xl border p-3 text-sm ${
                  x.nome === sim.data.melhor ? "border-primaria" : "border-borda"
                } ${x.possivel ? "" : "opacity-60"}`}
              >
                <span className="font-semibold">{x.nome}</span>
                {x.nome === sim.data.melhor ? <Etiqueta cor="primaria">mais barato</Etiqueta> : null}
                <span className="valor num text-xl font-bold">{formatarMoeda(x.total_mensal_centavos)}</span>
                <span className="text-xs text-texto-2">
                  por mês ({formatarMoeda(x.total_anual_centavos)} no ano)
                </span>
                <span className="text-xs text-texto-2">
                  Impostos {formatarMoeda(x.impostos_centavos)}
                  {x.aliquota_efetiva ? ` (${formatarPercentual(Number(x.aliquota_efetiva), 2)})` : ""}
                  {x.inss_anual_centavos ? ` · INSS ${formatarMoeda(x.inss_anual_centavos)}` : ""}
                  {x.irrf_anual_centavos ? ` · IRRF ${formatarMoeda(x.irrf_anual_centavos)}` : ""}
                  {x.contador_anual_centavos ? ` · contador ${formatarMoeda(x.contador_anual_centavos)}` : ""}
                </span>
                {x.observacao ? <span className="text-xs text-texto-2">{x.observacao}</span> : null}
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  );
}

export function FormFolha({ entidade }: { entidade: string }) {
  const invalidar = useInvalidarCNPJ();
  const folha = useQuery({
    queryKey: ["cnpj-folha", entidade],
    queryFn: () => obter<FolhaMes[]>(`/api/cnpj/${entidade}/folha`),
  });
  const [f, setF] = useState({ competencia: hojeSP().slice(0, 7), prolabore: "", salarios: "" });
  const pl = f.prolabore ? lerCentavos(f.prolabore) : 0;
  const sal = f.salarios ? lerCentavos(f.salarios) : 0;
  const salvar = useMutation({
    mutationFn: () =>
      api<FolhaMes>("PUT", `/api/cnpj/${entidade}/folha`, {
        competencia: f.competencia,
        prolabore_centavos: pl ?? 0,
        salarios_centavos: sal ?? 0,
      }),
    onSuccess: invalidar,
  });
  return (
    <div className="flex flex-col gap-3">
      <form
        className="grid grid-cols-2 gap-3 sm:grid-cols-4"
        onSubmit={(e) => {
          e.preventDefault();
          salvar.mutate();
        }}
      >
        <Campo
          rotulo="Competência"
          type="month"
          value={f.competencia}
          onChange={(e) => setF({ ...f, competencia: e.target.value })}
        />
        <Campo
          rotulo="Pró-labore (R$)"
          inputMode="decimal"
          value={f.prolabore}
          onChange={(e) => setF({ ...f, prolabore: e.target.value })}
        />
        <Campo
          rotulo="Salários (R$)"
          inputMode="decimal"
          value={f.salarios}
          onChange={(e) => setF({ ...f, salarios: e.target.value })}
        />
        <Botao
          type="submit"
          className="self-end"
          carregando={salvar.isPending}
          disabled={pl === null || sal === null}
        >
          Salvar mês
        </Botao>
      </form>
      {salvar.error ? <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso> : null}
      {salvar.data ? (
        <p className="text-xs text-texto-2">
          INSS do sócio {formatarMoeda(salvar.data.inss_centavos)} · IRRF{" "}
          {formatarMoeda(salvar.data.irrf_centavos)}
        </p>
      ) : null}
      {folha.data?.length ? (
        <ul className="divide-y divide-borda rounded-xl border border-borda text-sm">
          {folha.data.map((m) => (
            <li key={m.competencia} className="flex justify-between gap-2 px-3 py-2">
              <span>{m.competencia.split("-").reverse().join("/")}</span>
              <span className="valor num text-right">
                {formatarMoeda(m.prolabore_centavos + m.salarios_centavos)}
                <span className="block text-xs text-texto-2">
                  INSS {formatarMoeda(m.inss_centavos)} · IRRF {formatarMoeda(m.irrf_centavos)}
                </span>
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function FormLucro({ entidade }: { entidade: string }) {
  const invalidar = useInvalidarCNPJ();
  const [f, setF] = useState({ data: hojeSP(), valor: "", descricao: "" });
  const valor = lerCentavos(f.valor);
  const corpo = (simular: boolean) => ({
    data: f.data,
    valor_centavos: valor ?? 0,
    descricao: f.descricao,
    simular,
  });
  const simular = useMutation({
    mutationFn: () => api<AvisoDistribuicao>("POST", `/api/cnpj/${entidade}/distribuicoes`, corpo(true)),
  });
  const registrar = useMutation({
    mutationFn: () => api<AvisoDistribuicao>("POST", `/api/cnpj/${entidade}/distribuicoes`, corpo(false)),
    onSuccess: async () => {
      setF({ ...f, valor: "", descricao: "" });
      simular.reset();
      await invalidar();
    },
  });
  const av = simular.data;
  return (
    <div className="flex flex-col gap-3">
      <div className="grid grid-cols-2 gap-3">
        <Campo
          rotulo="Data"
          type="date"
          value={f.data}
          onChange={(e) => {
            setF({ ...f, data: e.target.value });
            simular.reset();
          }}
        />
        <Campo
          rotulo="Valor (R$)"
          inputMode="decimal"
          value={f.valor}
          onChange={(e) => {
            setF({ ...f, valor: e.target.value });
            simular.reset();
          }}
        />
      </div>
      {av ? (
        <Aviso tipo={av.irrf_centavos > 0 ? "alerta" : "info"}>
          {av.irrf_centavos > 0
            ? `Passa de ${formatarMoeda(av.limite_centavos)} no mês: IRRF de 10 % sobre o total, ${formatarMoeda(av.irrf_centavos)}.`
            : `Sem IRRF. Ainda cabem ${formatarMoeda(av.folga_centavos)} neste mês.`}
          {av.ja_no_mes_centavos ? ` Já distribuído no mês: ${formatarMoeda(av.ja_no_mes_centavos)}.` : ""}
        </Aviso>
      ) : null}
      {simular.error || registrar.error ? (
        <Aviso tipo="erro">{mensagemDe(simular.error ?? registrar.error)}</Aviso>
      ) : null}
      <div className="flex flex-wrap justify-end gap-2">
        <Botao
          variante="secundario"
          onClick={() => simular.mutate()}
          carregando={simular.isPending}
          disabled={!valor || valor <= 0}
        >
          Ver imposto
        </Botao>
        <Botao onClick={() => registrar.mutate()} carregando={registrar.isPending} disabled={!av}>
          Registrar retirada
        </Botao>
      </div>
    </div>
  );
}
