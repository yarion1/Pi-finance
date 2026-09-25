import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, RefreshCw, Smartphone, Trash2 } from "lucide-react";
import { useState } from "react";
import { CodigosRecuperacao, ConfigurarPasskey, ConfigurarTOTP } from "../components/fatores";
import { useComReautenticacao } from "../components/reautenticacao";
import { Aviso, Botao, CarregandoLista, Cartao, Etiqueta, Pagina, TituloCartao } from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { formatarDataHora } from "../lib/formato";
import { suportaPasskey } from "../lib/passkey";
import { chaveSessao, useSessao } from "../lib/sessao";

type Passkey = { id: string; nome: string; criada_em: string; usada_em: string | null };
type Painel = null | "totp" | "passkey";

export function Seguranca() {
  const { data: sessao } = useSessao();
  const cliente = useQueryClient();
  const comReautenticacao = useComReautenticacao();
  const [painel, setPainel] = useState<Painel>(null);
  const [codigos, setCodigos] = useState<string[] | null>(null);
  const [erro, setErro] = useState("");
  const passkeys = useQuery({
    queryKey: ["passkeys"],
    queryFn: () => obter<Passkey[]>("/api/auth/passkeys"),
  });

  const atualizar = async () => {
    setPainel(null);
    await Promise.all([
      cliente.invalidateQueries({ queryKey: chaveSessao }),
      cliente.invalidateQueries({ queryKey: ["passkeys"] }),
    ]);
  };

  const remover = useMutation({
    mutationFn: (id: string) => comReautenticacao(() => api("DELETE", `/api/auth/passkeys/${id}`)),
    onSuccess: atualizar,
    onError: (e) => setErro(mensagemDe(e)),
  });

  const novosCodigos = async () => {
    setErro("");
    try {
      const r = await comReautenticacao(() =>
        api<{ codigos_recuperacao: string[] }>("POST", "/api/auth/recuperacao/gerar"),
      );
      if (r) setCodigos(r.codigos_recuperacao);
    } catch (e) {
      setErro(mensagemDe(e));
    }
  };

  const trocarTotp = async () => {
    setErro("");
    // trocar um TOTP ativo pede a senha antes de gerar o segredo novo
    if (sessao?.tem_totp && !sessao.reautenticada) {
      try {
        const ok = await comReautenticacao(() => api("POST", "/api/auth/totp/iniciar").then(() => true));
        if (!ok) return;
        await cliente.invalidateQueries({ queryKey: chaveSessao });
      } catch (e) {
        setErro(mensagemDe(e));
        return;
      }
    }
    setPainel("totp");
  };

  return (
    <Pagina titulo="Segurança" subtitulo={sessao?.email}>
      {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
      {codigos ? (
        <Cartao>
          <TituloCartao>Novos códigos de recuperação</TituloCartao>
          <CodigosRecuperacao codigos={codigos} aoContinuar={() => setCodigos(null)} />
        </Cartao>
      ) : null}

      <Cartao>
        <TituloCartao
          acao={
            painel ? null : (
              <Botao variante="secundario" onClick={trocarTotp}>
                {sessao?.tem_totp ? "Trocar aplicativo" : "Ativar"}
              </Botao>
            )
          }
        >
          <span className="flex items-center gap-2">
            <Smartphone className="size-5 text-primaria" aria-hidden /> Aplicativo autenticador
          </span>
        </TituloCartao>
        {painel === "totp" ? (
          <ConfigurarTOTP aoConcluir={atualizar} aoCancelar={() => setPainel(null)} />
        ) : (
          <p className="text-sm text-texto-2">
            {sessao?.tem_totp ? <Etiqueta cor="entrada">Ativo</Etiqueta> : "Não configurado."}
          </p>
        )}
      </Cartao>

      <Cartao>
        <TituloCartao
          acao={
            painel || !suportaPasskey() ? null : (
              <Botao variante="secundario" onClick={() => setPainel("passkey")}>
                <Plus className="size-4" aria-hidden /> Adicionar
              </Botao>
            )
          }
        >
          <span className="flex items-center gap-2">
            <KeyRound className="size-5 text-primaria" aria-hidden /> Passkeys
          </span>
        </TituloCartao>
        {painel === "passkey" ? (
          <ConfigurarPasskey aoConcluir={atualizar} aoCancelar={() => setPainel(null)} />
        ) : null}
        {passkeys.isPending ? (
          <CarregandoLista linhas={1} />
        ) : passkeys.data?.length ? (
          <ul className="flex flex-col divide-y divide-borda">
            {passkeys.data.map((p) => (
              <li key={p.id} className="flex min-h-14 items-center gap-3">
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{p.nome}</span>
                  <span className="block text-xs text-texto-2">
                    criada {formatarDataHora(p.criada_em)}
                    {p.usada_em ? ` · usada ${formatarDataHora(p.usada_em)}` : ""}
                  </span>
                </span>
                <Botao
                  variante="fantasma"
                  aria-label={`Remover ${p.nome}`}
                  onClick={() => confirm(`Remover a passkey "${p.nome}"?`) && remover.mutate(p.id)}
                >
                  <Trash2 className="size-4" />
                </Botao>
              </li>
            ))}
          </ul>
        ) : painel !== "passkey" ? (
          <p className="text-sm text-texto-2">
            Nenhuma passkey. Com ela, o login é só o desbloqueio do aparelho.
          </p>
        ) : null}
      </Cartao>

      <Cartao>
        <TituloCartao>Códigos de recuperação</TituloCartao>
        <p className="mb-3 text-sm text-texto-2">
          Gerar novos invalida os anteriores. Use se tiver perdido os antigos ou gastado vários.
        </p>
        <Botao variante="secundario" onClick={novosCodigos}>
          <RefreshCw className="size-4" aria-hidden /> Gerar novos códigos
        </Botao>
      </Cartao>
    </Pagina>
  );
}
