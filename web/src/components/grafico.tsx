import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { type CoresTema, coresTema } from "../lib/graficos";
import { Esqueleto } from "./ui";

// biome-ignore lint/suspicious/noExplicitAny: a opção do ECharts é um objeto grande e dinâmico
type Opcao = Record<string, any>;
type Motor = typeof import("../lib/graficos-motor");

let motor: Promise<Motor> | null = null;
const carregarMotor = () => {
  motor ??= import("../lib/graficos-motor");
  return motor;
};

/** Cores do tema que mudam com o tema e o modo privacidade (atributos do <html>). */
export function useCoresTema(): CoresTema {
  const [cores, setCores] = useState(coresTema);
  useEffect(() => {
    const obs = new MutationObserver(() => setCores(coresTema()));
    obs.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme", "data-privado"],
    });
    return () => obs.disconnect();
  }, []);
  return cores;
}

/**
 * Gráfico do ECharts. `montar` recebe as cores do tema e devolve a opção; `aoClicar`
 * recebe o dado do pedaço clicado. Sempre com rótulo acessível e, quando houver, a
 * tabela com os mesmos números.
 */
export function Grafico({
  montar,
  rotulo,
  altura = "h-64",
  aoClicar,
  tabela,
}: {
  montar: (c: CoresTema) => Opcao;
  rotulo: string;
  altura?: string;
  // biome-ignore lint/suspicious/noExplicitAny: parâmetros do evento de clique do ECharts
  aoClicar?: (p: any) => void;
  tabela?: ReactNode;
}) {
  const caixa = useRef<HTMLDivElement>(null);
  const [carregado, setCarregado] = useState(false);
  const cores = useCoresTema();
  // biome-ignore lint/suspicious/noExplicitAny: instância do ECharts
  const instancia = useRef<any>(null);
  const clique = useRef(aoClicar);
  clique.current = aoClicar;

  useEffect(() => {
    let vivo = true;
    let obs: ResizeObserver | null = null;
    carregarMotor().then(({ echarts }) => {
      if (!vivo || !caixa.current) return;
      const g = echarts.init(caixa.current, null, { renderer: "canvas", locale: "PT-br" });
      instancia.current = g;
      g.on("click", (p: unknown) => clique.current?.(p));
      obs = new ResizeObserver(() => g.resize());
      obs.observe(caixa.current);
      setCarregado(true);
    });
    return () => {
      vivo = false;
      obs?.disconnect();
      instancia.current?.dispose();
      instancia.current = null;
    };
  }, []);

  useEffect(() => {
    if (!carregado || !instancia.current) return;
    const opcao = montar(cores);
    instancia.current.setOption(
      {
        animationDuration: 250,
        aria: { enabled: true, label: { description: rotulo } },
        textStyle: { fontFamily: "Inter Variable, Inter, system-ui, sans-serif", color: cores.texto2 },
        ...opcao,
        tooltip: opcao.tooltip && {
          backgroundColor: cores.superficie,
          borderColor: cores.borda,
          textStyle: { color: cores.texto, fontSize: 12 },
          confine: true,
          ...opcao.tooltip,
        },
      },
      true,
    );
  }, [carregado, montar, cores, rotulo]);

  return (
    <figure className="m-0">
      <div className="relative">
        <div ref={caixa} className={`${altura} w-full`} role="img" aria-label={rotulo} />
        {!carregado ? <Esqueleto className={`absolute inset-0 ${altura} w-full`} /> : null}
      </div>
      {tabela ? (
        <details className="mt-2 text-sm">
          <summary className="min-h-11 cursor-pointer py-2 text-xs font-semibold text-texto-2">
            Ver em tabela
          </summary>
          <div className="overflow-x-auto">{tabela}</div>
        </details>
      ) : null}
    </figure>
  );
}
