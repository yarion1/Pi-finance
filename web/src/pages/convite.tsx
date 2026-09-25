import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router";
import { Aviso, Botao, Esqueleto, TelaAuth } from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { formatarDataHora } from "../lib/formato";
import { useSessao } from "../lib/sessao";
import { nomesPapel } from "../lib/tipos";

type InfoConvite = {
  tipo: "casa" | "conta";
  casa?: string;
  papel?: string;
  convidado_por?: string;
  expira_em: string;
  valido: boolean;
};

export function Convite() {
  const { token = "" } = useParams();
  const { data: sessao } = useSessao();
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const convite = useQuery({
    queryKey: ["convite", token],
    queryFn: () => obter<InfoConvite>(`/api/convites/${token}`),
    retry: false,
  });
  const aceitar = useMutation({
    mutationFn: () => api<{ casa_id: string }>("POST", `/api/convites/${token}/aceitar`),
    onSuccess: async (r) => {
      await cliente.invalidateQueries({ queryKey: ["casas"] });
      navegar(`/casa/${r.casa_id}`, { replace: true });
    },
  });

  if (convite.isPending) {
    return (
      <TelaAuth titulo="Convite">
        <Esqueleto className="h-20 w-full" />
      </TelaAuth>
    );
  }
  if (convite.error || !convite.data?.valido) {
    return (
      <TelaAuth
        titulo="Convite inválido"
        subtitulo="O link expirou, já foi usado ou não existe. Peça um novo a quem convidou."
      >
        <Link to="/" className="text-sm font-semibold text-primaria">
          Ir para o início
        </Link>
      </TelaAuth>
    );
  }
  const c = convite.data;
  const logado = sessao?.autenticado && sessao.mfa_ok;

  if (c.tipo === "conta") {
    return (
      <TelaAuth
        titulo={`${c.convidado_por} te convidou para o Finanças`}
        subtitulo={`Você cria a sua conta, com os seus dados, que ninguém mais vê. Vale até ${formatarDataHora(c.expira_em)}.`}
      >
        {logado ? (
          <Aviso>Você já tem conta e está conectado. Este convite é para quem ainda não tem.</Aviso>
        ) : (
          <Botao onClick={() => navegar(`/cadastro?convite=${encodeURIComponent(token)}`)}>
            Criar minha conta
          </Botao>
        )}
      </TelaAuth>
    );
  }

  return (
    <TelaAuth
      titulo={`Convite para ${c.casa ?? ""}`}
      subtitulo={`Papel: ${nomesPapel[c.papel ?? ""]?.toLowerCase()}. Vale até ${formatarDataHora(c.expira_em)}.`}
    >
      <div className="flex flex-col gap-3">
        {aceitar.error ? <Aviso tipo="erro">{mensagemDe(aceitar.error)}</Aviso> : null}
        {logado ? (
          <Botao onClick={() => aceitar.mutate()} carregando={aceitar.isPending}>
            Entrar na casa como {sessao?.nome}
          </Botao>
        ) : (
          <>
            <Botao onClick={() => navegar(`/cadastro?convite=${encodeURIComponent(token)}`)}>
              Criar minha conta
            </Botao>
            <Botao
              variante="secundario"
              onClick={() => navegar(`/entrar?volta=${encodeURIComponent(`/convite/${token}`)}`)}
            >
              Já tenho conta
            </Botao>
          </>
        )}
      </div>
    </TelaAuth>
  );
}
