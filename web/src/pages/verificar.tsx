import { useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useSair } from "../components/shell";
import { Aviso, Botao, Campo, TelaAuth } from "../components/ui";
import { api, mensagemDe } from "../lib/api";
import { entrarComPasskey, erroPasskey, suportaPasskey } from "../lib/passkey";
import { chaveSessao, useSessao } from "../lib/sessao";

/** Segundo passo do login: código do aplicativo, código de recuperação ou passkey. */
export function Verificar() {
  const { data: sessao } = useSessao();
  const [codigo, setCodigo] = useState("");
  const [recuperacao, setRecuperacao] = useState(false);
  const [erro, setErro] = useState("");
  const [enviando, setEnviando] = useState(false);
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const sair = useSair();

  const concluir = async () => {
    await cliente.invalidateQueries({ queryKey: chaveSessao });
    navegar("/", { replace: true });
  };

  const enviar = async (e: React.FormEvent) => {
    e.preventDefault();
    setErro("");
    setEnviando(true);
    try {
      await api("POST", "/api/auth/2fa/verificar", { codigo });
      await concluir();
    } catch (err) {
      setErro(mensagemDe(err));
      setCodigo("");
    } finally {
      setEnviando(false);
    }
  };

  const comPasskey = async () => {
    setErro("");
    try {
      await entrarComPasskey();
      await concluir();
    } catch (err) {
      const m = erroPasskey(err);
      if (m) setErro(m);
    }
  };

  const temTotp = sessao?.tem_totp;
  const temPasskey = (sessao?.passkeys ?? 0) > 0 && suportaPasskey();

  return (
    <TelaAuth titulo="Confirme que é você" subtitulo={sessao?.email}>
      <div className="flex flex-col gap-4">
        {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
        {temPasskey ? (
          <Botao onClick={comPasskey}>
            <KeyRound className="size-4" aria-hidden /> Usar passkey
          </Botao>
        ) : null}
        {temTotp || recuperacao ? (
          <form onSubmit={enviar} className="flex flex-col gap-4">
            <Campo
              rotulo={recuperacao ? "Código de recuperação" : "Código do aplicativo autenticador"}
              inputMode={recuperacao ? "text" : "numeric"}
              autoComplete="one-time-code"
              pattern={recuperacao ? undefined : "[0-9 ]{6,7}"}
              value={codigo}
              onChange={(e) => setCodigo(e.target.value)}
              required
              autoFocus={!temPasskey}
              placeholder={recuperacao ? "xxxxx-xxxxx" : "123456"}
            />
            <Botao type="submit" variante={temPasskey ? "secundario" : "primario"} carregando={enviando}>
              Confirmar
            </Botao>
          </form>
        ) : null}
        <div className="flex flex-wrap justify-between gap-2 text-sm">
          <button
            type="button"
            className="min-h-11 font-medium text-primaria"
            onClick={() => setRecuperacao((r) => !r)}
          >
            {recuperacao ? "Usar o aplicativo" : "Usar código de recuperação"}
          </button>
          <button type="button" className="min-h-11 text-texto-2 hover:text-texto" onClick={sair}>
            Sair
          </button>
        </div>
      </div>
    </TelaAuth>
  );
}
