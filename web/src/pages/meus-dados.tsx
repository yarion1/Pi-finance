import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, FileSpreadsheet, History, Sparkles, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router";
import { useComReautenticacao } from "../components/reautenticacao";
import { Aviso, Botao, Campo, CarregandoLista, Cartao, Pagina, TituloCartao } from "../components/ui";
import { api, baixar, mensagemDe, obter } from "../lib/api";
import { dolares } from "../lib/dados";
import { formatarData, formatarDataHora } from "../lib/formato";

const nomesAcao: Record<string, string> = {
  cadastro: "Conta criada",
  login: "Entrou no painel",
  logout: "Saiu",
  senha_ok: "Senha conferida",
  totp_ativado: "Aplicativo autenticador ativado",
  passkey_adicionada: "Passkey adicionada",
  passkey_removida: "Passkey removida",
  codigos_recuperacao_gerados: "Códigos de recuperação gerados",
  exportacao: "Dados exportados",
  exclusao_pedida: "Pediu para apagar a conta",
  exclusao_cancelada: "Cancelou a exclusão da conta",
  permissao_alterada: "Permissão alterada",
  acesso_concedido: "Acesso dado a outra pessoa",
  acesso_removido: "Acesso removido",
  membro_removido: "Membro removido da casa",
  entidade_apagada: "Entidade apagada",
  conta_apagada: "Conta bancária apagada",
  open_finance_credenciais: "Credenciais do Meu Pluggy salvas",
  open_finance_desconectado: "Meu Pluggy desconectado",
  ia_config: "Configuração da IA mudou",
  telegram_codigo: "Link do Telegram gerado",
  telegram_desconectado: "Telegram desconectado",
};

const nomesEnvio: Record<string, string> = {
  categorizar: "Categorização",
  chat: "Pergunta",
  relatorio: "Relatório",
  documento: "Documento",
};

/** Resumo do aparelho a partir do user-agent (só para reconhecer). */
function aparelho(ua: string | null): string {
  if (!ua) return "";
  const so = /iPhone|iPad/.test(ua)
    ? "iPhone"
    : /Android/.test(ua)
      ? "Android"
      : /Windows/.test(ua)
        ? "Windows"
        : /Mac OS/.test(ua)
          ? "Mac"
          : /Linux/.test(ua)
            ? "Linux"
            : "";
  const nav = /Edg\//.test(ua)
    ? "Edge"
    : /Firefox\//.test(ua)
      ? "Firefox"
      : /Chrome\//.test(ua)
        ? "Chrome"
        : /Safari\//.test(ua)
          ? "Safari"
          : "";
  return [so, nav].filter(Boolean).join(" · ");
}

export function MeusDados() {
  const comReautenticacao = useComReautenticacao();
  const atividade = useQuery({
    queryKey: ["conta", "atividade"],
    queryFn: () =>
      obter<{ acao: string; ip: string | null; aparelho: string | null; quando: string }[]>(
        "/api/conta/atividade",
      ),
  });
  const envios = useQuery({
    queryKey: ["ia", "envios"],
    queryFn: () =>
      obter<{ tipo: string; resumo: string; quando: string; custo_microdolares: number }[]>("/api/ia/envios"),
  });
  const exportar = useMutation({
    mutationFn: (formato: "json" | "planilha") =>
      comReautenticacao(() => baixar("/api/conta/exportar", { formato })),
  });

  return (
    <Pagina titulo="Meus dados" subtitulo="Baixar tudo, ver o que aconteceu na conta e o que foi para a IA.">
      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <Download className="size-4 text-destaque" aria-hidden /> Exportar meus dados
          </span>
        </TituloCartao>
        <p className="text-sm text-texto-2">
          Tudo o que é seu: entidades (com CPF/CNPJ), contas, transações, categorias, orçamento, metas,
          investimentos, CNPJ, relatórios e o histórico. Senhas, segredos do 2FA e credenciais ficam de fora.
          Pede a senha de novo.
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <Botao
            variante="secundario"
            onClick={() => exportar.mutate("json")}
            carregando={exportar.isPending && exportar.variables === "json"}
          >
            <Download className="size-4" aria-hidden /> Tudo (JSON)
          </Botao>
          <Botao
            variante="secundario"
            onClick={() => exportar.mutate("planilha")}
            carregando={exportar.isPending && exportar.variables === "planilha"}
          >
            <FileSpreadsheet className="size-4" aria-hidden /> Planilha (.xlsx)
          </Botao>
        </div>
        {exportar.error ? (
          <div className="mt-3">
            <Aviso tipo="erro">{mensagemDe(exportar.error)}</Aviso>
          </div>
        ) : null}
        {exportar.isSuccess && exportar.data ? (
          <div className="mt-3">
            <Aviso tipo="sucesso">
              Arquivo baixado. Guarde em lugar seguro: ele tem todos os seus dados.
            </Aviso>
          </div>
        ) : null}
      </Cartao>

      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <History className="size-4 text-destaque" aria-hidden /> Atividade da conta
          </span>
        </TituloCartao>
        {atividade.isPending ? (
          <CarregandoLista />
        ) : (
          <ul
            className="flex max-h-96 flex-col divide-y divide-borda overflow-y-auto"
            aria-label="Atividade da conta"
          >
            {(atividade.data ?? []).map((e) => (
              <li
                key={`${e.quando}-${e.acao}`}
                className="flex flex-wrap items-baseline justify-between gap-x-3 py-2 text-sm"
              >
                <span className="font-medium">{nomesAcao[e.acao] ?? e.acao.replaceAll("_", " ")}</span>
                <span className="text-xs text-texto-2">
                  {formatarDataHora(e.quando)}
                  {e.aparelho ? ` · ${aparelho(e.aparelho)}` : ""}
                  {e.ip ? ` · ${e.ip}` : ""}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Cartao>

      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <Sparkles className="size-4 text-destaque" aria-hidden /> O que a IA viu
          </span>
        </TituloCartao>
        <p className="mb-3 text-sm text-texto-2">
          Cada envio ao Claude, com o resumo do que foi. O conteúdo não fica guardado.{" "}
          <Link to="/ia" className="font-semibold text-primaria">
            Ligar ou desligar a IA
          </Link>
        </p>
        {envios.data?.length ? (
          <ul
            className="flex max-h-96 flex-col divide-y divide-borda overflow-y-auto"
            aria-label="Envios à IA"
          >
            {envios.data.map((e) => (
              <li key={`${e.quando}-${e.tipo}`} className="py-2 text-sm">
                <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                  <span className="font-medium">{nomesEnvio[e.tipo] ?? e.tipo}</span>
                  <span className="text-xs text-texto-2">
                    {formatarDataHora(e.quando)} · {dolares(e.custo_microdolares)}
                  </span>
                </div>
                <p className="mt-0.5 break-words text-xs text-texto-2">{e.resumo}</p>
              </li>
            ))}
          </ul>
        ) : envios.isPending ? (
          <CarregandoLista linhas={2} />
        ) : (
          <p className="text-sm text-texto-2">Nada foi enviado à IA.</p>
        )}
      </Cartao>

      <ApagarConta />
    </Pagina>
  );
}

function ApagarConta() {
  const comReautenticacao = useComReautenticacao();
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const [texto, setTexto] = useState("");
  const apagar = useMutation({
    mutationFn: () =>
      comReautenticacao(() =>
        api<{ apagar_em: string }>("POST", "/api/conta/apagar", { confirmacao: texto.trim() }),
      ),
    onSuccess: async (r) => {
      if (!r) return;
      cliente.clear();
      navegar(`/entrar?apagada=${encodeURIComponent(r.apagar_em.slice(0, 10))}`, { replace: true });
    },
  });
  return (
    <Cartao>
      <TituloCartao>
        <span className="flex items-center gap-2 text-saida">
          <Trash2 className="size-4" aria-hidden /> Apagar minha conta
        </span>
      </TituloCartao>
      <p className="text-sm text-texto-2">
        Suas entidades, contas, transações e todo o resto somem de verdade em 30 dias. Até lá, é só entrar de
        novo para cancelar. Casas em que você é o único dono passam para o membro mais antigo. Exporte antes
        se quiser guardar.
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          apagar.mutate();
        }}
        className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end"
      >
        <Campo
          rotulo="Digite APAGAR para confirmar"
          value={texto}
          onChange={(e) => setTexto(e.target.value)}
          autoComplete="off"
          className="sm:flex-1"
        />
        <Botao
          type="submit"
          variante="perigo"
          carregando={apagar.isPending}
          disabled={texto.trim() !== "APAGAR"}
        >
          Apagar em 30 dias
        </Botao>
      </form>
      {apagar.error ? (
        <div className="mt-3">
          <Aviso tipo="erro">{mensagemDe(apagar.error)}</Aviso>
        </div>
      ) : null}
    </Cartao>
  );
}

/** Aviso no topo enquanto a conta está marcada para exclusão. */
export function AvisoExclusao({ apagarEm }: { apagarEm: string }) {
  const cliente = useQueryClient();
  const cancelar = useMutation({
    mutationFn: () => api("POST", "/api/conta/cancelar-exclusao"),
    onSuccess: () => cliente.invalidateQueries({ queryKey: ["sessao"] }),
  });
  return (
    <div className="mb-4">
      <Aviso tipo="alerta">
        <span className="flex flex-wrap items-center justify-between gap-2">
          <span>Sua conta será apagada em {formatarData(apagarEm.slice(0, 10))}.</span>
          <Botao variante="secundario" onClick={() => cancelar.mutate()} carregando={cancelar.isPending}>
            Cancelar exclusão
          </Botao>
        </span>
      </Aviso>
    </div>
  );
}
