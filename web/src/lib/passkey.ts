import {
  browserSupportsWebAuthn,
  type PublicKeyCredentialCreationOptionsJSON,
  type PublicKeyCredentialRequestOptionsJSON,
  startAuthentication,
  startRegistration,
} from "@simplewebauthn/browser";
import { api } from "./api";

export const suportaPasskey = () => browserSupportsWebAuthn();

export async function registrarPasskey(nome: string) {
  const opcoes = await api<{ publicKey: PublicKeyCredentialCreationOptionsJSON }>(
    "POST",
    "/api/auth/passkey/registro/iniciar",
  );
  const credencial = await startRegistration({ optionsJSON: opcoes.publicKey });
  return api<{ codigos_recuperacao?: string[] }>("POST", "/api/auth/passkey/registro/concluir", {
    nome,
    credencial,
  });
}

/** Sem sessão: login direto. Com sessão parcial: a passkey confirma o segundo fator. */
export async function entrarComPasskey() {
  const opcoes = await api<{ publicKey: PublicKeyCredentialRequestOptionsJSON }>(
    "POST",
    "/api/auth/passkey/login/iniciar",
  );
  const credencial = await startAuthentication({ optionsJSON: opcoes.publicKey });
  return api("POST", "/api/auth/passkey/login/concluir", { credencial });
}

/** Nome sugerido para a passkey a partir do aparelho. */
export function nomeDoAparelho(): string {
  const ua = navigator.userAgent;
  if (/iPhone/.test(ua)) return "iPhone";
  if (/iPad/.test(ua)) return "iPad";
  if (/Android/.test(ua)) return "Android";
  if (/Mac OS X/.test(ua)) return "Mac";
  if (/Windows/.test(ua)) return "Windows";
  if (/Linux/.test(ua)) return "Linux";
  return "Passkey";
}

export function erroPasskey(e: unknown): string | null {
  // o usuário cancelou o diálogo do navegador: não é erro
  if (e instanceof Error && (e.name === "NotAllowedError" || e.name === "AbortError")) return null;
  return e instanceof Error ? e.message : "Não foi possível usar a passkey.";
}
