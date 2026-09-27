import { Search } from "lucide-react";
import { type KeyboardEvent, useEffect, useId, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router";
import { usePrivacidade, useTema } from "../lib/preferencias";

type Comando = { id: string; nome: string; grupo: string; palavras?: string; executar: () => void };

/** Minúsculas e sem acento, para buscar "orcamento" e achar "Orçamento". */
const normalizar = (s: string) =>
  s
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLowerCase();

const paginas: [string, string, string?][] = [
  ["/", "Início", "painel resumo"],
  ["/gastos", "Gastos (lista)", "transacoes extrato"],
  ["/gastos/graficos", "Gastos em gráficos", "treemap calendario categorias"],
  ["/gastos/fluxo", "Para onde vai o dinheiro", "sankey cascata fluxo"],
  ["/investimentos", "Investimentos", "carteira acoes rentabilidade"],
  ["/investimentos/ir", "IR dos investimentos", "darf imposto"],
  ["/cnpj", "CNPJ", "mei simples das empresa"],
  ["/orcamento", "Orçamento", "limite"],
  ["/metas", "Metas", "objetivo reserva"],
  ["/patrimonio", "Patrimônio", "bens dividas"],
  ["/agenda", "Agenda", "contas a pagar vencimentos"],
  ["/cartoes", "Cartões e parcelas", "fatura"],
  ["/recorrencias", "Assinaturas e recorrências", "fixas"],
  ["/projecoes", "Projeções e simulações", "monte carlo aposentadoria simular compra"],
  ["/relatorios", "Relatórios", "mes semana resumo"],
  ["/perguntar", "Pergunte às suas finanças", "ia chat"],
  ["/contas", "Contas", "bancos saldo"],
  ["/open-finance", "Open Finance", "pluggy banco conectar"],
  ["/importar", "Importar extrato", "ofx csv"],
  ["/documentos", "Ler documento", "pdf foto holerite nota"],
  ["/categorias", "Categorias e regras", ""],
  ["/seguranca", "Segurança", "senha 2fa passkey"],
  ["/notificacoes", "Notificações", "push telegram avisos alertas"],
  ["/ia", "Inteligência artificial", "configurar teto"],
];

/** Paleta de comandos: Ctrl+K (ou ⌘K) em qualquer tela, ou o botão de busca. */
export function PaletaComandos() {
  const [aberta, setAberta] = useState(false);
  const [texto, setTexto] = useState("");
  const [ativo, setAtivo] = useState(0);
  const dialogo = useRef<HTMLDialogElement>(null);
  const campo = useRef<HTMLInputElement>(null);
  const navegar = useNavigate();
  const { alternar: alternarTema } = useTema();
  const { privado, alternar: alternarPrivado } = usePrivacidade();
  const idLista = useId();

  useEffect(() => {
    const abrir = (e: globalThis.KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setAberta(true);
      }
    };
    const pelaBusca = () => setAberta(true);
    window.addEventListener("keydown", abrir);
    window.addEventListener("abrir-paleta", pelaBusca);
    return () => {
      window.removeEventListener("keydown", abrir);
      window.removeEventListener("abrir-paleta", pelaBusca);
    };
  }, []);

  useEffect(() => {
    const d = dialogo.current;
    if (!d) return;
    if (aberta && !d.open) {
      d.showModal();
      setTexto("");
      setAtivo(0);
      campo.current?.focus();
    }
    if (!aberta && d.open) d.close();
  }, [aberta]);

  const comandos = useMemo<Comando[]>(() => {
    const ir = (para: string) => () => navegar(para);
    const lista: Comando[] = [
      {
        id: "nova",
        nome: "Nova transação",
        grupo: "Ações",
        palavras: "lancar gasto",
        executar: ir("/gastos?nova=1"),
      },
      {
        id: "tema",
        nome: "Trocar tema (claro/escuro)",
        grupo: "Ações",
        palavras: "dark light",
        executar: alternarTema,
      },
      {
        id: "privado",
        nome: privado ? "Mostrar valores" : "Esconder valores (modo privacidade)",
        grupo: "Ações",
        palavras: "borrar privacidade",
        executar: alternarPrivado,
      },
      ...paginas.map(([para, nome, palavras]) => ({
        id: para,
        nome,
        grupo: "Ir para",
        palavras,
        executar: ir(para),
      })),
    ];
    const t = normalizar(texto.trim());
    const filtrados = t
      ? lista.filter((c) =>
          t.split(/\s+/).every((p) => normalizar(`${c.nome} ${c.palavras ?? ""}`).includes(p)),
        )
      : lista;
    if (texto.trim()) {
      const q = encodeURIComponent(texto.trim());
      filtrados.push(
        {
          id: "buscar",
          nome: `Buscar transações: “${texto.trim()}”`,
          grupo: "Buscar",
          executar: ir(`/gastos?busca=${q}`),
        },
        {
          id: "perguntar",
          nome: `Perguntar à IA: “${texto.trim()}”`,
          grupo: "Buscar",
          executar: ir(`/perguntar?q=${q}`),
        },
      );
    }
    return filtrados;
  }, [texto, navegar, alternarTema, alternarPrivado, privado]);

  const executar = (c?: Comando) => {
    if (!c) return;
    setAberta(false);
    c.executar();
  };
  const teclar = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setAtivo((a) => Math.min(a + 1, comandos.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setAtivo((a) => Math.max(a - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      executar(comandos[ativo]);
    }
  };

  return (
    <dialog
      ref={dialogo}
      onClose={() => setAberta(false)}
      aria-label="Paleta de comandos"
      className="mx-auto mt-[12dvh] w-[min(100vw-1.5rem,36rem)] rounded-cartao border border-borda bg-superficie p-0 text-texto shadow-cartao backdrop:bg-black/60"
    >
      <div className="flex items-center gap-2 border-b border-borda px-4">
        <Search className="size-4 shrink-0 text-texto-2" aria-hidden />
        <input
          ref={campo}
          role="combobox"
          aria-expanded="true"
          aria-controls={idLista}
          aria-activedescendant={comandos[ativo] ? `${idLista}-${comandos[ativo].id}` : undefined}
          aria-label="Buscar tela ou ação"
          placeholder="Buscar tela, ação ou transação…"
          value={texto}
          onChange={(e) => {
            setTexto(e.target.value);
            setAtivo(0);
          }}
          onKeyDown={teclar}
          className="min-h-14 flex-1 bg-transparent text-base outline-none"
        />
        <kbd className="hidden rounded border border-borda px-1.5 text-xs text-texto-2 sm:inline">Esc</kbd>
      </div>
      <div id={idLista} role="listbox" aria-label="Resultados" className="max-h-[50dvh] overflow-y-auto p-2">
        {comandos.map((c, i) => (
          <div
            key={c.id}
            tabIndex={-1}
            id={`${idLista}-${c.id}`}
            role="option"
            aria-selected={i === ativo}
            onMouseEnter={() => setAtivo(i)}
            onClick={() => executar(c)}
            onKeyDown={() => undefined}
            className="flex min-h-11 cursor-pointer items-center justify-between gap-3 rounded-xl px-3 text-sm aria-selected:bg-superficie-2"
          >
            <span className="truncate">{c.nome}</span>
            <span className="shrink-0 text-xs text-texto-2">{c.grupo}</span>
          </div>
        ))}
        {!comandos.length ? <p className="px-3 py-4 text-sm text-texto-2">Nada encontrado.</p> : null}
      </div>
    </dialog>
  );
}

/** Abre a paleta (o botão de busca do topo). */
export function abrirPaleta() {
  window.dispatchEvent(new Event("abrir-paleta"));
}
