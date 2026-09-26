import {
  Building2,
  CalendarDays,
  ChevronRight,
  CreditCard,
  FileScan,
  FileText,
  Landmark,
  LineChart,
  LogOut,
  MessageCircleQuestion,
  PiggyBank,
  Plug,
  Repeat,
  Scale,
  Shield,
  Sparkles,
  Tags,
  Target,
  Upload,
  UserPlus,
  Users,
} from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { useSair } from "../components/shell";
import { Cartao, Pagina } from "../components/ui";
import { useSessao } from "../lib/sessao";

export function Mais() {
  const { data: sessao } = useSessao();
  const sair = useSair();
  return (
    <Pagina titulo="Mais" subtitulo={sessao?.nome}>
      <Cartao className="p-2 sm:p-2">
        <nav aria-label="Planejamento">
          <ul className="flex flex-col">
            <Item para="/perguntar" icone={<MessageCircleQuestion className="size-5" />}>
              Pergunte às suas finanças
            </Item>
            <Item para="/relatorios" icone={<FileText className="size-5" />}>
              Relatórios do mês e da semana
            </Item>
            <Item para="/projecoes" icone={<LineChart className="size-5" />}>
              Projeções e simulações
            </Item>
            <Item para="/orcamento" icone={<PiggyBank className="size-5" />}>
              Orçamento
            </Item>
            <Item para="/agenda" icone={<CalendarDays className="size-5" />}>
              Agenda e saldo projetado
            </Item>
            <Item para="/cartoes" icone={<CreditCard className="size-5" />}>
              Cartões e parcelas
            </Item>
            <Item para="/recorrencias" icone={<Repeat className="size-5" />}>
              Assinaturas e recorrências
            </Item>
            <Item para="/metas" icone={<Target className="size-5" />}>
              Metas
            </Item>
            <Item para="/patrimonio" icone={<Scale className="size-5" />}>
              Patrimônio, bens e dívidas
            </Item>
          </ul>
        </nav>
      </Cartao>
      <Cartao className="p-2 sm:p-2">
        <nav aria-label="Mais opções">
          <ul className="flex flex-col">
            <Item para="/contas" icone={<Landmark className="size-5" />}>
              Contas e cartões
            </Item>
            <Item para="/open-finance" icone={<Plug className="size-5" />}>
              Open Finance (Meu Pluggy)
            </Item>
            <Item para="/importar" icone={<Upload className="size-5" />}>
              Importar extrato
            </Item>
            <Item para="/documentos" icone={<FileScan className="size-5" />}>
              Ler documento (PDF ou foto)
            </Item>
            <Item para="/categorias" icone={<Tags className="size-5" />}>
              Categorias e regras
            </Item>
            <Item para="/casa" icone={<Building2 className="size-5" />}>
              Casa
            </Item>
            <Item para="/entidades" icone={<Users className="size-5" />}>
              Entidades
            </Item>
            <Item para="/convidar" icone={<UserPlus className="size-5" />}>
              Convidar pessoas
            </Item>
            <Item para="/ia" icone={<Sparkles className="size-5" />}>
              Inteligência artificial
            </Item>
            <Item para="/seguranca" icone={<Shield className="size-5" />}>
              Segurança
            </Item>
          </ul>
        </nav>
      </Cartao>
      <Cartao className="p-2 sm:p-2">
        <button
          type="button"
          onClick={sair}
          className="flex min-h-12 w-full items-center gap-3 rounded-xl px-3 text-left text-saida hover:bg-superficie-2"
        >
          <LogOut className="size-5" aria-hidden /> Sair
        </button>
      </Cartao>
      <p className="text-center text-xs text-texto-2">
        O painel organiza e simula; não substitui contador nem assessor de investimentos.
      </p>
    </Pagina>
  );
}

function Item({ para, icone, children }: { para: string; icone: ReactNode; children: ReactNode }) {
  return (
    <li>
      <Link to={para} className="flex min-h-12 items-center gap-3 rounded-xl px-3 hover:bg-superficie-2">
        <span className="text-texto-2">{icone}</span>
        <span className="flex-1 font-medium">{children}</span>
        <ChevronRight className="size-4 text-texto-2" aria-hidden />
      </Link>
    </li>
  );
}
