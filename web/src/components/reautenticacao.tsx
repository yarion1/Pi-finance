import { createContext, type ReactNode, useCallback, useContext, useRef, useState } from "react";
import { api, ehErro, mensagemDe } from "../lib/api";
import { Aviso, Botao, Campo } from "./ui";

type Pedir = () => Promise<boolean>;
const Contexto = createContext<Pedir>(async () => false);

/**
 * Ações sensíveis (apagar, remover passkey, novos códigos) pedem a senha de novo.
 * O servidor responde "reautenticar"; aqui abrimos o diálogo e repetimos a ação.
 */
export function ProvedorReautenticacao({ children }: { children: ReactNode }) {
  const dialogo = useRef<HTMLDialogElement>(null);
  const resolver = useRef<(ok: boolean) => void>(() => undefined);
  const [senha, setSenha] = useState("");
  const [erro, setErro] = useState("");
  const [enviando, setEnviando] = useState(false);

  const pedir = useCallback<Pedir>(() => {
    setSenha("");
    setErro("");
    dialogo.current?.showModal();
    return new Promise((ok) => {
      resolver.current = ok;
    });
  }, []);

  const fechar = (ok: boolean) => {
    dialogo.current?.close();
    resolver.current(ok);
  };

  const confirmar = async (e: React.FormEvent) => {
    e.preventDefault();
    setEnviando(true);
    try {
      await api("POST", "/api/auth/reautenticar", { senha });
      fechar(true);
    } catch (err) {
      setErro(mensagemDe(err));
    } finally {
      setEnviando(false);
    }
  };

  return (
    <Contexto.Provider value={pedir}>
      {children}
      <dialog
        ref={dialogo}
        onCancel={() => resolver.current(false)}
        aria-labelledby="titulo-reautenticar"
        className="m-auto w-[min(100vw-2rem,24rem)] rounded-cartao border border-borda bg-superficie p-5 text-texto shadow-cartao backdrop:bg-black/60"
      >
        <form onSubmit={confirmar} className="flex flex-col gap-4">
          <div>
            <h2 id="titulo-reautenticar" className="text-lg font-semibold">
              Confirme sua senha
            </h2>
            <p className="mt-1 text-sm text-texto-2">
              Esta ação é sensível. A confirmação vale por 10 minutos.
            </p>
          </div>
          {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
          <Campo
            rotulo="Senha"
            type="password"
            autoComplete="current-password"
            value={senha}
            onChange={(e) => setSenha(e.target.value)}
            required
          />
          <div className="flex justify-end gap-2">
            <Botao variante="fantasma" onClick={() => fechar(false)}>
              Cancelar
            </Botao>
            <Botao type="submit" carregando={enviando}>
              Confirmar
            </Botao>
          </div>
        </form>
      </dialog>
    </Contexto.Provider>
  );
}

/** Executa a ação; se o servidor pedir reautenticação, pede a senha e tenta de novo. */
export function useComReautenticacao() {
  const pedir = useContext(Contexto);
  return useCallback(
    async <T,>(acao: () => Promise<T>): Promise<T | undefined> => {
      try {
        return await acao();
      } catch (e) {
        if (!ehErro(e, "reautenticar")) throw e;
        if (!(await pedir())) return undefined;
        return acao();
      }
    },
    [pedir],
  );
}
