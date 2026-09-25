import { useQuery } from "@tanstack/react-query";
import { Building2, CheckCircle2, Circle, Landmark, ShieldAlert, Tags, Target, Users } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { Cartao, Esqueleto, Etiqueta, Pagina, TituloCartao, Vazio } from "../components/ui";
import { obter } from "../lib/api";
import { formatarDataHora } from "../lib/formato";
import { useSessao } from "../lib/sessao";
import type { Alerta, Casa, Entidade } from "../lib/tipos";

function saudacao() {
  const h = Number(
    new Intl.DateTimeFormat("pt-BR", {
      hour: "numeric",
      hourCycle: "h23",
      timeZone: "America/Sao_Paulo",
    }).format(new Date()),
  );
  return h < 12 ? "Bom dia" : h < 18 ? "Boa tarde" : "Boa noite";
}

export function Inicio() {
  const { data: sessao } = useSessao();
  const entidades = useQuery({ queryKey: ["entidades"], queryFn: () => obter<Entidade[]>("/api/entidades") });
  const casas = useQuery({ queryKey: ["casas"], queryFn: () => obter<Casa[]>("/api/casas") });
  const alertas = useQuery({ queryKey: ["alertas"], queryFn: () => obter<Alerta[]>("/api/alertas") });

  const temPF = entidades.data?.some((e) => e.tipo === "PF" && e.papel === "dono");
  const primeiroNome = sessao?.nome?.split(" ")[0];

  return (
    <Pagina titulo={`${saudacao()}, ${primeiroNome ?? ""}`} subtitulo="Como estou este mês?">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Indicador titulo="Patrimônio líquido" />
        <Indicador titulo="Gasto do mês × orçamento" />
        <Indicador titulo="Saldo projetado em 30 dias" />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Cartao>
          <TituloCartao>Primeiros passos</TituloCartao>
          <ol className="flex flex-col gap-1">
            <Passo feito={!!temPF} para="/entidades" icone={<Users className="size-4" />}>
              Criar sua entidade pessoa física
            </Passo>
            <Passo feito={false} icone={<Landmark className="size-4" />} fase="fase 3">
              Conectar o primeiro banco
            </Passo>
            <Passo feito={false} icone={<Tags className="size-4" />} fase="fase 1">
              Revisar categorias
            </Passo>
            <Passo feito={false} icone={<Target className="size-4" />} fase="fase 2">
              Definir uma meta
            </Passo>
          </ol>
        </Cartao>

        <Cartao>
          <TituloCartao>Alertas</TituloCartao>
          {alertas.isPending ? (
            <Esqueleto className="h-12 w-full" />
          ) : alertas.data?.length ? (
            <ul className="flex flex-col divide-y divide-borda">
              {alertas.data.slice(0, 5).map((a) => (
                <li key={a.id} className="flex items-start gap-3 py-2.5">
                  <ShieldAlert className="mt-0.5 size-5 shrink-0 text-alerta" aria-hidden />
                  <div className="min-w-0">
                    <p className="text-sm font-medium">
                      {a.tipo === "login_aparelho_novo" ? "Login em aparelho novo" : a.tipo}
                    </p>
                    <p className="truncate text-xs text-texto-2">
                      {formatarDataHora(a.criado_em)}
                      {typeof a.dados.ip === "string" ? ` · ${a.dados.ip}` : ""}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          ) : (
            <Vazio
              titulo="Nenhum alerta"
              texto="Gastos fora do padrão, cobranças duplicadas e logins novos aparecem aqui."
            />
          )}
        </Cartao>
      </div>

      <Cartao>
        <TituloCartao
          acao={
            <Link to="/casa" className="text-sm font-semibold text-primaria">
              Abrir
            </Link>
          }
        >
          Casa
        </TituloCartao>
        {casas.isPending ? (
          <Esqueleto className="h-10 w-full" />
        ) : casas.data?.length ? (
          <ul className="flex flex-wrap gap-2">
            {casas.data.map((c) => (
              <li key={c.id}>
                <Link
                  to={`/casa/${c.id}`}
                  className="inline-flex min-h-11 items-center gap-2 rounded-xl border border-borda px-3 text-sm hover:border-primaria"
                >
                  <Building2 className="size-4 text-texto-2" aria-hidden /> {c.nome}
                  <Etiqueta>{c.papel}</Etiqueta>
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <Vazio
            titulo="Você ainda não está em uma casa"
            texto="Crie uma casa para compartilhar contas e metas com quem mora com você."
            acao={
              <Link to="/casa" className="text-sm font-semibold text-primaria">
                Criar casa
              </Link>
            }
          />
        )}
      </Cartao>
    </Pagina>
  );
}

// Sem dados ainda (contas chegam na fase 1): o traço fica neutro, sem cor de significado.
function Indicador({ titulo }: { titulo: string }) {
  return (
    <Cartao>
      <p className="text-sm text-texto-2">{titulo}</p>
      <p className="num mt-2 text-3xl font-bold tracking-tight text-texto-2">—</p>
      <p className="mt-2 text-xs text-texto-2">Conecte um banco ou importe um extrato para ver isto.</p>
    </Cartao>
  );
}

function Passo({
  feito,
  para,
  icone,
  fase,
  children,
}: {
  feito: boolean;
  para?: string;
  icone: ReactNode;
  fase?: string;
  children: ReactNode;
}) {
  const conteudo = (
    <span className="flex min-h-11 items-center gap-3">
      {feito ? (
        <CheckCircle2 className="size-5 text-entrada" aria-label="feito" />
      ) : (
        <Circle className="size-5 text-texto-2" aria-label="pendente" />
      )}
      <span className="text-texto-2">{icone}</span>
      <span className={feito ? "text-texto-2 line-through" : ""}>{children}</span>
      {fase ? <Etiqueta>{fase}</Etiqueta> : null}
    </span>
  );
  return (
    <li>
      {para && !feito ? (
        <Link to={para} className="block rounded-lg hover:bg-superficie-2">
          {conteudo}
        </Link>
      ) : (
        conteudo
      )}
    </li>
  );
}
