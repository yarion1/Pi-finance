import { useQueryClient } from "@tanstack/react-query";
import { Briefcase, Eye, EyeOff, Home, LineChart, LogOut, Menu, Moon, ReceiptText, Sun } from "lucide-react";
import type { ReactNode } from "react";
import { NavLink, Outlet, useNavigate } from "react-router";
import { api } from "../lib/api";
import { usePrivacidade, useTema } from "../lib/preferencias";
import { chaveSessao, useSessao } from "../lib/sessao";

const itens = [
  { para: "/", rotulo: "Início", icone: Home },
  { para: "/gastos", rotulo: "Gastos", icone: ReceiptText },
  { para: "/investimentos", rotulo: "Investimentos", icone: LineChart },
  { para: "/cnpj", rotulo: "CNPJ", icone: Briefcase },
  { para: "/mais", rotulo: "Mais", icone: Menu },
];

function BotaoIcone({
  rotulo,
  onClick,
  children,
}: {
  rotulo: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={rotulo}
      title={rotulo}
      className="inline-flex size-11 items-center justify-center rounded-xl text-texto-2 transition-colors duration-150 hover:bg-superficie-2 hover:text-texto"
    >
      {children}
    </button>
  );
}

export function Controles() {
  const { tema, alternar: alternarTema } = useTema();
  const { privado, alternar: alternarPrivado } = usePrivacidade();
  return (
    <div className="flex items-center">
      <BotaoIcone rotulo={privado ? "Mostrar valores" : "Esconder valores"} onClick={alternarPrivado}>
        {privado ? <EyeOff className="size-5" /> : <Eye className="size-5" />}
      </BotaoIcone>
      <BotaoIcone rotulo={tema === "escuro" ? "Tema claro" : "Tema escuro"} onClick={alternarTema}>
        {tema === "escuro" ? <Sun className="size-5" /> : <Moon className="size-5" />}
      </BotaoIcone>
    </div>
  );
}

export function useSair() {
  const cliente = useQueryClient();
  const navegar = useNavigate();
  return async () => {
    await api("POST", "/api/auth/sair").catch(() => undefined);
    cliente.clear();
    await cliente.invalidateQueries({ queryKey: chaveSessao });
    navegar("/entrar", { replace: true });
  };
}

export function Shell() {
  const { data: sessao } = useSessao();
  const sair = useSair();
  return (
    <div className="min-h-dvh md:grid md:grid-cols-[232px_1fr]">
      {/* barra lateral no desktop */}
      <aside className="sticky top-0 hidden h-dvh flex-col border-r border-borda bg-superficie px-3 py-5 md:flex">
        <div className="mb-6 flex items-center gap-2.5 px-2">
          <img src="/icone.svg" alt="" className="size-8" />
          <span className="text-lg font-bold tracking-tight">Finanças</span>
        </div>
        <nav aria-label="Principal" className="flex flex-col gap-1">
          {itens.map(({ para, rotulo, icone: Icone }) => (
            <NavLink
              key={para}
              to={para}
              end={para === "/"}
              className={({ isActive }) =>
                `flex min-h-11 items-center gap-3 rounded-xl px-3 text-sm font-medium transition-colors duration-150 ${
                  isActive
                    ? "bg-superficie-2 text-texto"
                    : "text-texto-2 hover:bg-superficie-2 hover:text-texto"
                }`
              }
            >
              <Icone className="size-5" aria-hidden />
              {rotulo}
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto flex flex-col gap-2 border-t border-borda pt-4">
          <p className="truncate px-2 text-sm text-texto-2">{sessao?.nome}</p>
          <div className="flex items-center justify-between">
            <Controles />
            <button
              type="button"
              onClick={sair}
              aria-label="Sair"
              title="Sair"
              className="inline-flex size-11 items-center justify-center rounded-xl text-texto-2 hover:bg-superficie-2 hover:text-saida"
            >
              <LogOut className="size-5" />
            </button>
          </div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-col">
        {/* topo no celular */}
        <header className="sticky top-0 z-10 flex items-center justify-between border-b border-borda bg-fundo/90 px-4 pt-[env(safe-area-inset-top)] backdrop-blur md:hidden">
          <div className="flex items-center gap-2">
            <img src="/icone.svg" alt="" className="size-7" />
            <span className="font-bold tracking-tight">Finanças</span>
          </div>
          <Controles />
        </header>

        <main className="flex-1 px-4 pt-5 pb-28 sm:px-6 md:px-8 md:pt-8 md:pb-10">
          <Outlet />
        </main>

        {/* barra inferior no celular */}
        <nav
          aria-label="Principal"
          className="fixed inset-x-0 bottom-0 z-10 grid grid-cols-5 border-t border-borda bg-superficie/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
        >
          {itens.map(({ para, rotulo, icone: Icone }) => (
            <NavLink
              key={para}
              to={para}
              end={para === "/"}
              className={({ isActive }) =>
                `flex min-h-14 flex-col items-center justify-center gap-0.5 text-[11px] font-medium ${
                  isActive ? "text-primaria" : "text-texto-2"
                }`
              }
            >
              <Icone className="size-5" aria-hidden />
              {rotulo}
            </NavLink>
          ))}
        </nav>
      </div>
    </div>
  );
}
