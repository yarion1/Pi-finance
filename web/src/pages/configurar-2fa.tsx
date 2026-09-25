import { useQueryClient } from "@tanstack/react-query";
import { KeyRound, Smartphone } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { CodigosRecuperacao, ConfigurarPasskey, ConfigurarTOTP } from "../components/fatores";
import { useSair } from "../components/shell";
import { TelaAuth } from "../components/ui";
import { suportaPasskey } from "../lib/passkey";
import { chaveSessao } from "../lib/sessao";

type Etapa = "escolher" | "totp" | "passkey" | "codigos";

/** Primeiro acesso: o segundo fator é obrigatório antes de ver qualquer dado. */
export function Configurar2FA() {
  const [etapa, setEtapa] = useState<Etapa>("escolher");
  const [codigos, setCodigos] = useState<string[]>([]);
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const sair = useSair();

  const concluido = (c?: string[]) => {
    if (c?.length) {
      setCodigos(c);
      setEtapa("codigos");
    } else {
      void continuar();
    }
  };

  const continuar = async () => {
    await cliente.invalidateQueries({ queryKey: chaveSessao });
    navegar("/", { replace: true });
  };

  const titulos: Record<Etapa, string> = {
    escolher: "Proteja sua conta",
    totp: "Aplicativo autenticador",
    passkey: "Passkey",
    codigos: "Códigos de recuperação",
  };

  return (
    <TelaAuth
      titulo={titulos[etapa]}
      subtitulo={
        etapa === "escolher" ? "O segundo fator é obrigatório. Escolha como confirmar que é você." : undefined
      }
    >
      {etapa === "escolher" ? (
        <div className="flex flex-col gap-3">
          {suportaPasskey() ? (
            <Opcao
              icone={<KeyRound className="size-6" />}
              titulo="Passkey"
              texto="Digital, rosto ou PIN do aparelho. Recomendado."
              onClick={() => setEtapa("passkey")}
            />
          ) : null}
          <Opcao
            icone={<Smartphone className="size-6" />}
            titulo="Aplicativo autenticador"
            texto="Código de 6 dígitos que muda a cada 30 segundos."
            onClick={() => setEtapa("totp")}
          />
          <button
            type="button"
            onClick={sair}
            className="mt-2 min-h-11 self-start text-sm text-texto-2 hover:text-texto"
          >
            Sair
          </button>
        </div>
      ) : null}
      {etapa === "totp" ? (
        <ConfigurarTOTP aoConcluir={concluido} aoCancelar={() => setEtapa("escolher")} />
      ) : null}
      {etapa === "passkey" ? (
        <ConfigurarPasskey aoConcluir={concluido} aoCancelar={() => setEtapa("escolher")} />
      ) : null}
      {etapa === "codigos" ? <CodigosRecuperacao codigos={codigos} aoContinuar={continuar} /> : null}
    </TelaAuth>
  );
}

function Opcao({
  icone,
  titulo,
  texto,
  onClick,
}: {
  icone: React.ReactNode;
  titulo: string;
  texto: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-center gap-4 rounded-cartao border border-borda bg-superficie p-4 text-left transition-colors duration-150 hover:border-primaria"
    >
      <span className="text-primaria">{icone}</span>
      <span>
        <span className="block font-semibold">{titulo}</span>
        <span className="block text-sm text-texto-2">{texto}</span>
      </span>
    </button>
  );
}
