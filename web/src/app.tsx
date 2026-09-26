import { Navigate, Outlet, Route, Routes, useLocation } from "react-router";
import { ProvedorReautenticacao } from "./components/reautenticacao";
import { Shell } from "./components/shell";
import { Aviso, Esqueleto } from "./components/ui";
import { mensagemDe } from "./lib/api";
import { useSessao } from "./lib/sessao";
import { Agenda } from "./pages/agenda";
import { Cadastro } from "./pages/cadastro";
import { Cartoes } from "./pages/cartoes";
import { Casas, DetalheCasa } from "./pages/casa";
import { Categorias } from "./pages/categorias";
import { CNPJ } from "./pages/cnpj";
import { Configurar2FA } from "./pages/configurar-2fa";
import { Contas } from "./pages/contas";
import { Convidar } from "./pages/convidar";
import { Convite } from "./pages/convite";
import { Documentos } from "./pages/documentos";
import { EmBreve } from "./pages/em-breve";
import { DetalheEntidade, Entidades } from "./pages/entidades";
import { Entrar } from "./pages/entrar";
import { ConfiguracaoIA } from "./pages/ia";
import { Importar } from "./pages/importar";
import { Inicio } from "./pages/inicio";
import { Investimentos } from "./pages/investimentos";
import { InvestimentosIR } from "./pages/investimentos-ir";
import { Mais } from "./pages/mais";
import { Metas } from "./pages/metas";
import { OpenFinance } from "./pages/open-finance";
import { Orcamento } from "./pages/orcamento";
import { Patrimonio } from "./pages/patrimonio";
import { Perguntar } from "./pages/perguntar";
import { Projecoes } from "./pages/projecoes";
import { Recorrencias } from "./pages/recorrencias";
import { DetalheRelatorio, Relatorios } from "./pages/relatorios";
import { Seguranca } from "./pages/seguranca";
import { Transacoes } from "./pages/transacoes";
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
          <Route path="/gastos" element={<Transacoes />} />
          <Route path="/transacoes" element={<Navigate to="/gastos" replace />} />
          <Route path="/contas" element={<Contas />} />
          <Route path="/importar" element={<Importar />} />
          <Route path="/documentos" element={<Documentos />} />
          <Route path="/categorias" element={<Categorias />} />
          <Route path="/cartoes" element={<Cartoes />} />
          <Route path="/orcamento" element={<Orcamento />} />
          <Route path="/metas" element={<Metas />} />
          <Route path="/patrimonio" element={<Patrimonio />} />
          <Route path="/agenda" element={<Agenda />} />
          <Route path="/recorrencias" element={<Recorrencias />} />
          <Route path="/open-finance" element={<OpenFinance />} />
          <Route path="/convidar" element={<Convidar />} />
          <Route path="/investimentos" element={<Investimentos />} />
          <Route path="/investimentos/ir" element={<InvestimentosIR />} />
          <Route path="/cnpj" element={<CNPJ />} />
          <Route path="/ia" element={<ConfiguracaoIA />} />
          <Route path="/perguntar" element={<Perguntar />} />
          <Route path="/projecoes" element={<Projecoes />} />
          <Route path="/relatorios" element={<Relatorios />} />
          <Route path="/relatorios/:id" element={<DetalheRelatorio />} />
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
