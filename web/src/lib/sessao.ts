import { useQuery } from "@tanstack/react-query";
import { obter } from "./api";

export type Sessao = {
  autenticado: boolean;
  usuario_id?: string;
  nome?: string;
  email?: string;
  mfa_ok: boolean;
  tem_totp: boolean;
  passkeys: number;
  precisa_configurar_2fa: boolean;
  reautenticada: boolean;
};

export const chaveSessao = ["sessao"] as const;

export function useSessao() {
  return useQuery({
    queryKey: chaveSessao,
    queryFn: () => obter<Sessao>("/api/auth/sessao"),
    staleTime: 30_000,
  });
}
