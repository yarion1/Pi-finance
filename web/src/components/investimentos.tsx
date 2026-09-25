import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileUp, Trash2 } from "lucide-react";
import { useState } from "react";
import { api, mensagemDe, obter } from "../lib/api";
import { lerBase64, useEditaveis } from "../lib/dados";
import {
  decimalBR,
  formatarData,
  formatarMoeda,
  hojeSP,
  lerCentavos,
  percentualParaFracao,
} from "../lib/formato";
import {
  type AtivoCarteira,
  nomesClasse,
  nomesOperacao,
  type OperacaoAtivo,
  type ResultadoB3,
} from "../lib/tipos";
import { Dialogo, SeletorEntidade } from "./extras";
import { Aviso, Botao, Campo, CarregandoLista, Etiqueta, Selecao } from "./ui";

/** Tudo que muda quando a carteira muda. */
const chavesCarteira = ["investimentos", "investimentos-ir", "operacoes", "indicadores", "patrimonio"];

function useInvalidarCarteira() {
  const cliente = useQueryClient();
  return () => Promise.all(chavesCarteira.map((k) => cliente.invalidateQueries({ queryKey: [k] })));
}

const classesManuais = [
  "acao",
  "fii",
  "etf",
  "bdr",
  "tesouro",
  "renda_fixa",
  "fundo",
  "cripto",
  "exterior",
  "previdencia",
  "outro",
];
const rendaFixa = (c: string) => c === "tesouro" || c === "renda_fixa";

export function DialogoB3({ aberto, aoFechar }: { aberto: boolean; aoFechar: () => void }) {
  const invalidar = useInvalidarCarteira();
  const editaveis = useEditaveis();
  const [entidade, setEntidade] = useState("");
  const [arquivo, setArquivo] = useState<{ nome: string; conteudo: string } | null>(null);
  const [previa, setPrevia] = useState<ResultadoB3 | null>(null);
  const [feito, setFeito] = useState<ResultadoB3 | null>(null);
  const ent = entidade || editaveis.lista[0]?.id || "";

  const enviar = useMutation({
    mutationFn: (simular: boolean) =>
      api<ResultadoB3>("POST", "/api/investimentos/b3", {
        entidade_id: ent,
        conteudo: arquivo?.conteudo,
        simular,
      }),
    onSuccess: async (r, simular) => {
      if (simular) {
        setPrevia(r);
        return;
      }
      setFeito(r);
      setPrevia(null);
      setArquivo(null);
      await invalidar();
    },
  });

  const fechar = () => {
    setArquivo(null);
    setPrevia(null);
    setFeito(null);
    enviar.reset();
    aoFechar();
  };

  return (
    <Dialogo aberto={aberto} aoFechar={fechar} titulo="Importar da B3">
      <div className="flex flex-col gap-4">
        <p className="text-sm text-texto-2">
          Na Área do Investidor da B3 (investidor.b3.com.br), em Extratos, baixe em Excel a{" "}
          <strong className="text-texto">Negociação</strong> (compras e vendas) e a{" "}
          <strong className="text-texto">Movimentação</strong> (proventos, desdobros e bonificações). Pode
          importar os dois, quantas vezes quiser: nada duplica.
        </p>
        <SeletorEntidade entidades={editaveis.lista} valor={ent} onChange={setEntidade} />
        <label className="flex min-h-24 cursor-pointer flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-borda bg-superficie-2 p-4 text-center text-sm hover:border-texto-2">
          <FileUp className="size-6 text-texto-2" aria-hidden />
          <span>{arquivo ? arquivo.nome : "Escolher arquivo .xlsx ou .csv"}</span>
          <input
            type="file"
            accept=".xlsx,.csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"
            className="sr-only"
            onChange={async (e) => {
              const f = e.target.files?.[0];
              setPrevia(null);
              setFeito(null);
              enviar.reset();
              if (!f) return;
              setArquivo({ nome: f.name, conteudo: await lerBase64(f) });
            }}
          />
        </label>
        {enviar.error ? <Aviso tipo="erro">{mensagemDe(enviar.error)}</Aviso> : null}
        {previa ? <ResumoB3 r={previa} previa /> : null}
        {feito ? <ResumoB3 r={feito} /> : null}
        <div className="flex flex-wrap justify-end gap-2">
          {previa ? (
            <Botao
              onClick={() => enviar.mutate(false)}
              carregando={enviar.isPending}
              disabled={previa.novas === 0}
            >
              Importar {previa.novas} {previa.novas === 1 ? "linha" : "linhas"}
            </Botao>
          ) : (
            <Botao
              onClick={() => enviar.mutate(true)}
              carregando={enviar.isPending}
              disabled={!arquivo || !ent}
            >
              Ver prévia
            </Botao>
          )}
        </div>
      </div>
    </Dialogo>
  );
}

function ResumoB3({ r, previa }: { r: ResultadoB3; previa?: boolean }) {
  const tipo = r.formato === "b3_negociacao" ? "negociação" : "movimentação";
  return (
    <Aviso tipo={previa ? "info" : "sucesso"}>
      <span className="block">
        {previa ? "Extrato de " : "Importado: extrato de "}
        {tipo}. {r.novas} {r.novas === 1 ? "linha nova" : "linhas novas"}
        {r.duplicadas ? `, ${r.duplicadas} já importadas` : ""}.
      </span>
      {r.ativos_novos.length ? (
        <span className="block">
          {previa ? "Ativos que serão criados" : "Ativos criados"}: {r.ativos_novos.join(", ")}.
        </span>
      ) : null}
      {r.avisos.length ? (
        <details className="mt-1">
          <summary className="cursor-pointer">{r.avisos.length} aviso(s)</summary>
          <ul className="mt-1 list-disc pl-5 text-xs">
            {r.avisos.slice(0, 20).map((a) => (
              <li key={a}>{a}</li>
            ))}
          </ul>
        </details>
      ) : null}
    </Aviso>
  );
}

export function DialogoNovoAtivo({ aberto, aoFechar }: { aberto: boolean; aoFechar: () => void }) {
  const invalidar = useInvalidarCarteira();
  const editaveis = useEditaveis();
  const [entidade, setEntidade] = useState("");
  const [f, setF] = useState({
    codigo: "",
    classe: "acao",
    instituicao: "",
    indexador: "cdi",
    taxa: "",
    vencimento: "",
    isento: false,
  });
  const ent = entidade || editaveis.lista[0]?.id || "";
  const taxa = f.taxa ? percentualParaFracao(f.taxa) : null;

  const criar = useMutation({
    mutationFn: () =>
      api<{ id: string }>("POST", "/api/investimentos/ativos", {
        entidade_id: ent,
        codigo: f.codigo.trim(),
        classe: f.classe,
        instituicao: f.instituicao,
        ...(rendaFixa(f.classe)
          ? { indexador: f.indexador, taxa: taxa ?? "", vencimento: f.vencimento, isento_ir: f.isento }
          : {}),
      }),
    onSuccess: async () => {
      await invalidar();
      setF({ ...f, codigo: "", taxa: "", vencimento: "" });
      aoFechar();
    },
  });

  return (
    <Dialogo aberto={aberto} aoFechar={aoFechar} titulo="Novo ativo">
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          criar.mutate();
        }}
      >
        <SeletorEntidade entidades={editaveis.lista} valor={ent} onChange={setEntidade} />
        <Selecao rotulo="Classe" value={f.classe} onChange={(e) => setF({ ...f, classe: e.target.value })}>
          {classesManuais.map((c) => (
            <option key={c} value={c}>
              {nomesClasse[c]}
            </option>
          ))}
        </Selecao>
        <Campo
          rotulo={rendaFixa(f.classe) ? "Nome do título" : "Código"}
          placeholder={
            rendaFixa(f.classe) ? "CDB Banco X 110% CDI 2028" : f.classe === "cripto" ? "bitcoin" : "PETR4"
          }
          value={f.codigo}
          onChange={(e) => setF({ ...f, codigo: e.target.value })}
          required
          maxLength={120}
        />
        <Campo
          rotulo="Corretora ou banco (opcional)"
          value={f.instituicao}
          onChange={(e) => setF({ ...f, instituicao: e.target.value })}
        />
        {rendaFixa(f.classe) ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Selecao
              rotulo="Indexador"
              value={f.indexador}
              onChange={(e) => setF({ ...f, indexador: e.target.value })}
            >
              <option value="cdi">% do CDI</option>
              <option value="prefixado">Prefixado</option>
              <option value="ipca">IPCA +</option>
            </Selecao>
            <Campo
              rotulo={f.indexador === "cdi" ? "% do CDI" : "Taxa ao ano (%)"}
              inputMode="decimal"
              placeholder={f.indexador === "cdi" ? "110" : "6,5"}
              value={f.taxa}
              onChange={(e) => setF({ ...f, taxa: e.target.value })}
              erro={f.taxa && !taxa ? "Número inválido" : undefined}
            />
            <Campo
              rotulo="Vencimento"
              type="date"
              value={f.vencimento}
              onChange={(e) => setF({ ...f, vencimento: e.target.value })}
            />
            <label className="flex min-h-11 items-center gap-2 self-end text-sm">
              <input
                type="checkbox"
                checked={f.isento}
                onChange={(e) => setF({ ...f, isento: e.target.checked })}
                className="size-5 accent-[var(--primaria)]"
              />
              Isento de IR (LCI, LCA, CRI, CRA)
            </label>
          </div>
        ) : null}
        {criar.error ? <Aviso tipo="erro">{mensagemDe(criar.error)}</Aviso> : null}
        <div className="flex justify-end">
          <Botao type="submit" carregando={criar.isPending} disabled={!ent || !f.codigo.trim()}>
            Criar ativo
          </Botao>
        </div>
      </form>
    </Dialogo>
  );
}

export function DialogoAtivo({ ativo, aoFechar }: { ativo: AtivoCarteira | null; aoFechar: () => void }) {
  const invalidar = useInvalidarCarteira();
  const doBanco = ativo?.origem === "pluggy";
  const ops = useQuery({
    queryKey: ["operacoes", ativo?.id],
    queryFn: () => obter<OperacaoAtivo[]>(`/api/investimentos/ativos/${ativo?.id}/operacoes`),
    enabled: !!ativo && !doBanco,
  });
  const [op, setOp] = useState({
    tipo: "compra",
    data: hojeSP(),
    quantidade: "",
    preco: "",
    valor: "",
    taxas: "",
  });
  const negocio = op.tipo === "compra" || op.tipo === "venda";
  const qtd = decimalBR(op.quantidade);
  const preco = decimalBR(op.preco);
  const valor = lerCentavos(op.valor);
  const taxas = op.taxas ? lerCentavos(op.taxas) : 0;
  const valido = negocio ? !!qtd && !!preco && taxas !== null : valor !== null && valor > 0;

  const lancar = useMutation({
    mutationFn: () =>
      api("POST", `/api/investimentos/ativos/${ativo?.id}/operacoes`, {
        tipo: op.tipo,
        data: op.data,
        quantidade: negocio ? qtd : "0",
        preco: negocio ? preco : "0",
        valor_centavos: negocio ? 0 : Math.abs(valor ?? 0),
        taxas_centavos: Math.abs(taxas ?? 0),
      }),
    onSuccess: async () => {
      setOp({ ...op, quantidade: "", preco: "", valor: "", taxas: "" });
      await invalidar();
    },
  });
  const apagarOp = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/investimentos/ativos/${ativo?.id}/operacoes/${id}`),
    onSuccess: invalidar,
  });
  const mudarClasse = useMutation({
    mutationFn: (classe: string) => api("PATCH", `/api/investimentos/ativos/${ativo?.id}`, { classe }),
    onSuccess: invalidar,
  });
  const apagar = useMutation({
    mutationFn: () => api("DELETE", `/api/investimentos/ativos/${ativo?.id}`),
    onSuccess: async () => {
      await invalidar();
      aoFechar();
    },
  });

  return (
    <Dialogo aberto={!!ativo} aoFechar={aoFechar} titulo={ativo?.nome ?? "Ativo"}>
      {ativo ? (
        <div className="flex flex-col gap-4">
          <dl className="grid grid-cols-2 gap-3 text-sm">
            {doBanco ? null : (
              <>
                <Dado rotulo="Quantidade" valor={ativo.quantidade.replace(".", ",")} />
                <Dado rotulo="Preço médio" valor={ativo.preco_medio.replace(".", ",")} />
              </>
            )}
            <Dado rotulo="Custo" valor={formatarMoeda(ativo.custo_centavos)} />
            <Dado
              rotulo={ativo.cotacao_em ? `Valor (cotação de ${formatarData(ativo.cotacao_em)})` : "Valor"}
              valor={formatarMoeda(ativo.valor_centavos)}
            />
            {ativo.proventos_centavos ? (
              <Dado rotulo="Proventos recebidos" valor={formatarMoeda(ativo.proventos_centavos)} />
            ) : null}
          </dl>
          {doBanco ? (
            <Aviso>Vem do Open Finance: o valor é o que a instituição informa a cada sincronização.</Aviso>
          ) : !ativo.cotacao && ativo.valor_centavos > 0 ? (
            <p className="text-xs text-texto-2">Ainda sem cotação: o valor mostrado é o custo.</p>
          ) : null}

          {doBanco ? null : (
            <>
              <Selecao
                rotulo="Classe"
                value={ativo.classe}
                onChange={(e) => mudarClasse.mutate(e.target.value)}
                disabled={mudarClasse.isPending}
              >
                {classesManuais.map((c) => (
                  <option key={c} value={c}>
                    {nomesClasse[c]}
                  </option>
                ))}
              </Selecao>

              <form
                className="flex flex-col gap-3 rounded-xl border border-borda p-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  lancar.mutate();
                }}
              >
                <p className="text-sm font-semibold">Lançar operação</p>
                <div className="grid grid-cols-2 gap-3">
                  <Selecao
                    rotulo="Tipo"
                    value={op.tipo}
                    onChange={(e) => setOp({ ...op, tipo: e.target.value })}
                  >
                    {["compra", "venda", "provento", "juros", "amortizacao"].map((t) => (
                      <option key={t} value={t}>
                        {nomesOperacao[t]}
                      </option>
                    ))}
                  </Selecao>
                  <Campo
                    rotulo="Data"
                    type="date"
                    value={op.data}
                    onChange={(e) => setOp({ ...op, data: e.target.value })}
                    required
                  />
                  {negocio ? (
                    <>
                      <Campo
                        rotulo="Quantidade"
                        inputMode="decimal"
                        value={op.quantidade}
                        onChange={(e) => setOp({ ...op, quantidade: e.target.value })}
                      />
                      <Campo
                        rotulo="Preço unitário"
                        inputMode="decimal"
                        value={op.preco}
                        onChange={(e) => setOp({ ...op, preco: e.target.value })}
                      />
                      <Campo
                        rotulo="Taxas (R$)"
                        inputMode="decimal"
                        value={op.taxas}
                        onChange={(e) => setOp({ ...op, taxas: e.target.value })}
                        className="col-span-2"
                      />
                    </>
                  ) : (
                    <Campo
                      rotulo="Valor recebido (R$)"
                      inputMode="decimal"
                      value={op.valor}
                      onChange={(e) => setOp({ ...op, valor: e.target.value })}
                      className="col-span-2"
                    />
                  )}
                </div>
                {lancar.error ? <Aviso tipo="erro">{mensagemDe(lancar.error)}</Aviso> : null}
                <Botao type="submit" carregando={lancar.isPending} disabled={!valido}>
                  Lançar
                </Botao>
              </form>

              <section>
                <h3 className="mb-2 text-sm font-semibold">Operações</h3>
                {apagarOp.error ? <Aviso tipo="erro">{mensagemDe(apagarOp.error)}</Aviso> : null}
                {ops.isPending ? (
                  <CarregandoLista linhas={2} />
                ) : ops.data?.length ? (
                  <ul className="divide-y divide-borda rounded-xl border border-borda">
                    {ops.data.map((o) => (
                      <li key={o.id} className="flex items-center gap-2 px-3 py-2 text-sm">
                        <span className="min-w-0 flex-1">
                          <span className="block font-medium">
                            {nomesOperacao[o.tipo] ?? o.tipo}{" "}
                            {o.origem === "b3" ? <Etiqueta>B3</Etiqueta> : null}
                          </span>
                          <span className="block text-xs text-texto-2">
                            {formatarData(o.data)}
                            {o.evento
                              ? ` · ${o.quantidade.startsWith("-") ? "" : "+"}${o.quantidade.replace(".", ",")}`
                              : o.valor_centavos
                                ? ` · ${formatarMoeda(o.valor_centavos)}`
                                : ` · ${o.quantidade.replace(".", ",")} × ${o.preco.replace(".", ",")}`}
                          </span>
                        </span>
                        <button
                          type="button"
                          aria-label={`Apagar ${nomesOperacao[o.tipo] ?? o.tipo} de ${formatarData(o.data)}`}
                          onClick={() => apagarOp.mutate(o.id)}
                          className="inline-flex size-11 items-center justify-center rounded-xl text-texto-2 hover:bg-superficie-2 hover:text-saida"
                        >
                          <Trash2 className="size-4" />
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-sm text-texto-2">Nenhuma operação ainda.</p>
                )}
              </section>
            </>
          )}
          {apagar.error ? <Aviso tipo="erro">{mensagemDe(apagar.error)}</Aviso> : null}
          <Botao
            variante="perigo"
            carregando={apagar.isPending}
            onClick={() => {
              if (confirm(`Apagar ${ativo.nome} e todas as operações?`)) apagar.mutate();
            }}
          >
            Apagar ativo
          </Botao>
        </div>
      ) : null}
    </Dialogo>
  );
}

function Dado({ rotulo, valor }: { rotulo: string; valor: string }) {
  return (
    <div>
      <dt className="text-xs text-texto-2">{rotulo}</dt>
      <dd className="valor num font-semibold">{valor}</dd>
    </div>
  );
}
