import { Check, Copy, Download } from "lucide-react";
import QRCode from "qrcode";
import { useEffect, useState } from "react";
import { api, mensagemDe } from "../lib/api";
import { erroPasskey, nomeDoAparelho, registrarPasskey } from "../lib/passkey";
import { Aviso, Botao, Campo } from "./ui";

type AoConcluir = (codigos?: string[]) => void;

/** Ativa (ou troca) o aplicativo autenticador: QR code, segredo e confirmação. */
export function ConfigurarTOTP({
  aoConcluir,
  aoCancelar,
}: {
  aoConcluir: AoConcluir;
  aoCancelar?: () => void;
}) {
  const [dados, setDados] = useState<{ uri: string; segredo: string; qr: string } | null>(null);
  const [codigo, setCodigo] = useState("");
  const [erro, setErro] = useState("");
  const [enviando, setEnviando] = useState(false);

  useEffect(() => {
    let vivo = true;
    api<{ uri: string; segredo: string }>("POST", "/api/auth/totp/iniciar")
      .then(async (d) => {
        const qr = await QRCode.toDataURL(d.uri, { margin: 1, width: 224, errorCorrectionLevel: "M" });
        if (vivo) setDados({ ...d, qr });
      })
      .catch((e) => vivo && setErro(mensagemDe(e)));
    return () => {
      vivo = false;
    };
  }, []);

  const confirmar = async (e: React.FormEvent) => {
    e.preventDefault();
    setErro("");
    setEnviando(true);
    try {
      const r = await api<{ codigos_recuperacao?: string[] }>("POST", "/api/auth/totp/confirmar", { codigo });
      aoConcluir(r.codigos_recuperacao);
    } catch (err) {
      setErro(mensagemDe(err));
      setCodigo("");
    } finally {
      setEnviando(false);
    }
  };

  return (
    <form onSubmit={confirmar} className="flex flex-col gap-4">
      {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
      <ol className="flex list-decimal flex-col gap-1 pl-5 text-sm text-texto-2">
        <li>Abra o aplicativo autenticador (Google Authenticator, 1Password, Aegis…).</li>
        <li>Leia o QR code ou digite o segredo.</li>
        <li>Informe o código de 6 dígitos que aparecer.</li>
      </ol>
      <div className="flex flex-col items-center gap-3 rounded-xl border border-borda bg-white p-3">
        {dados ? (
          <img src={dados.qr} alt="QR code para o aplicativo autenticador" className="size-56" />
        ) : (
          <div className="esqueleto size-56" />
        )}
      </div>
      {dados ? (
        <p className="num break-all text-center font-mono text-sm tracking-wider text-texto-2">
          {dados.segredo.match(/.{1,4}/g)?.join(" ")}
        </p>
      ) : null}
      <Campo
        rotulo="Código de 6 dígitos"
        inputMode="numeric"
        autoComplete="one-time-code"
        pattern="[0-9 ]{6,7}"
        value={codigo}
        onChange={(e) => setCodigo(e.target.value)}
        required
      />
      <div className="flex justify-end gap-2">
        {aoCancelar ? (
          <Botao variante="fantasma" onClick={aoCancelar}>
            Cancelar
          </Botao>
        ) : null}
        <Botao type="submit" carregando={enviando} disabled={!dados}>
          Ativar
        </Botao>
      </div>
    </form>
  );
}

/** Registra uma passkey neste aparelho. */
export function ConfigurarPasskey({
  aoConcluir,
  aoCancelar,
}: {
  aoConcluir: AoConcluir;
  aoCancelar?: () => void;
}) {
  const [nome, setNome] = useState(nomeDoAparelho);
  const [erro, setErro] = useState("");
  const [enviando, setEnviando] = useState(false);

  const registrar = async (e: React.FormEvent) => {
    e.preventDefault();
    setErro("");
    setEnviando(true);
    try {
      const r = await registrarPasskey(nome);
      aoConcluir(r.codigos_recuperacao);
    } catch (err) {
      const m = erroPasskey(err);
      if (m) setErro(m);
    } finally {
      setEnviando(false);
    }
  };

  return (
    <form onSubmit={registrar} className="flex flex-col gap-4">
      {erro ? <Aviso tipo="erro">{erro}</Aviso> : null}
      <p className="text-sm text-texto-2">
        A passkey usa o desbloqueio do aparelho (digital, rosto ou PIN). Não há senha para vazar.
      </p>
      <Campo
        rotulo="Nome da passkey"
        value={nome}
        onChange={(e) => setNome(e.target.value)}
        maxLength={60}
        required
      />
      <div className="flex justify-end gap-2">
        {aoCancelar ? (
          <Botao variante="fantasma" onClick={aoCancelar}>
            Cancelar
          </Botao>
        ) : null}
        <Botao type="submit" carregando={enviando}>
          Criar passkey
        </Botao>
      </div>
    </form>
  );
}

/** Mostra os códigos de recuperação uma única vez. */
export function CodigosRecuperacao({ codigos, aoContinuar }: { codigos: string[]; aoContinuar: () => void }) {
  const [copiado, setCopiado] = useState(false);
  const [guardou, setGuardou] = useState(false);
  const texto = `Finanças — códigos de recuperação\nCada código vale uma vez.\n\n${codigos.join("\n")}\n`;

  const copiar = async () => {
    try {
      await navigator.clipboard.writeText(texto);
      setCopiado(true);
    } catch {
      setCopiado(false);
    }
  };

  const baixar = () => {
    const url = URL.createObjectURL(new Blob([texto], { type: "text/plain" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "financas-codigos-recuperacao.txt";
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="flex flex-col gap-4">
      <Aviso tipo="alerta">
        Guarde estes códigos no gerenciador de senhas. Eles entram no lugar do segundo fator se você perder o
        celular ou a passkey. <strong>Não serão mostrados de novo.</strong>
      </Aviso>
      <ul className="num grid grid-cols-2 gap-2 rounded-xl border border-borda bg-superficie-2 p-3 font-mono text-sm">
        {codigos.map((c) => (
          <li key={c} className="text-center">
            {c}
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap gap-2">
        <Botao variante="secundario" onClick={copiar}>
          {copiado ? <Check className="size-4" /> : <Copy className="size-4" />}{" "}
          {copiado ? "Copiado" : "Copiar"}
        </Botao>
        <Botao variante="secundario" onClick={baixar}>
          <Download className="size-4" /> Baixar .txt
        </Botao>
      </div>
      <label className="flex min-h-11 items-center gap-3 text-sm">
        <input
          type="checkbox"
          className="size-5 accent-[var(--primaria)]"
          checked={guardou}
          onChange={(e) => setGuardou(e.target.checked)}
        />
        Guardei os códigos em lugar seguro
      </label>
      <Botao onClick={aoContinuar} disabled={!guardou}>
        Continuar
      </Botao>
    </div>
  );
}
