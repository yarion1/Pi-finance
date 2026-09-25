import { useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Aviso, Botao, Campo, TelaAuth } from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { entrarComPasskey, erroPasskey, suportaPasskey } from "../lib/passkey";
import { chaveSessao } from "../lib/sessao";

export function Entrar() {
  const [email, setEmail] = useState("");
  const [senha, setSenha] = useState("");
  const [erro, setErro] = useState("");
  const [enviando, setEnviando] = useState(false);
  const navegar = useNavigate();
  const [params] = useSearchParams();
  const cliente = useQueryClient();
  const { data: estado } = useQuery({
    queryKey: ["cadastro-aberto"],
    queryFn: () => obter<{ cadastro_aberto: boolean }>("/api/auth/estado"),
  });

  const seguir = async () => {
    await cliente.invalidateQueries({ queryKey: chaveSessao });
    navegar(params.get("volta") ?? "/", { replace: true });
  };

  const enviar = async (e: React.FormEvent) => {
    e.preventDefault();
    setErro("");
    setEnviando(true);
    try {
      await api("POST", "/api/auth/entrar", { email, senha });
      await seguir();
    } catch (err) {
      setErro(mensagemDe(err));
    } finally {
      setEnviando(false);
    }
  };

  const comPasskey = async () => {
    setErro("");
    try {
      await entrarComPasskey();
      await seguir();
    } catch (err) {
      const m = erroPasskey(err);
      if (m) setErro(m);
    }
  };

  return (
    <TelaAuth titulo="Entrar" subtitulo="Acesso só pela rede de casa e pela tailnet.">
      <form onSubmit={enviar} className="flex flex-col gap-4">
        {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
        {estado?.cadastro_aberto ? (
          <Aviso>
            Ainda não há contas.{" "}
            <Link to="/cadastro" className="font-semibold underline">
              Crie a primeira
            </Link>
            .
          </Aviso>
        ) : null}
        <Campo
          rotulo="E-mail"
          type="email"
          autoComplete="username webauthn"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <Campo
          rotulo="Senha"
          type="password"
          autoComplete="current-password"
          value={senha}
          onChange={(e) => setSenha(e.target.value)}
          required
        />
        <Botao type="submit" carregando={enviando}>
          Continuar
        </Botao>
        {suportaPasskey() ? (
          <>
            <div className="flex items-center gap-3 text-xs text-texto-2">
              <span className="h-px flex-1 bg-borda" /> ou <span className="h-px flex-1 bg-borda" />
            </div>
            <Botao variante="secundario" onClick={comPasskey}>
              <KeyRound className="size-4" aria-hidden /> Entrar com passkey
            </Botao>
          </>
        ) : null}
      </form>
    </TelaAuth>
  );
}
