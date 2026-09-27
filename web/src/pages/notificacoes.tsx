import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, BellOff, Send, Smartphone, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { useComReautenticacao } from "../components/reautenticacao";
import { Aviso, Botao, Cartao, Pagina, TituloCartao } from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";

type Canal = { push: boolean; telegram: boolean };
type Preferencias = { tipos: Record<string, Canal>; mostrar_valores: boolean };
type Estado = {
  preferencias: Preferencias;
  tipos: string[];
  push_disponivel: boolean;
  chave_publica: string;
  aparelhos: { id: string; aparelho: string; criada_em: string }[];
  telegram_disponivel: boolean;
  telegram_conectado: boolean;
};

const nomes: Record<string, [string, string]> = {
  alertas: ["Alertas", "Gasto fora do padrão, cobrança duplicada, tarifa, assinatura nova, saldo diferente"],
  agenda: ["Contas do dia", "Às 8 h, o que vence hoje e amanhã"],
  relatorios: ["Relatórios", "Relatório do mês e resumo da semana prontos"],
  seguranca: ["Segurança", "Login em aparelho novo"],
};

/** Suporte a push neste navegador (no iPhone, só com o app instalado na tela de início). */
const temPush = () => "serviceWorker" in navigator && "PushManager" in window && "Notification" in window;

function chaveDoServidor(b64: string): Uint8Array<ArrayBuffer> {
  const s = atob(b64.replace(/-/g, "+").replace(/_/g, "/"));
  const out = new Uint8Array(new ArrayBuffer(s.length));
  for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
  return out;
}

/** "Android · Chrome", "iPhone · Safari"... só para reconhecer na lista. */
function nomeAparelho(): string {
  const ua = navigator.userAgent;
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
            : "Aparelho";
  const nav = /Edg\//.test(ua)
    ? "Edge"
    : /Firefox\//.test(ua)
      ? "Firefox"
      : /Chrome\//.test(ua)
        ? "Chrome"
        : /Safari\//.test(ua)
          ? "Safari"
          : "navegador";
  return `${so} · ${nav}`;
}

async function inscricaoLocal(): Promise<PushSubscription | null> {
  if (!temPush()) return null;
  const reg = await navigator.serviceWorker.getRegistration();
  return reg ? reg.pushManager.getSubscription() : null;
}

export function Notificacoes() {
  const cliente = useQueryClient();
  const comReautenticacao = useComReautenticacao();
  const [link, setLink] = useState("");
  const estado = useQuery({
    queryKey: ["notificacoes"],
    queryFn: () => obter<Estado>("/api/notificacoes"),
    // esperando a pessoa tocar em Iniciar no Telegram
    refetchInterval: (q) => (link && !q.state.data?.telegram_conectado ? 4000 : false),
  });
  const e = estado.data;
  const [local, setLocal] = useState<PushSubscription | null>(null);
  const [permissao, setPermissao] = useState(() => (temPush() ? Notification.permission : "default"));
  useEffect(() => {
    inscricaoLocal().then(setLocal, () => setLocal(null));
  }, []);
  useEffect(() => {
    if (e?.telegram_conectado) setLink("");
  }, [e?.telegram_conectado]);
  const atualizar = () => cliente.invalidateQueries({ queryKey: ["notificacoes"] });

  const ativar = useMutation({
    mutationFn: async () => {
      if (!e?.chave_publica) throw new Error("Notificações no aparelho indisponíveis neste servidor.");
      const p = await Notification.requestPermission();
      setPermissao(p);
      if (p !== "granted")
        throw new Error("Sem permissão para notificar. Libere nas configurações do navegador.");
      const reg = await navigator.serviceWorker.ready;
      const insc = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: chaveDoServidor(e.chave_publica),
      });
      const j = insc.toJSON();
      await api("POST", "/api/notificacoes/push", {
        endpoint: insc.endpoint,
        p256dh: j.keys?.p256dh ?? "",
        auth: j.keys?.auth ?? "",
        aparelho: nomeAparelho(),
      });
      setLocal(insc);
    },
    onSuccess: atualizar,
  });
  const desativar = useMutation({
    mutationFn: async () => {
      await local?.unsubscribe();
      setLocal(null);
    },
  });
  const remover = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/notificacoes/push/${id}`),
    onSuccess: atualizar,
  });
  // as escolhas mudam na hora (estado local) e vão para o servidor em seguida
  const [prefs, setPrefs] = useState<Preferencias | null>(null);
  useEffect(() => {
    if (e && !prefs) setPrefs(e.preferencias);
  }, [e, prefs]);
  const salvar = useMutation({
    mutationFn: (p: Preferencias) => api("PUT", "/api/notificacoes", p),
    onError: () => setPrefs(null),
    onSettled: atualizar,
  });
  const escolher = (p: Preferencias) => {
    setPrefs(p);
    salvar.mutate(p);
  };
  const conectar = useMutation({
    mutationFn: () =>
      comReautenticacao(() =>
        api<{ link: string }>("POST", "/api/notificacoes/telegram").then((r) => {
          setLink(r.link);
          return true;
        }),
      ),
  });
  const desconectar = useMutation({
    mutationFn: () => api("DELETE", "/api/notificacoes/telegram"),
    onSuccess: atualizar,
  });
  const testar = useMutation({
    mutationFn: () => api<{ enviados: number }>("POST", "/api/notificacoes/teste"),
  });

  const p = prefs ?? e?.preferencias;
  const quer = (tipo: string, canal: keyof Canal) => p?.tipos[tipo]?.[canal] ?? true;
  const mudar = (tipo: string, canal: keyof Canal, ligado: boolean) => {
    if (!p) return;
    const atual = { push: quer(tipo, "push"), telegram: quer(tipo, "telegram") };
    escolher({ ...p, tipos: { ...p.tipos, [tipo]: { ...atual, [canal]: ligado } } });
  };
  const semCanal = e && !e.aparelhos.length && !e.telegram_conectado;

  return (
    <Pagina titulo="Notificações" subtitulo="Avisos no celular e no Telegram, do jeito que você escolher.">
      {estado.error ? <Aviso tipo="erro">{mensagemDe(estado.error)}</Aviso> : null}

      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <Smartphone className="size-4 text-destaque" aria-hidden /> Neste aparelho
          </span>
        </TituloCartao>
        {!temPush() ? (
          <Aviso tipo="info">
            Este navegador não recebe notificações. No iPhone, instale o painel na tela de início
            (Compartilhar › Adicionar à Tela de Início) e abra por lá.
          </Aviso>
        ) : e && !e.push_disponivel ? (
          <Aviso tipo="alerta">As notificações no aparelho estão desligadas neste servidor.</Aviso>
        ) : permissao === "denied" ? (
          <Aviso tipo="alerta">
            O navegador bloqueou as notificações do painel. Libere nas configurações do site e volte aqui.
          </Aviso>
        ) : local ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="flex items-center gap-2 text-sm">
              <Bell className="size-4 text-entrada" aria-hidden /> Recebendo notificações aqui.
            </p>
            <Botao variante="secundario" onClick={() => desativar.mutate()} carregando={desativar.isPending}>
              <BellOff className="size-4" aria-hidden /> Parar neste aparelho
            </Botao>
          </div>
        ) : (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-texto-2">Toque para permitir e receber os avisos aqui.</p>
            <Botao onClick={() => ativar.mutate()} carregando={ativar.isPending} disabled={!e}>
              <Bell className="size-4" aria-hidden /> Ativar neste aparelho
            </Botao>
          </div>
        )}
        {ativar.error ? (
          <div className="mt-3">
            <Aviso tipo="erro">{mensagemDe(ativar.error)}</Aviso>
          </div>
        ) : null}
        {e?.aparelhos.length ? (
          <ul className="mt-4 flex flex-col divide-y divide-borda border-t border-borda">
            {e.aparelhos.map((a) => (
              <li key={a.id} className="flex min-h-12 items-center justify-between gap-3 py-2 text-sm">
                <span className="min-w-0">
                  <span className="block truncate font-medium">{a.aparelho || "Aparelho"}</span>
                  <span className="text-xs text-texto-2">
                    desde {new Date(a.criada_em).toLocaleDateString("pt-BR")}
                  </span>
                </span>
                <button
                  type="button"
                  onClick={() => remover.mutate(a.id)}
                  disabled={remover.isPending}
                  aria-label={`Remover ${a.aparelho || "aparelho"}`}
                  className="grid size-11 shrink-0 place-items-center rounded-xl text-texto-2 hover:bg-superficie-2 disabled:opacity-50"
                >
                  <Trash2 className="size-4" aria-hidden />
                </button>
              </li>
            ))}
          </ul>
        ) : null}
      </Cartao>

      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <Send className="size-4 text-destaque" aria-hidden /> Telegram
          </span>
        </TituloCartao>
        {e && !e.telegram_disponivel ? (
          <p className="text-sm text-texto-2">
            O bot do Telegram não está configurado neste servidor. Quem cuida do Pi pode criar um no
            @BotFather e colocar o token em TELEGRAM_PAINEL_TOKEN.
          </p>
        ) : e?.telegram_conectado ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm">Conectado: os avisos escolhidos abaixo chegam no seu Telegram.</p>
            <Botao
              variante="secundario"
              onClick={() => desconectar.mutate()}
              carregando={desconectar.isPending}
            >
              Desconectar
            </Botao>
          </div>
        ) : link ? (
          <div className="flex flex-col gap-3 text-sm">
            <p>
              Abra o link no celular com o Telegram e toque em <strong>Iniciar</strong>. Ele vale 15 minutos e
              uma vez só; esta tela atualiza sozinha.
            </p>
            <a
              href={link}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex min-h-11 items-center justify-center rounded-xl bg-primaria px-4 font-semibold text-primaria-texto"
            >
              Abrir o Telegram
            </a>
          </div>
        ) : (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-texto-2">
              Receba os avisos numa conversa com o bot do painel. Pede sua senha de novo.
            </p>
            <Botao onClick={() => conectar.mutate()} carregando={conectar.isPending} disabled={!e}>
              Conectar Telegram
            </Botao>
          </div>
        )}
        {conectar.error ? (
          <div className="mt-3">
            <Aviso tipo="erro">{mensagemDe(conectar.error)}</Aviso>
          </div>
        ) : null}
      </Cartao>

      {e ? (
        <Cartao>
          <TituloCartao>O que avisar</TituloCartao>
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-texto-2">
                <th className="py-2 font-medium">Tipo</th>
                <th className="w-20 py-2 text-center font-medium">Aparelho</th>
                <th className="w-20 py-2 text-center font-medium">Telegram</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-borda">
              {e.tipos.map((t) => (
                <tr key={t}>
                  <td className="py-2 pr-2">
                    <span className="block font-medium">{nomes[t]?.[0] ?? t}</span>
                    <span className="text-xs text-texto-2">{nomes[t]?.[1]}</span>
                  </td>
                  {(["push", "telegram"] as const).map((canal) => (
                    <td key={canal} className="text-center">
                      <label className="inline-grid size-11 place-items-center">
                        <input
                          type="checkbox"
                          className="size-5 accent-[var(--primaria)]"
                          checked={quer(t, canal)}
                          onChange={(ev) => mudar(t, canal, ev.target.checked)}
                          aria-label={`${nomes[t]?.[0] ?? t} no ${canal === "push" ? "aparelho" : "Telegram"}`}
                        />
                      </label>
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          <label className="mt-3 flex min-h-11 items-start gap-3 text-sm">
            <input
              type="checkbox"
              className="mt-0.5 size-5 shrink-0 accent-[var(--primaria)]"
              checked={p?.mostrar_valores ?? false}
              onChange={(ev) => p && escolher({ ...p, mostrar_valores: ev.target.checked })}
            />
            <span>
              Mostrar valores nas notificações
              <span className="block text-xs text-texto-2">
                Desligado, os valores aparecem como ••• (a notificação aparece na tela bloqueada).
              </span>
            </span>
          </label>
          {salvar.error ? (
            <div className="mt-3">
              <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso>
            </div>
          ) : null}
          <div className="mt-4 flex flex-wrap items-center gap-3">
            <Botao
              variante="secundario"
              onClick={() => testar.mutate()}
              carregando={testar.isPending}
              disabled={!!semCanal}
            >
              Mandar um teste
            </Botao>
            {semCanal ? (
              <span className="text-xs text-texto-2">Ative um aparelho ou o Telegram primeiro.</span>
            ) : null}
          </div>
          {testar.data ? (
            <div className="mt-3">
              <Aviso tipo={testar.data.enviados ? "sucesso" : "alerta"}>
                {testar.data.enviados
                  ? `Enviado para ${testar.data.enviados} ${testar.data.enviados === 1 ? "canal" : "canais"}.`
                  : "Nenhum canal recebeu. Confira as permissões do aparelho."}
              </Aviso>
            </div>
          ) : null}
          {testar.error ? (
            <div className="mt-3">
              <Aviso tipo="erro">{mensagemDe(testar.error)}</Aviso>
            </div>
          ) : null}
        </Cartao>
      ) : null}
    </Pagina>
  );
}
