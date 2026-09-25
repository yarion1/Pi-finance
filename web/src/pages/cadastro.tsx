import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Aviso, Botao, Campo, TelaAuth } from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { chaveSessao } from "../lib/sessao";

export function Cadastro() {
  const [params] = useSearchParams();
  const convite = params.get("convite") ?? "";
  const [nome, setNome] = useState("");
  const [email, setEmail] = useState("");
  const [senha, setSenha] = useState("");
  const [erro, setErro] = useState("");
  const [enviando, setEnviando] = useState(false);
  const navegar = useNavigate();
  const cliente = useQueryClient();
  const { data: estado } = useQuery({
    queryKey: ["cadastro-aberto"],
    queryFn: () => obter<{ cadastro_aberto: boolean }>("/api/auth/estado"),
  });

  const fechado = estado && !estado.cadastro_aberto && !convite;

  const enviar = async (e: React.FormEvent) => {
    e.preventDefault();
    setErro("");
    setEnviando(true);
    try {
      await api("POST", "/api/auth/cadastro", { nome, email, senha, convite });
      await cliente.invalidateQueries({ queryKey: chaveSessao });
      navegar("/configurar-2fa", { replace: true });
    } catch (err) {
      setErro(mensagemDe(err));
    } finally {
      setEnviando(false);
    }
  };

  if (fechado) {
    return (
      <TelaAuth titulo="Cadastro por convite" subtitulo="Peça a quem administra a casa um link de convite.">
        <Link to="/entrar" className="text-sm font-semibold text-primaria">
          Voltar para o login
        </Link>
      </TelaAuth>
    );
  }

  return (
    <TelaAuth
      titulo={convite ? "Aceitar convite" : "Criar a primeira conta"}
      subtitulo={
        convite
          ? "Crie sua conta; você entra na casa em seguida."
          : "Esta conta será a dona da primeira casa."
      }
    >
      <form onSubmit={enviar} className="flex flex-col gap-4">
        {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
        <Campo
          rotulo="Nome"
          autoComplete="name"
          value={nome}
          onChange={(e) => setNome(e.target.value)}
          required
          maxLength={100}
        />
        <Campo
          rotulo="E-mail"
          type="email"
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <Campo
          rotulo="Senha"
          type="password"
          autoComplete="new-password"
          value={senha}
          onChange={(e) => setSenha(e.target.value)}
          required
          minLength={12}
          maxLength={128}
          dica="Pelo menos 12 caracteres. Uma frase longa é mais forte e mais fácil de lembrar."
        />
        <Botao type="submit" carregando={enviando}>
          Criar conta
        </Botao>
        <p className="text-sm text-texto-2">
          Já tem conta?{" "}
          <Link to="/entrar" className="font-semibold text-primaria">
            Entrar
          </Link>
        </p>
      </form>
    </TelaAuth>
  );
}
