import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileScan, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { AvisoSimulacao, SeletorEntidade } from "../components/extras";
import {
  Aviso,
  Botao,
  Campo,
  Cartao,
  Etiqueta,
  Pagina,
  Selecao,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { lerBase64, nomeMes, useContas, useEditaveis } from "../lib/dados";
import { formatarData, formatarMoeda } from "../lib/formato";
import type { HoleritesDoAno, PreviaDocumento, ResultadoImportacao, TipoDocumento } from "../lib/tipos";

const tipos: { id: TipoDocumento; nome: string; dica: string }[] = [
  { id: "fatura", nome: "Fatura do cartão (PDF)", dica: "os lançamentos entram no cartão, sem duplicar" },
  { id: "comprovante", nome: "Comprovante (foto ou PDF)", dica: "vira uma transação" },
  { id: "nota_corretagem", nome: "Nota de corretagem", dica: "compras e vendas com as taxas" },
  { id: "holerite", nome: "Holerite", dica: "guarda bruto, INSS e IRRF para o IR do ano" },
];

type Lancamento = { data?: string; descricao?: string; valor?: string; parcela?: string };
type OperacaoLida = { codigo?: string; tipo?: string; quantidade?: string; preco?: string };

/** "123.45" (como a IA devolve) em reais, sem passar por float. */
const reais = (s?: string) => {
  const m = /^(-?)(\d+)(?:\.(\d{1,2}))?$/.exec((s ?? "").trim());
  if (!m) return s ?? "";
  const centavos = Number(m[2]) * 100 + Number((m[3] ?? "").padEnd(2, "0"));
  return formatarMoeda(m[1] ? -centavos : centavos);
};

export function Documentos() {
  const [tipo, setTipo] = useState<TipoDocumento>("fatura");
  const [arquivo, setArquivo] = useState<File | null>(null);
  const [confirmado, setConfirmado] = useState(false);
  const [previa, setPrevia] = useState<PreviaDocumento | null>(null);
  const ler = useMutation({
    mutationFn: async () => {
      if (!arquivo) throw new Error("Escolha o arquivo.");
      return api<PreviaDocumento>("POST", "/api/documentos/ler", {
        tipo,
        conteudo: await lerBase64(arquivo),
        envio_confirmado: confirmado,
      });
    },
    onSuccess: setPrevia,
  });

  return (
    <Pagina titulo="Ler documento" subtitulo="A IA lê o PDF ou a foto; você confere antes de lançar.">
      <Cartao>
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            setPrevia(null);
            ler.mutate();
          }}
        >
          <Selecao rotulo="O que é" value={tipo} onChange={(e) => setTipo(e.target.value as TipoDocumento)}>
            {tipos.map((t) => (
              <option key={t.id} value={t.id}>
                {t.nome}
              </option>
            ))}
          </Selecao>
          <p className="-mt-1 text-xs text-texto-2">{tipos.find((t) => t.id === tipo)?.dica}</p>
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Arquivo</span>
            <input
              type="file"
              accept="application/pdf,image/jpeg,image/png,image/webp"
              onChange={(e) => setArquivo(e.target.files?.[0] ?? null)}
              className="text-sm"
            />
          </label>
          <label className="flex items-start gap-2 text-sm">
            <input
              type="checkbox"
              className="mt-1 size-4"
              checked={confirmado}
              onChange={(e) => setConfirmado(e.target.checked)}
            />
            <span>
              Entendo que o documento vai inteiro, com os meus dados pessoais, para a API da Anthropic
              (Claude) ser lido. O arquivo não fica guardado no painel.
            </span>
          </label>
          <div>
            <Botao type="submit" carregando={ler.isPending} disabled={!arquivo || !confirmado}>
              Ler documento
            </Botao>
          </div>
          {ler.error ? (
            <Aviso tipo="erro">
              {mensagemDe(ler.error)}{" "}
              {mensagemDe(ler.error).includes("IA está desligada") ? (
                <Link to="/ia" className="font-semibold underline">
                  Ligar a IA
                </Link>
              ) : null}
            </Aviso>
          ) : null}
        </form>
      </Cartao>

      {previa ? (
        <Revisao key={JSON.stringify(previa.extraido)} previa={previa} aoTerminar={() => setPrevia(null)} />
      ) : null}
      <Holerites />
      <AvisoSimulacao />
    </Pagina>
  );
}

function Revisao({ previa, aoTerminar }: { previa: PreviaDocumento; aoTerminar: () => void }) {
  const contas = useContas();
  const editaveis = useEditaveis();
  const cliente = useQueryClient();
  const [fora, setFora] = useState<Set<number>>(new Set());
  const [extraido, setExtraido] = useState<Record<string, unknown>>(previa.extraido);
  const [conta, setConta] = useState("");
  const [entidade, setEntidade] = useState("");
  const listaContas = (contas.data ?? []).filter(
    (c) => !c.arquivada && (previa.tipo === "fatura" ? c.tipo === "cartao" : c.tipo !== "cartao"),
  );
  const contaEscolhida = conta || listaContas[0]?.id || "";
  const entidadeEscolhida = entidade || editaveis.lista[0]?.id || "";

  const itens = (previa.tipo === "fatura" ? extraido.lancamentos : extraido.operacoes) as
    | (Lancamento & OperacaoLida)[]
    | undefined;
  const corpo = (simular: boolean) => {
    const e = { ...extraido };
    if (previa.tipo === "fatura") e.lancamentos = (itens ?? []).filter((_, i) => !fora.has(i));
    if (previa.tipo === "nota_corretagem") e.operacoes = (itens ?? []).filter((_, i) => !fora.has(i));
    return {
      tipo: previa.tipo,
      extraido: e,
      conta_id: contaEscolhida,
      entidade_id: entidadeEscolhida,
      simular,
    };
  };
  const lancar = useMutation({
    mutationFn: () => api<ResultadoImportacao>("POST", "/api/documentos/lancar", corpo(false)),
    onSuccess: () => {
      for (const chave of ["transacoes", "resumo", "contas", "investimentos", "holerites", "indicadores"])
        cliente.invalidateQueries({ queryKey: [chave] });
    },
  });
  const r = lancar.data;
  const quantos = (itens?.length ?? 1) - fora.size;

  return (
    <Cartao>
      <TituloCartao>Confira o que a IA leu</TituloCartao>
      {previa.avisos.length ? (
        <Aviso tipo="alerta">
          {previa.avisos.length} {previa.avisos.length === 1 ? "linha ficou" : "linhas ficaram"} de fora:{" "}
          {previa.avisos.join("; ")}.
        </Aviso>
      ) : null}

      {previa.tipo === "fatura" || previa.tipo === "nota_corretagem" ? (
        <ul className="mt-2 flex flex-col divide-y divide-borda" aria-label="Itens lidos">
          {(itens ?? []).map((it, i) => (
            // biome-ignore lint/suspicious/noArrayIndexKey: os itens lidos não têm id; a ordem é fixa
            <li key={i} className="flex min-h-12 items-center gap-3 py-2 text-sm">
              <input
                type="checkbox"
                className="size-4"
                aria-label={`Incluir ${it.descricao ?? it.codigo ?? `item ${i + 1}`}`}
                checked={!fora.has(i)}
                onChange={() => {
                  const n = new Set(fora);
                  if (n.has(i)) n.delete(i);
                  else n.add(i);
                  setFora(n);
                }}
              />
              {previa.tipo === "fatura" ? (
                <>
                  <span className="w-12 shrink-0 text-xs text-texto-2">
                    {formatarData(it.data ?? "").slice(0, 5)}
                  </span>
                  <span className="min-w-0 flex-1 truncate">
                    {it.descricao}
                    {it.parcela ? <span className="text-texto-2"> · {it.parcela}</span> : null}
                  </span>
                  <span className="valor num">{reais(it.valor)}</span>
                </>
              ) : (
                <>
                  <Etiqueta cor={it.tipo === "V" ? "alerta" : "entrada"}>
                    {it.tipo === "V" ? "Venda" : "Compra"}
                  </Etiqueta>
                  <span className="min-w-0 flex-1 truncate font-medium">{it.codigo}</span>
                  <span className="valor num text-xs">
                    {it.quantidade} × {reais(it.preco)}
                  </span>
                </>
              )}
            </li>
          ))}
        </ul>
      ) : null}

      {previa.tipo === "comprovante" ? (
        <div className="mt-2 grid grid-cols-2 gap-3">
          <Campo
            rotulo="Data"
            type="date"
            value={String(extraido.data ?? "")}
            onChange={(e) => setExtraido({ ...extraido, data: e.target.value })}
          />
          <Campo
            rotulo="Valor (R$, com ponto)"
            inputMode="decimal"
            value={String(extraido.valor ?? "")}
            onChange={(e) => setExtraido({ ...extraido, valor: e.target.value })}
          />
          <Campo
            className="col-span-2"
            rotulo="Descrição"
            value={String(extraido.descricao ?? "")}
            onChange={(e) => setExtraido({ ...extraido, descricao: e.target.value })}
          />
          <Selecao
            rotulo="É"
            value={String(extraido.sentido ?? "saida")}
            onChange={(e) => setExtraido({ ...extraido, sentido: e.target.value })}
          >
            <option value="saida">um pagamento</option>
            <option value="entrada">um recebimento</option>
          </Selecao>
        </div>
      ) : null}

      {previa.tipo === "holerite" && previa.holerite ? (
        <dl className="mt-2 grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-xs text-texto-2">Competência</dt>
            <dd>{nomeMes(previa.holerite.competencia)}</dd>
          </div>
          <div>
            <dt className="text-xs text-texto-2">Empregador</dt>
            <dd>{previa.holerite.empregador || "—"}</dd>
          </div>
          {(
            [
              ["Bruto", previa.holerite.bruto_centavos],
              ["INSS", previa.holerite.inss_centavos],
              ["IRRF", previa.holerite.irrf_centavos],
              ["Outros descontos", previa.holerite.outros_descontos_centavos],
              ["Líquido", previa.holerite.liquido_centavos],
            ] as const
          ).map(([nome, v]) => (
            <div key={nome}>
              <dt className="text-xs text-texto-2">{nome}</dt>
              <dd className="valor num font-semibold">{formatarMoeda(v)}</dd>
            </div>
          ))}
        </dl>
      ) : null}

      <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
        {previa.tipo === "fatura" || previa.tipo === "comprovante" ? (
          <Selecao rotulo="Lançar na conta" value={contaEscolhida} onChange={(e) => setConta(e.target.value)}>
            {listaContas.map((c) => (
              <option key={c.id} value={c.id}>
                {c.nome}
              </option>
            ))}
          </Selecao>
        ) : (
          <SeletorEntidade entidades={editaveis.lista} valor={entidadeEscolhida} onChange={setEntidade} />
        )}
      </div>
      {(previa.tipo === "fatura" || previa.tipo === "comprovante") && !listaContas.length ? (
        <Aviso tipo="alerta">
          Cadastre {previa.tipo === "fatura" ? "o cartão" : "a conta"} em{" "}
          <Link to="/contas" className="font-semibold underline">
            Contas
          </Link>{" "}
          primeiro.
        </Aviso>
      ) : null}

      {lancar.error ? <Aviso tipo="erro">{mensagemDe(lancar.error)}</Aviso> : null}
      {r ? (
        <Aviso tipo="info">
          {previa.tipo === "fatura" || previa.tipo === "comprovante"
            ? `Lançado: ${r.novas} ${r.novas === 1 ? "transação nova" : "transações novas"}${r.duplicadas ? `, ${r.duplicadas} já existiam` : ""}.`
            : previa.tipo === "holerite"
              ? "Holerite guardado."
              : "Operações lançadas na carteira."}
        </Aviso>
      ) : (
        <div className="mt-4 flex flex-wrap gap-2">
          <Botao onClick={() => lancar.mutate()} carregando={lancar.isPending} disabled={quantos < 1}>
            {previa.tipo === "holerite"
              ? "Guardar holerite"
              : previa.tipo === "comprovante"
                ? "Lançar transação"
                : `Lançar ${quantos} ${quantos === 1 ? "item" : "itens"}`}
          </Botao>
          <Botao variante="secundario" onClick={aoTerminar}>
            Descartar
          </Botao>
        </div>
      )}
    </Cartao>
  );
}

function Holerites() {
  const ano = new Date().getFullYear();
  const dados = useQuery({
    queryKey: ["holerites", ano],
    queryFn: () => obter<HoleritesDoAno>(`/api/holerites?ano=${ano}`),
  });
  const cliente = useQueryClient();
  const apagar = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/holerites/${id}`),
    onSuccess: () => cliente.invalidateQueries({ queryKey: ["holerites"] }),
  });
  const h = dados.data;
  return (
    <Cartao>
      <TituloCartao>Holerites de {ano}</TituloCartao>
      {(dados.error ?? apagar.error) ? (
        <Aviso tipo="erro">{mensagemDe(dados.error ?? apagar.error)}</Aviso>
      ) : null}
      {h?.itens.length ? (
        <>
          <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {(
              [
                ["Rendimentos brutos", h.bruto_centavos],
                ["INSS", h.inss_centavos],
                ["IRRF retido", h.irrf_centavos],
                ["Líquido", h.liquido_centavos],
              ] as const
            ).map(([nome, v]) => (
              <div key={nome}>
                <dt className="text-xs text-texto-2">{nome}</dt>
                <dd className="valor num text-sm font-semibold">{formatarMoeda(v)}</dd>
              </div>
            ))}
          </dl>
          <ul className="mt-3 flex flex-col divide-y divide-borda" aria-label="Holerites">
            {h.itens.map((x) => (
              <li key={x.id} className="flex min-h-12 items-center gap-3 py-2 text-sm">
                <span className="min-w-0 flex-1 truncate">
                  {nomeMes(x.competencia)}
                  {x.empregador ? <span className="text-texto-2"> · {x.empregador}</span> : null}
                </span>
                <span className="valor num">{formatarMoeda(x.bruto_centavos)}</span>
                <Botao
                  variante="fantasma"
                  aria-label={`Apagar holerite de ${nomeMes(x.competencia)}`}
                  onClick={() => apagar.mutate(x.id)}
                >
                  <Trash2 className="size-4" />
                </Botao>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-xs text-texto-2">
            Base para a declaração do IR: rendimentos tributáveis e imposto retido.
          </p>
        </>
      ) : (
        <Vazio
          icone={<FileScan className="size-8" />}
          titulo="Nenhum holerite"
          texto="Leia o PDF ou a foto do holerite acima para guardar os valores do ano."
        />
      )}
    </Cartao>
  );
}
