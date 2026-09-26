import { useMutation } from "@tanstack/react-query";
import { Send, Sparkles } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { AvisoSimulacao } from "../components/extras";
import { Aviso, Botao, Etiqueta, Pagina } from "../components/ui";
import { api, mensagemDe } from "../lib/api";
import { useConfigIA } from "../lib/dados";
import { nomesFerramentaIA, type RespostaChat } from "../lib/tipos";

type Mensagem = { papel: "usuario" | "assistente"; texto: string; ferramentas?: string[] };

const sugestoes = [
  "Quanto gastei por categoria este mês?",
  "Quais contas vencem nos próximos 7 dias?",
  "Quanto gastei com Uber nos últimos 3 meses?",
  "Como estão meus investimentos contra o CDI?",
];

export function Perguntar() {
  const config = useConfigIA();
  const [mensagens, setMensagens] = useState<Mensagem[]>([]);
  const [texto, setTexto] = useState("");
  const fim = useRef<HTMLDivElement>(null);

  const perguntar = useMutation({
    mutationFn: (historico: Mensagem[]) =>
      api<RespostaChat>("POST", "/api/ia/chat", {
        // as últimas 20 mensagens bastam; o servidor não guarda nada
        mensagens: historico.slice(-19).map(({ papel, texto }) => ({ papel, texto })),
      }),
    onSuccess: (r) =>
      setMensagens((m) => [...m, { papel: "assistente", texto: r.texto, ferramentas: r.ferramentas }]),
  });

  // rola para o fim a cada mensagem nova (e quando aparece o "consultando")
  const itensNaTela = mensagens.length + (perguntar.isPending ? 1 : 0);
  useEffect(() => {
    if (itensNaTela > 0) fim.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [itensNaTela]);

  const enviar = (t: string) => {
    const pergunta = t.trim();
    if (!pergunta || perguntar.isPending) return;
    const historico = [...mensagens, { papel: "usuario" as const, texto: pergunta }];
    setMensagens(historico);
    setTexto("");
    perguntar.mutate(historico);
  };

  const c = config.data;
  if (c && (!c.ativa || !c.servidor)) {
    return (
      <Pagina titulo="Pergunte às suas finanças">
        <Aviso tipo="info">
          {c.servidor ? "A IA está desligada para você." : "A IA ainda não está configurada neste servidor."}{" "}
          <Link to="/ia" className="font-semibold underline">
            Ver configuração
          </Link>
        </Aviso>
      </Pagina>
    );
  }

  return (
    <Pagina titulo="Pergunte às suas finanças" subtitulo="A resposta usa os mesmos números das telas.">
      <div className="flex min-h-[50dvh] flex-col gap-3" aria-live="polite">
        {mensagens.length === 0 ? (
          <div className="flex flex-col gap-3">
            <p className="flex items-center gap-2 text-sm text-texto-2">
              <Sparkles className="size-4 text-destaque" aria-hidden /> Experimente:
            </p>
            <div className="flex flex-wrap gap-2">
              {sugestoes.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => enviar(s)}
                  className="min-h-11 rounded-xl border border-borda bg-superficie px-3 text-left text-sm hover:border-texto-2"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        ) : null}
        {mensagens.map((m, i) => (
          <div
            // biome-ignore lint/suspicious/noArrayIndexKey: a conversa só cresce no fim
            key={i}
            className={`max-w-[92%] rounded-cartao px-4 py-3 text-sm ${
              m.papel === "usuario"
                ? "self-end bg-primaria text-primaria-texto"
                : "self-start border border-borda bg-superficie"
            }`}
          >
            <p className="whitespace-pre-wrap">{m.texto}</p>
            {m.ferramentas?.length ? (
              <p className="mt-2 flex flex-wrap gap-1">
                {[...new Set(m.ferramentas)].map((f) => (
                  <Etiqueta key={f}>{nomesFerramentaIA[f] ?? f}</Etiqueta>
                ))}
              </p>
            ) : null}
          </div>
        ))}
        {perguntar.isPending ? (
          <p className="self-start text-sm text-texto-2" role="status">
            Consultando as suas finanças…
          </p>
        ) : null}
        {perguntar.error ? <Aviso tipo="erro">{mensagemDe(perguntar.error)}</Aviso> : null}
        <div ref={fim} />
      </div>
      <form
        className="sticky bottom-20 flex gap-2 rounded-cartao border border-borda bg-superficie p-2 sm:bottom-4"
        onSubmit={(e) => {
          e.preventDefault();
          enviar(texto);
        }}
      >
        <label htmlFor="pergunta" className="sr-only">
          Sua pergunta
        </label>
        <input
          id="pergunta"
          value={texto}
          onChange={(e) => setTexto(e.target.value)}
          maxLength={2000}
          placeholder="Pergunte sobre gastos, contas, investimentos…"
          className="min-h-11 min-w-0 flex-1 rounded-xl bg-superficie-2 px-3 text-base text-texto placeholder:text-texto-2/70 focus:outline-none"
        />
        <Botao type="submit" aria-label="Enviar" carregando={perguntar.isPending} disabled={!texto.trim()}>
          <Send className="size-4" aria-hidden />
        </Botao>
      </form>
      <AvisoSimulacao />
    </Pagina>
  );
}
