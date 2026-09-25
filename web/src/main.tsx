import "@fontsource-variable/inter";
import "./styles.css";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { App } from "./app";
import { ErroApi } from "./lib/api";
import { aplicarPreferenciasSalvas } from "./lib/preferencias";
import { chaveSessao } from "./lib/sessao";

aplicarPreferenciasSalvas();

const cliente: QueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (tentativas, erro) => !(erro instanceof ErroApi && erro.status < 500) && tentativas < 2,
      refetchOnWindowFocus: true,
    },
  },
});

// sessão caiu ou falta o segundo fator: a sessão é relida e as guardas redirecionam
cliente.getQueryCache().subscribe((evento) => {
  const erro = evento.query.state.error;
  if (
    evento.type === "updated" &&
    erro instanceof ErroApi &&
    (erro.codigo === "sessao" || erro.codigo === "segundo_fator")
  ) {
    if (evento.query.queryKey[0] !== chaveSessao[0])
      void cliente.invalidateQueries({ queryKey: chaveSessao });
  }
});

const raiz = document.getElementById("raiz");
if (raiz) {
  createRoot(raiz).render(
    <StrictMode>
      <QueryClientProvider client={cliente}>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </QueryClientProvider>
    </StrictMode>,
  );
}
