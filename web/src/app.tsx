import { Navigate, Outlet, Route, Routes, useLocation } from "react-router";
import { ProvedorReautenticacao } from "./components/reautenticacao";
import { Shell } from "./components/shell";
import { Aviso, Esqueleto } from "./components/ui";
import { mensagemDe } from "./lib/api";
import { useSessao } from "./lib/sessao";
import { Cadastro } from "./pages/cadastro";
import { Casas, DetalheCasa } from "./pages/casa";
import { Configurar2FA } from "./pages/configurar-2fa";
import { Convite } from "./pages/convite";
import { EmBreve } from "./pages/em-breve";
import { DetalheEntidade, Entidades } from "./pages/entidades";
import { Entrar } from "./pages/entrar";
import { Inicio } from "./pages/inicio";
import { Mais } from "./pages/mais";
import { Seguranca } from "./pages/seguranca";
import { Verificar } from "./pages/verificar";

function Carregando() {
  return (
    <div className="mx-auto flex max-w-sm flex-col gap-3 px-4 py-20" aria-busy="true">
      <Esqueleto className="h-8 w-40" />
      <Esqueleto className="h-24 w-full" />
    </div>
  );
}

/** Só com login completo (senha + segundo fator). */
function Protegida() {
  const { data, isPending, error } = useSessao();
  const local = useLocation();
  if (isPending) return <Carregando />;
  if (error)
    return (
      <div className="p-4">
        <Aviso tipo="erro">{mensagemDe(error)}</Aviso>
      </div>
    );
  if (!data?.autenticado)
    return <Navigate to={`/entrar?volta=${encodeURIComponent(local.pathname)}`} replace />;
  if (!data.mfa_ok)
    return <Navigate to={data.precisa_configurar_2fa ? "/configurar-2fa" : "/verificar"} replace />;
  return (
    <ProvedorReautenticacao>
      <Outlet />
    </ProvedorReautenticacao>
  );
}

/** Telas do meio do login: só com sessão parcial. */
function Parcial({ exigir }: { exigir: "verificar" | "configurar" }) {
  const { data, isPending } = useSessao();
  if (isPending) return <Carregando />;
  if (!data?.autenticado) return <Navigate to="/entrar" replace />;
  if (data.mfa_ok) return <Navigate to="/" replace />;
  if (exigir === "configurar" && !data.precisa_configurar_2fa) return <Navigate to="/verificar" replace />;
  if (exigir === "verificar" && data.precisa_configurar_2fa) return <Navigate to="/configurar-2fa" replace />;
  return <Outlet />;
}

/** Login e cadastro: quem já entrou vai para o início. */
function Publica() {
  const { data, isPending } = useSessao();
  const local = useLocation();
  if (isPending) return <Carregando />;
  if (data?.autenticado && data.mfa_ok) {
    const volta = new URLSearchParams(local.search).get("volta");
    return <Navigate to={volta?.startsWith("/") ? volta : "/"} replace />;
  }
  return <Outlet />;
}

export function App() {
  return (
    <Routes>
      <Route element={<Publica />}>
        <Route path="/entrar" element={<Entrar />} />
        <Route path="/cadastro" element={<Cadastro />} />
      </Route>
      <Route path="/convite/:token" element={<Convite />} />
      <Route element={<Parcial exigir="verificar" />}>
        <Route path="/verificar" element={<Verificar />} />
      </Route>
      <Route element={<Parcial exigir="configurar" />}>
        <Route path="/configurar-2fa" element={<Configurar2FA />} />
      </Route>
      <Route element={<Protegida />}>
        <Route element={<Shell />}>
          <Route index element={<Inicio />} />
          <Route
            path="/gastos"
            element={<EmBreve titulo="Gastos" pergunta="Com o que gasto mais?" fase="fase 1" />}
          />
          <Route
            path="/investimentos"
            element={
              <EmBreve titulo="Investimentos" pergunta="Minha carteira está indo bem?" fase="fase 4" />
            }
          />
          <Route
            path="/cnpj"
            element={<EmBreve titulo="CNPJ" pergunta="Como está a empresa?" fase="fase 5" />}
          />
          <Route path="/mais" element={<Mais />} />
          <Route path="/casa" element={<Casas />} />
          <Route path="/casa/:id" element={<DetalheCasa />} />
          <Route path="/entidades" element={<Entidades />} />
          <Route path="/entidades/:id" element={<DetalheEntidade />} />
          <Route path="/seguranca" element={<Seguranca />} />
          <Route
            path="*"
            element={
              <EmBreve
                titulo="Página não encontrada"
                pergunta="Este endereço não existe."
                fase="próxima versão"
              />
            }
          />
        </Route>
      </Route>
    </Routes>
  );
}
