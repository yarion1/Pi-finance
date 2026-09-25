import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, FileUp, Undo2 } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Valor } from "../components/extras";
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
import { api, ErroApi, mensagemDe, obter } from "../lib/api";
import { lerBase64, rotuloCategoria, useCategorias, useContas } from "../lib/dados";
import { formatarData, formatarDataHora, formatarMoeda } from "../lib/formato";
import type { Importacao, Mapeamento, ResultadoImportacao } from "../lib/tipos";

type Arquivo = { nome: string; conteudo: string };
type MapeamentoSalvo = { id: string; nome: string; config: Mapeamento };

const formatos: Record<string, string> = {
  ofx: "OFX",
  nubank_conta: "CSV do Nubank (conta)",
  nubank_cartao: "CSV do Nubank (cartão)",
  inter: "CSV do Inter",
  generico: "CSV com colunas escolhidas",
};

export function Importar() {
  const cliente = useQueryClient();
  const contas = useContas();
  const categorias = useCategorias();
  const editaveis = (contas.data ?? []).filter((c) => c.pode_editar && !c.arquivada);
  const [contaId, setContaId] = useState("");
  const [arquivo, setArquivo] = useState<Arquivo | null>(null);
  const [cabecalhos, setCabecalhos] = useState<string[] | null>(null);
  const [mapa, setMapa] = useState<Mapeamento | null>(null);
  const [previa, setPrevia] = useState<ResultadoImportacao | null>(null);
  const [feito, setFeito] = useState<ResultadoImportacao | null>(null);
  const conta = contaId || editaveis[0]?.id || "";

  const historico = useQuery({
    queryKey: ["importacoes"],
    queryFn: () => obter<Importacao[]>("/api/importacoes"),
  });

  const enviar = useMutation({
    mutationFn: (simular: boolean) =>
      api<ResultadoImportacao>("POST", "/api/importacoes", {
        conta_id: conta,
        arquivo: arquivo?.nome,
        conteudo: arquivo?.conteudo,
        mapeamento: mapa,
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
      await Promise.all(
        ["transacoes", "resumo", "contas", "importacoes"].map((k) =>
          cliente.invalidateQueries({ queryKey: [k] }),
        ),
      );
    },
    onError: (e) => {
      // CSV desconhecido: o servidor manda os cabeçalhos para escolher as colunas
      if (e instanceof ErroApi && e.codigo === "mapeamento") {
        void api<{ cabecalhos: string[] }>("POST", "/api/importacoes/cabecalhos", {
          conteudo: arquivo?.conteudo,
        }).then((r) => {
          setCabecalhos(r.cabecalhos);
          setMapa((m) => m ?? mapaInicial(r.cabecalhos));
        });
      }
    },
  });

  const escolher = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    setPrevia(null);
    setFeito(null);
    setCabecalhos(null);
    setMapa(null);
    enviar.reset();
    if (!f) return;
    if (f.size > 8 * 1024 * 1024) {
      setArquivo(null);
      alert("Arquivo maior que 8 MB.");
      return;
    }
    setArquivo({ nome: f.name, conteudo: await lerBase64(f) });
  };

  const desfazer = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/importacoes/${id}`),
    onSuccess: () =>
      Promise.all(
        ["transacoes", "resumo", "contas", "importacoes"].map((k) =>
          cliente.invalidateQueries({ queryKey: [k] }),
        ),
      ),
  });

  const erroMostrar =
    enviar.error && !(enviar.error instanceof ErroApi && enviar.error.codigo === "mapeamento")
      ? enviar.error
      : null;

  return (
    <Pagina
      titulo="Importar extrato"
      subtitulo="OFX do internet banking ou CSV (Nubank, Inter ou qualquer banco)."
    >
      {editaveis.length === 0 && !contas.isPending ? (
        <Aviso>
          Cadastre uma conta antes, em{" "}
          <Link to="/contas" className="font-semibold underline">
            Contas
          </Link>
          .
        </Aviso>
      ) : null}

      <Cartao>
        <div className="grid gap-4 sm:grid-cols-2">
          <Selecao
            rotulo="Conta"
            value={conta}
            onChange={(e) => {
              setContaId(e.target.value);
              setPrevia(null);
            }}
          >
            {editaveis.map((c) => (
              <option key={c.id} value={c.id}>
                {c.nome}
                {c.entidade_nome ? ` — ${c.entidade_nome}` : ""}
              </option>
            ))}
          </Selecao>
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">Arquivo</span>
            <span className="flex min-h-11 items-center gap-3 rounded-xl border border-dashed border-borda bg-superficie-2 px-3 text-sm">
              <FileUp className="size-5 text-texto-2" aria-hidden />
              <input
                type="file"
                accept=".ofx,.OFX,.csv,.CSV,.txt,text/csv,application/x-ofx"
                onChange={escolher}
                className="min-w-0 flex-1 text-sm file:hidden"
              />
            </span>
          </label>
        </div>

        {cabecalhos && mapa ? <Mapeador cabecalhos={cabecalhos} mapa={mapa} onChange={setMapa} /> : null}

        {erroMostrar ? (
          <div className="mt-4">
            <Aviso tipo="erro">{mensagemDe(erroMostrar)}</Aviso>
          </div>
        ) : null}

        <div className="mt-4 flex flex-wrap gap-2">
          <Botao
            onClick={() => enviar.mutate(true)}
            disabled={!arquivo || !conta}
            carregando={enviar.isPending && enviar.variables === true}
          >
            Ver prévia
          </Botao>
        </div>
      </Cartao>

      {feito ? (
        <Cartao>
          <div className="flex items-start gap-3">
            <CheckCircle2 className="mt-0.5 size-6 shrink-0 text-entrada" aria-hidden />
            <div className="flex flex-col gap-1 text-sm">
              <p className="font-semibold">Importação concluída</p>
              <p>
                {feito.novas} novas · {feito.duplicadas} já existiam · {feito.transferencias} transferências
                entre contas · {feito.categorizadas} categorizadas automaticamente
              </p>
              {feito.saldo_arquivo_centavos != null && feito.saldo_sistema_centavos != null ? (
                feito.saldo_arquivo_centavos === feito.saldo_sistema_centavos ? (
                  <p className="text-entrada">
                    Saldo conferido com o do banco: {formatarMoeda(feito.saldo_sistema_centavos)}
                  </p>
                ) : (
                  <p className="text-alerta">
                    Saldo do banco {formatarMoeda(feito.saldo_arquivo_centavos)} × calculado{" "}
                    {formatarMoeda(feito.saldo_sistema_centavos)}. Ajuste o saldo inicial da conta ou importe
                    o período que falta.
                  </p>
                )
              ) : null}
              <p>
                <Link
                  to={`/gastos?importacao=${feito.importacao_id}`}
                  className="font-semibold text-primaria"
                >
                  Revisar as transações
                </Link>
              </p>
            </div>
          </div>
        </Cartao>
      ) : null}

      {previa ? (
        <Cartao>
          <TituloCartao
            acao={
              <Botao
                onClick={() => enviar.mutate(false)}
                disabled={previa.novas === 0}
                carregando={enviar.isPending && enviar.variables === false}
              >
                Importar {previa.novas} {previa.novas === 1 ? "transação" : "transações"}
              </Botao>
            }
          >
            Prévia — {formatos[previa.formato] ?? previa.formato}
          </TituloCartao>
          <p className="mb-3 text-sm text-texto-2">
            {previa.novas} novas · {previa.duplicadas} já importadas (serão ignoradas) ·{" "}
            {previa.categorizadas} com categoria sugerida
          </p>
          {previa.avisos?.length ? (
            <div className="mb-3">
              <Aviso tipo="alerta">
                {previa.avisos.slice(0, 3).join(" · ")}
                {previa.avisos.length > 3 ? ` · e mais ${previa.avisos.length - 3}` : ""}
              </Aviso>
            </div>
          ) : null}
          <ul className="flex max-h-[28rem] flex-col divide-y divide-borda overflow-y-auto">
            {(previa.previa ?? [])
              .map((l, i) => ({ ...l, chave: `${i}|${l.data}|${l.valor_centavos}` }))
              .map((l) => (
                <li
                  key={l.chave}
                  className={`flex min-h-12 items-center gap-3 py-1.5 text-sm ${l.duplicada ? "opacity-50" : ""}`}
                >
                  <span className="num w-20 shrink-0 text-texto-2">{formatarData(l.data)}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">{l.descricao}</span>
                    <span className="block truncate text-xs text-texto-2">
                      {l.duplicada ? "já importada" : rotuloCategoria(categorias.data ?? [], l.categoria_id)}
                      {l.origem_categoria === "regra"
                        ? " (regra)"
                        : l.origem_categoria === "historico"
                          ? " (histórico)"
                          : ""}
                    </span>
                  </span>
                  <Valor centavos={l.valor_centavos} className="font-medium" />
                </li>
              ))}
          </ul>
        </Cartao>
      ) : null}

      <Cartao>
        <TituloCartao>Importações anteriores</TituloCartao>
        {historico.isPending ? (
          <CarregandoLista linhas={2} />
        ) : historico.data?.length ? (
          <ul className="flex flex-col divide-y divide-borda">
            {historico.data.map((i) => (
              <li
                key={i.id}
                className={`flex min-h-14 flex-wrap items-center gap-3 py-2 ${i.desfeita_em ? "opacity-60" : ""}`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{i.arquivo}</span>
                  <span className="block text-xs text-texto-2">
                    {i.conta} · {formatarDataHora(i.criada_em)} · {i.novas} novas, {i.duplicadas} repetidas
                  </span>
                </span>
                {i.desfeita_em ? (
                  <Etiqueta>Desfeita</Etiqueta>
                ) : (
                  <>
                    <Link to={`/gastos?importacao=${i.id}`} className="text-sm font-semibold text-primaria">
                      Ver
                    </Link>
                    <Botao
                      variante="fantasma"
                      carregando={desfazer.isPending && desfazer.variables === i.id}
                      onClick={() =>
                        confirm(`Desfazer "${i.arquivo}"? As ${i.novas} transações dela serão apagadas.`) &&
                        desfazer.mutate(i.id)
                      }
                    >
                      <Undo2 className="size-4" aria-hidden /> Desfazer
                    </Botao>
                  </>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <Vazio
            titulo="Nenhuma importação ainda"
            texto="Cada importação fica registrada aqui e pode ser desfeita por inteiro."
          />
        )}
      </Cartao>
    </Pagina>
  );
}

function mapaInicial(cab: string[]): Mapeamento {
  const achar = (...nomes: string[]) =>
    cab.find((c) => nomes.some((n) => c.toLowerCase().normalize("NFD").replace(/\p{M}/gu, "").includes(n))) ??
    "";
  return {
    coluna_data: achar("data", "date", "dia", "quando"),
    colunas_descricao: [achar("descri", "histor", "title", "memo", "estabelec", "o que")].filter(Boolean),
    coluna_valor: achar("valor", "amount", "quantia", "quanto"),
    formato_data: "dd/mm/aaaa",
    decimal: ",",
    inverter_sinal: false,
  };
}

function Mapeador({
  cabecalhos,
  mapa,
  onChange,
}: {
  cabecalhos: string[];
  mapa: Mapeamento;
  onChange: (m: Mapeamento) => void;
}) {
  const cliente = useQueryClient();
  const salvos = useQuery({
    queryKey: ["mapeamentos"],
    queryFn: () => obter<MapeamentoSalvo[]>("/api/mapeamentos"),
  });
  const [nome, setNome] = useState("");
  const salvar = useMutation({
    mutationFn: () => api("POST", "/api/mapeamentos", { nome, config: mapa }),
    onSuccess: () => cliente.invalidateQueries({ queryKey: ["mapeamentos"] }),
  });
  const colunas = (
    <>
      <option value="">—</option>
      {cabecalhos.map((c) => (
        <option key={c} value={c}>
          {c}
        </option>
      ))}
    </>
  );
  return (
    <div className="mt-4 flex flex-col gap-4 rounded-xl border border-borda p-4">
      <Aviso>Não reconheci este CSV. Diga quais colunas são a data, a descrição e o valor.</Aviso>
      {salvos.data?.length ? (
        <Selecao
          rotulo="Usar um mapeamento salvo"
          value=""
          onChange={(e) => {
            const m = salvos.data?.find((x) => x.id === e.target.value);
            if (m) onChange(m.config);
          }}
        >
          <option value="">—</option>
          {salvos.data.map((m) => (
            <option key={m.id} value={m.id}>
              {m.nome}
            </option>
          ))}
        </Selecao>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-3">
        <Selecao
          rotulo="Data"
          value={mapa.coluna_data}
          onChange={(e) => onChange({ ...mapa, coluna_data: e.target.value })}
        >
          {colunas}
        </Selecao>
        <Selecao
          rotulo="Descrição"
          value={mapa.colunas_descricao[0] ?? ""}
          onChange={(e) => onChange({ ...mapa, colunas_descricao: e.target.value ? [e.target.value] : [] })}
        >
          {colunas}
        </Selecao>
        <Selecao
          rotulo="Valor"
          value={mapa.coluna_valor}
          onChange={(e) => onChange({ ...mapa, coluna_valor: e.target.value })}
        >
          {colunas}
        </Selecao>
        <Selecao
          rotulo="Formato da data"
          value={mapa.formato_data}
          onChange={(e) => onChange({ ...mapa, formato_data: e.target.value })}
        >
          <option value="dd/mm/aaaa">25/09/2026</option>
          <option value="aaaa-mm-dd">2026-09-25</option>
          <option value="mm/dd/aaaa">09/25/2026</option>
          <option value="dd-mm-aaaa">25-09-2026</option>
          <option value="dd/mm/aa">25/09/26</option>
        </Selecao>
        <Selecao
          rotulo="Decimal"
          value={mapa.decimal}
          onChange={(e) => onChange({ ...mapa, decimal: e.target.value as "," | "." })}
        >
          <option value=",">1.234,56</option>
          <option value=".">1,234.56</option>
        </Selecao>
        <label className="flex min-h-11 items-center gap-3 self-end text-sm">
          <input
            type="checkbox"
            className="size-5 accent-[var(--primaria)]"
            checked={mapa.inverter_sinal}
            onChange={(e) => onChange({ ...mapa, inverter_sinal: e.target.checked })}
          />
          Valor positivo é gasto (fatura)
        </label>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <Campo
          rotulo="Salvar como (opcional)"
          className="flex-1"
          value={nome}
          onChange={(e) => setNome(e.target.value)}
          placeholder="Extrato do Banco X"
        />
        <Botao
          variante="secundario"
          disabled={!nome.trim()}
          carregando={salvar.isPending}
          onClick={() => salvar.mutate()}
        >
          {salvar.isSuccess ? "Salvo" : "Salvar mapeamento"}
        </Botao>
      </div>
    </div>
  );
}
