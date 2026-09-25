import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, Check, ChevronRight, Copy, LogOut, Plus, Share2, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
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
import { formatarDataHora } from "../lib/formato";
import { useSessao } from "../lib/sessao";
import { type ConvitePendente, type Membro, nomesPapel, type Casa as TCasa } from "../lib/tipos";

export function Casas() {
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const [nome, setNome] = useState("");
  const { data, isPending } = useQuery({ queryKey: ["casas"], queryFn: () => obter<TCasa[]>("/api/casas") });
  const criar = useMutation({
    mutationFn: () => api<{ id: string }>("POST", "/api/casas", { nome }),
    onSuccess: async (r) => {
      await cliente.invalidateQueries({ queryKey: ["casas"] });
      navegar(`/casa/${r.id}`);
    },
  });

  return (
    <Pagina titulo="Casa" subtitulo="O que cada pessoa compartilha, e só isso, aparece para os outros.">
      <Cartao>
        {isPending ? (
          <CarregandoLista linhas={2} />
        ) : data?.length ? (
          <ul className="flex flex-col divide-y divide-borda">
            {data.map((c) => (
              <li key={c.id}>
                <Link
                  to={`/casa/${c.id}`}
                  className="flex min-h-14 items-center gap-3 rounded-lg px-1 hover:bg-superficie-2"
                >
                  <Building2 className="size-5 text-primaria" aria-hidden />
                  <span className="flex-1 font-medium">{c.nome}</span>
                  <Etiqueta>{nomesPapel[c.papel]}</Etiqueta>
                  <ChevronRight className="size-4 text-texto-2" aria-hidden />
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <Vazio
            icone={<Building2 className="size-8" />}
            titulo="Nenhuma casa"
            texto="Crie a casa e convide quem mora com você."
          />
        )}
      </Cartao>
      <Cartao>
        <TituloCartao>Nova casa</TituloCartao>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            criar.mutate();
          }}
          className="flex flex-col gap-3 sm:flex-row sm:items-end"
        >
          <Campo
            rotulo="Nome"
            className="flex-1"
            value={nome}
            onChange={(e) => setNome(e.target.value)}
            required
            maxLength={100}
            placeholder="Casa da família"
          />
          <Botao type="submit" carregando={criar.isPending}>
            <Plus className="size-4" aria-hidden /> Criar
          </Botao>
        </form>
        {criar.error ? (
          <div className="mt-3">
            <Aviso tipo="erro">{mensagemDe(criar.error)}</Aviso>
          </div>
        ) : null}
      </Cartao>
    </Pagina>
  );
}

type DetalheCasa = TCasa & { membros: Membro[]; convites: ConvitePendente[] };

export function DetalheCasa() {
  const { id = "" } = useParams();
  const { data: sessao } = useSessao();
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const chave = ["casas", id];
  const { data, isPending, error } = useQuery({
    queryKey: chave,
    queryFn: () => obter<DetalheCasa>(`/api/casas/${id}`),
  });
  const invalidar = () => cliente.invalidateQueries({ queryKey: chave });

  const papel = useMutation({
    mutationFn: ({ usuario, papel }: { usuario: string; papel: string }) =>
      api("PATCH", `/api/casas/${id}/membros/${usuario}`, { papel }),
    onSuccess: invalidar,
  });
  const remover = useMutation({
    mutationFn: (usuario: string) => api("DELETE", `/api/casas/${id}/membros/${usuario}`),
    onSuccess: async (_, usuario) => {
      if (usuario === sessao?.usuario_id) {
        await cliente.invalidateQueries({ queryKey: ["casas"] });
        navegar("/casa", { replace: true });
      } else {
        await invalidar();
      }
    },
  });

  if (isPending) return <CarregandoLista />;
  if (error || !data) return <Aviso tipo="erro">{mensagemDe(error)}</Aviso>;
  const dono = data.papel === "dono";
  const erro = papel.error ?? remover.error;

  return (
    <Pagina titulo={data.nome} subtitulo={`Você é ${nomesPapel[data.papel]?.toLowerCase()} desta casa.`}>
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      <Cartao>
        <TituloCartao>Membros</TituloCartao>
        <ul className="flex flex-col divide-y divide-borda">
          {data.membros.map((m) => {
            const eu = m.usuario_id === sessao?.usuario_id;
            return (
              <li key={m.usuario_id} className="flex min-h-14 flex-wrap items-center gap-3 py-2">
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {m.nome} {eu ? <span className="text-texto-2">(você)</span> : null}
                  </span>
                  <span className="block truncate text-xs text-texto-2">{m.email}</span>
                </span>
                {dono && !eu ? (
                  <select
                    aria-label={`Papel de ${m.nome}`}
                    value={m.papel}
                    onChange={(e) => papel.mutate({ usuario: m.usuario_id, papel: e.target.value })}
                    className="min-h-11 rounded-xl border border-borda bg-superficie-2 px-2 text-sm"
                  >
                    <option value="dono">Dono</option>
                    <option value="membro">Membro</option>
                    <option value="leitor">Leitor</option>
                  </select>
                ) : (
                  <Etiqueta>{nomesPapel[m.papel]}</Etiqueta>
                )}
                {(dono && !eu) || (eu && !dono) ? (
                  <Botao
                    variante="fantasma"
                    aria-label={eu ? "Sair da casa" : `Remover ${m.nome}`}
                    onClick={() => {
                      if (
                        confirm(
                          eu
                            ? "Sair desta casa? Suas contas compartilhadas deixam de aparecer para ela."
                            : `Remover ${m.nome} da casa?`,
                        )
                      ) {
                        remover.mutate(m.usuario_id);
                      }
                    }}
                  >
                    {eu ? <LogOut className="size-4" /> : <Trash2 className="size-4" />}
                  </Botao>
                ) : null}
              </li>
            );
          })}
        </ul>
      </Cartao>
      {dono ? <Convites casa={data} aoMudar={invalidar} /> : null}
    </Pagina>
  );
}

function Convites({ casa, aoMudar }: { casa: DetalheCasa; aoMudar: () => void }) {
  const [papel, setPapel] = useState<"membro" | "leitor">("membro");
  const [link, setLink] = useState<{ link: string; expira_em: string } | null>(null);
  const [copiado, setCopiado] = useState(false);
  const criar = useMutation({
    mutationFn: () =>
      api<{ link: string; expira_em: string }>("POST", `/api/casas/${casa.id}/convites`, { papel }),
    onSuccess: (r) => {
      setLink(r);
      setCopiado(false);
      aoMudar();
    },
  });
  const cancelar = useMutation({
    mutationFn: (convite: string) => api("DELETE", `/api/casas/${casa.id}/convites/${convite}`),
    onSuccess: aoMudar,
  });

  const copiar = async () => {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link.link);
      setCopiado(true);
    } catch {
      setCopiado(false);
    }
  };
  const compartilhar = () =>
    link && navigator.share?.({ title: `Convite para ${casa.nome}`, url: link.link }).catch(() => undefined);

  return (
    <Cartao>
      <TituloCartao>Convidar</TituloCartao>
      <p className="mb-3 text-sm text-texto-2">
        O link vale por 48 horas e para uma pessoa só. Cada um cria a própria senha e passkey.
      </p>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <Selecao
          rotulo="Papel"
          value={papel}
          onChange={(e) => setPapel(e.target.value as "membro" | "leitor")}
          className="sm:w-48"
        >
          <option value="membro">Membro — gerencia o que é seu e compartilha</option>
          <option value="leitor">Leitor — só vê o que compartilharem</option>
        </Selecao>
        <Botao onClick={() => criar.mutate()} carregando={criar.isPending}>
          Gerar link
        </Botao>
      </div>
      {criar.error ? (
        <div className="mt-3">
          <Aviso tipo="erro">{mensagemDe(criar.error)}</Aviso>
        </div>
      ) : null}
      {link ? (
        <div className="mt-4 flex flex-col gap-2 rounded-xl border border-borda bg-superficie-2 p-3">
          <code className="break-all text-sm">{link.link}</code>
          <p className="text-xs text-texto-2">
            Expira em {formatarDataHora(link.expira_em)}. Não será mostrado de novo.
          </p>
          <div className="flex flex-wrap gap-2">
            <Botao variante="secundario" onClick={copiar}>
              {copiado ? <Check className="size-4" /> : <Copy className="size-4" />}{" "}
              {copiado ? "Copiado" : "Copiar"}
            </Botao>
            {"share" in navigator ? (
              <Botao variante="secundario" onClick={compartilhar}>
                <Share2 className="size-4" /> Compartilhar
              </Botao>
            ) : null}
          </div>
        </div>
      ) : null}
      {casa.convites.length ? (
        <>
          <h3 className="mt-5 mb-2 text-sm font-semibold">Convites pendentes</h3>
          <ul className="flex flex-col divide-y divide-borda">
            {casa.convites.map((c) => (
              <li key={c.id} className="flex min-h-12 items-center gap-3 text-sm">
                <Etiqueta>{nomesPapel[c.papel]}</Etiqueta>
                <span className="flex-1 text-texto-2">expira {formatarDataHora(c.expira_em)}</span>
                <Botao
                  variante="fantasma"
                  aria-label="Cancelar convite"
                  onClick={() => cancelar.mutate(c.id)}
                >
                  <Trash2 className="size-4" />
                </Botao>
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </Cartao>
  );
}
