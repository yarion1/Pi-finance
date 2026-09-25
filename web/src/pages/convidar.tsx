import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Link2, Trash2, UserPlus } from "lucide-react";
import { useState } from "react";
import {
  Aviso,
  Botao,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { formatarDataHora } from "../lib/formato";

type ConviteConta = {
  id: string;
  criado_em: string;
  expira_em: string;
  aceito_em: string | null;
  aceito_por: string | null;
};

/** Convite de conta: um amigo cria a própria conta, sem entrar na sua casa. */
export function Convidar() {
  const cliente = useQueryClient();
  const lista = useQuery({
    queryKey: ["convites-conta"],
    queryFn: () => obter<ConviteConta[]>("/api/convites-conta"),
  });
  const [link, setLink] = useState("");
  const [copiado, setCopiado] = useState(false);
  const gerar = useMutation({
    mutationFn: () => api<{ link: string }>("POST", "/api/convites-conta"),
    onSuccess: (r) => {
      setLink(r.link);
      setCopiado(false);
      return cliente.invalidateQueries({ queryKey: ["convites-conta"] });
    },
  });
  const cancelar = useMutation({
    mutationFn: (id: string) => api("DELETE", `/api/convites-conta/${id}`),
    onSuccess: () => cliente.invalidateQueries({ queryKey: ["convites-conta"] }),
  });
  const copiar = async () => {
    try {
      await navigator.clipboard.writeText(link);
      setCopiado(true);
    } catch {
      setCopiado(false);
    }
  };
  const erro = gerar.error ?? cancelar.error ?? lista.error;

  return (
    <Pagina titulo="Convidar pessoas" subtitulo="Para um amigo usar o Finanças com a conta dele.">
      {erro ? <Aviso tipo="erro">{mensagemDe(erro)}</Aviso> : null}
      <Cartao>
        <p className="text-sm text-texto-2">
          A pessoa cria a própria conta, com senha e segundo fator, e os dados dela ficam só com ela: ela não
          entra na sua casa e você não vê nada dela. O link vale 48 horas e só uma vez. Para morar junto e
          compartilhar contas, use o convite da casa.
        </p>
        <div className="mt-4 flex flex-wrap gap-2">
          <Botao onClick={() => gerar.mutate()} carregando={gerar.isPending}>
            <UserPlus className="size-4" aria-hidden /> Gerar link de convite
          </Botao>
        </div>
        {link ? (
          <div className="mt-4 flex flex-col gap-2">
            <label htmlFor="link-convite" className="text-sm font-medium">
              Link para mandar
            </label>
            <div className="flex gap-2">
              <input
                id="link-convite"
                readOnly
                value={link}
                onFocus={(e) => e.target.select()}
                className="min-h-11 min-w-0 flex-1 rounded-xl border border-borda bg-superficie-2 px-3 font-mono text-sm"
              />
              <Botao variante="secundario" onClick={copiar} aria-label="Copiar link">
                <Copy className="size-4" aria-hidden /> {copiado ? "Copiado" : "Copiar"}
              </Botao>
            </div>
          </div>
        ) : null}
      </Cartao>

      <Cartao>
        <TituloCartao>Convites</TituloCartao>
        {lista.isPending ? (
          <CarregandoLista />
        ) : lista.data?.length ? (
          <ul className="flex flex-col divide-y divide-borda">
            {lista.data.map((c) => (
              <li key={c.id} className="flex min-h-14 items-center gap-3 py-2">
                <Link2 className="size-4 shrink-0 text-texto-2" aria-hidden />
                <span className="min-w-0 flex-1 text-sm">
                  {c.aceito_por ? (
                    <>
                      Usado por <strong>{c.aceito_por}</strong>
                    </>
                  ) : (
                    <>Criado em {formatarDataHora(c.criado_em)}</>
                  )}
                  <span className="block text-xs text-texto-2">
                    {c.aceito_em
                      ? `em ${formatarDataHora(c.aceito_em)}`
                      : `vale até ${formatarDataHora(c.expira_em)}`}
                  </span>
                </span>
                {c.aceito_em ? (
                  <Etiqueta cor="entrada">usado</Etiqueta>
                ) : (
                  <Botao
                    variante="fantasma"
                    aria-label="Cancelar convite"
                    onClick={() => cancelar.mutate(c.id)}
                  >
                    <Trash2 className="size-4" />
                  </Botao>
                )}
              </li>
            ))}
          </ul>
        ) : (
          <Vazio titulo="Nenhum convite" texto="Gere um link e mande para a pessoa." />
        )}
      </Cartao>
    </Pagina>
  );
}
