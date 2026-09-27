import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DatabaseBackup, KeyRound, Plus, RefreshCw, Smartphone, Trash2 } from "lucide-react";
import { useState } from "react";
import { CodigosRecuperacao, ConfigurarPasskey, ConfigurarTOTP } from "../components/fatores";
import { useComReautenticacao } from "../components/reautenticacao";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  TituloCartao,
} from "../components/ui";
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

      <TrocarSenha />

      <Cartao>
        <TituloCartao>Códigos de recuperação</TituloCartao>
        <p className="mb-3 text-sm text-texto-2">
          Gerar novos invalida os anteriores. Use se tiver perdido os antigos ou gastado vários.
        </p>
        <Botao variante="secundario" onClick={novosCodigos}>
          <RefreshCw className="size-4" aria-hidden /> Gerar novos códigos
        </Botao>
      </Cartao>

      <Backups />
    </Pagina>
  );
}

function TrocarSenha() {
  const [atual, setAtual] = useState("");
  const [nova, setNova] = useState("");
  const [repetida, setRepetida] = useState("");
  const trocar = useMutation({
    mutationFn: () => api("POST", "/api/auth/senha", { atual, nova }),
    onSuccess: () => {
      setAtual("");
      setNova("");
      setRepetida("");
    },
  });
  const curta = nova.length > 0 && [...nova].length < 12;
  const diferente = repetida.length > 0 && repetida !== nova;
  return (
    <Cartao>
      <TituloCartao>Trocar senha</TituloCartao>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          trocar.mutate();
        }}
        className="flex flex-col gap-3"
      >
        <Campo
          rotulo="Senha atual"
          type="password"
          autoComplete="current-password"
          value={atual}
          onChange={(e) => setAtual(e.target.value)}
          required
        />
        <Campo
          rotulo="Senha nova"
          type="password"
          autoComplete="new-password"
          value={nova}
          onChange={(e) => setNova(e.target.value)}
          required
          maxLength={128}
          erro={curta ? "Pelo menos 12 caracteres (uma frase serve)" : undefined}
        />
        <Campo
          rotulo="Repita a senha nova"
          type="password"
          autoComplete="new-password"
          value={repetida}
          onChange={(e) => setRepetida(e.target.value)}
          required
          erro={diferente ? "As duas não batem" : undefined}
        />
        <p className="text-xs text-texto-2">Os outros aparelhos saem da conta na hora.</p>
        {trocar.error ? <Aviso tipo="erro">{mensagemDe(trocar.error)}</Aviso> : null}
        {trocar.isSuccess ? (
          <Aviso tipo="sucesso">Senha trocada. Os outros aparelhos saíram da conta.</Aviso>
        ) : null}
        <div>
          <Botao
            type="submit"
            carregando={trocar.isPending}
            disabled={!atual || curta || !nova || repetida !== nova}
          >
            Trocar senha
          </Botao>
        </div>
      </form>
    </Cartao>
  );
}

type EstadoBackups = {
  backup: {
    estado: "ok" | "atrasado" | "pendente";
    em: string | null;
    cifrado: boolean;
    nuvem: { em: string | null; ok: boolean } | null;
  };
  restore: { estado: "ok" | "atrasado" | "pendente" | "falhou"; em: string | null; segundos: number | null };
};

const rotuloEstado: Record<string, [string, "sucesso" | "alerta" | "erro" | "neutro"]> = {
  ok: ["em dia", "sucesso"],
  atrasado: ["atrasado", "alerta"],
  pendente: ["ainda não rodou", "neutro"],
  falhou: ["falhou", "erro"],
};

/** Backups do servidor: vale para a casa toda (os dados de todos estão no mesmo banco). */
function Backups() {
  const { data } = useQuery({
    queryKey: ["sistema", "backups"],
    queryFn: () => obter<EstadoBackups>("/api/sistema/backups"),
  });
  if (!data) return null;
  const linha = (nome: string, estado: string, detalhe: string) => {
    const [texto, tom] = rotuloEstado[estado] ?? [estado, "neutro"];
    const cor =
      tom === "sucesso"
        ? "text-entrada"
        : tom === "alerta"
          ? "text-alerta"
          : tom === "erro"
            ? "text-saida"
            : "text-texto-2";
    return (
      <li className="flex min-h-12 flex-wrap items-center justify-between gap-x-3 py-2 text-sm">
        <span>
          <span className="block font-medium">{nome}</span>
          <span className="text-xs text-texto-2">{detalhe}</span>
        </span>
        <span className={`font-semibold ${cor}`}>{texto}</span>
      </li>
    );
  };
  const { backup, restore } = data;
  return (
    <Cartao>
      <TituloCartao>
        <span className="flex items-center gap-2">
          <DatabaseBackup className="size-4 text-destaque" aria-hidden /> Backups do servidor
        </span>
      </TituloCartao>
      <ul className="flex flex-col divide-y divide-borda" aria-label="Estado dos backups">
        {linha(
          "Último backup",
          backup.estado,
          backup.em
            ? `${formatarDataHora(backup.em)}${backup.cifrado ? " · cifrado" : " · sem cifra"}`
            : "a cada 6 horas, cifrado",
        )}
        {backup.nuvem
          ? linha(
              "Cópia na nuvem",
              backup.nuvem.ok ? "ok" : "falhou",
              backup.nuvem.em ? formatarDataHora(backup.nuvem.em) : "uma vez por dia",
            )
          : null}
        {linha(
          "Teste de restore",
          restore.estado,
          restore.em
            ? `${formatarDataHora(restore.em)}${restore.segundos != null ? ` · ${restore.segundos} s` : ""}`
            : "toda semana, num banco temporário",
        )}
      </ul>
      <p className="mt-2 text-xs text-texto-2">
        O teste restaura o último backup e confere os totais com o banco. Vale para a casa toda.
      </p>
    </Cartao>
  );
}
