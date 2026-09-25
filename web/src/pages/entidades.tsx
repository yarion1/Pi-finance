import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Briefcase, ChevronRight, Plus, Trash2, User } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useComReautenticacao } from "../components/reautenticacao";
import {
  Aviso,
  Botao,
  Campo,
  CarregandoLista,
  Cartao,
  Etiqueta,
  Pagina,
  Selecao,
  TituloCartao,
  Vazio,
} from "../components/ui";
import { api, mensagemDe, obter } from "../lib/api";
import { mascararDocumento } from "../lib/formato";
import { type Acesso, type Entidade, nomesPapel, nomesRegime } from "../lib/tipos";

export function Entidades() {
  const [criando, setCriando] = useState(false);
  const { data, isPending, error } = useQuery({
    queryKey: ["entidades"],
    queryFn: () => obter<Entidade[]>("/api/entidades"),
  });

  return (
    <Pagina
      titulo="Entidades"
      subtitulo="Quem tem o dinheiro: você como pessoa física e seus CNPJs."
      acao={
        criando ? null : (
          <Botao onClick={() => setCriando(true)}>
            <Plus className="size-4" aria-hidden /> Nova entidade
          </Botao>
        )
      }
    >
      {criando ? (
        <Cartao>
          <TituloCartao>Nova entidade</TituloCartao>
          <FormEntidade aoFechar={() => setCriando(false)} />
        </Cartao>
      ) : null}
      <Cartao>
        {error ? <Aviso tipo="erro">{mensagemDe(error)}</Aviso> : null}
        {isPending ? (
          <CarregandoLista />
        ) : data?.length ? (
          <ul className="flex flex-col divide-y divide-borda">
            {data.map((e) => (
              <li key={e.id}>
                <Link
                  to={`/entidades/${e.id}`}
                  className="flex min-h-14 items-center gap-3 rounded-lg px-1 py-2 hover:bg-superficie-2"
                >
                  <span className={e.tipo === "PF" ? "text-primaria" : "text-imposto"}>
                    {e.tipo === "PF" ? (
                      <User className="size-5" aria-hidden />
                    ) : (
                      <Briefcase className="size-5" aria-hidden />
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">{e.nome}</span>
                    <span className="block text-xs text-texto-2">
                      {e.tipo === "PF" ? "Pessoa física" : e.regime ? nomesRegime[e.regime] : "CNPJ"}
                      {e.documento ? <span className="valor num"> · {e.documento}</span> : null}
                    </span>
                  </span>
                  {e.papel !== "dono" ? <Etiqueta cor="primaria">{nomesPapel[e.papel]}</Etiqueta> : null}
                  <ChevronRight className="size-4 text-texto-2" aria-hidden />
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <Vazio
            icone={<User className="size-8" />}
            titulo="Nenhuma entidade ainda"
            texto="Comece pela sua pessoa física. O MEI ou a empresa entram como entidade CNPJ."
            acao={!criando ? <Botao onClick={() => setCriando(true)}>Criar pessoa física</Botao> : undefined}
          />
        )}
      </Cartao>
    </Pagina>
  );
}

type Dados = {
  tipo: "PF" | "PJ";
  nome: string;
  documento: string;
  regime: string;
  anexo: string;
  cnae: string;
  municipio: string;
  data_abertura: string;
};

function FormEntidade({ inicial, aoFechar }: { inicial?: Entidade; aoFechar: () => void }) {
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const [d, setD] = useState<Dados>({
    tipo: inicial?.tipo ?? "PF",
    nome: inicial?.nome ?? "",
    documento: "",
    regime: inicial?.regime ?? "",
    anexo: inicial?.anexo ?? "",
    cnae: inicial?.cnae ?? "",
    municipio: inicial?.municipio ?? "",
    data_abertura: inicial?.data_abertura ?? "",
  });
  const mudar = (campo: keyof Dados) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setD((v) => ({
      ...v,
      [campo]: campo === "documento" ? mascararDocumento(e.target.value, v.tipo) : e.target.value,
    }));

  const salvar = useMutation({
    mutationFn: () => {
      const pj = d.tipo === "PJ";
      const corpo = {
        tipo: d.tipo,
        nome: d.nome,
        documento: d.documento || null,
        regime: pj ? d.regime || null : null,
        anexo: pj ? d.anexo || null : null,
        cnae: pj ? d.cnae || null : null,
        municipio: d.municipio || null,
        data_abertura: d.data_abertura || null,
      };
      return inicial
        ? api("PATCH", `/api/entidades/${inicial.id}`, corpo)
        : api<{ id: string }>("POST", "/api/entidades", corpo);
    },
    onSuccess: async (r) => {
      await cliente.invalidateQueries({ queryKey: ["entidades"] });
      aoFechar();
      if (!inicial && r && typeof r === "object" && "id" in r) navegar(`/entidades/${r.id}`);
    },
  });

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        salvar.mutate();
      }}
      className="grid gap-4 sm:grid-cols-2"
    >
      {salvar.error ? (
        <div className="sm:col-span-2">
          <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso>
        </div>
      ) : null}
      <Selecao
        rotulo="Tipo"
        value={d.tipo}
        onChange={(e) => setD((v) => ({ ...v, tipo: e.target.value as Dados["tipo"], documento: "" }))}
      >
        <option value="PF">Pessoa física</option>
        <option value="PJ">CNPJ</option>
      </Selecao>
      <Campo rotulo="Nome" value={d.nome} onChange={mudar("nome")} required maxLength={100} />
      <Campo
        rotulo={d.tipo === "PF" ? "CPF (opcional)" : "CNPJ (opcional)"}
        inputMode="numeric"
        value={d.documento}
        onChange={mudar("documento")}
        dica={
          inicial?.documento
            ? `Atual: ${inicial.documento}. Deixe vazio para manter.`
            : "Guardado cifrado; a tela mostra só parte do número."
        }
      />
      <Campo rotulo="Município (opcional)" value={d.municipio} onChange={mudar("municipio")} />
      {d.tipo === "PJ" ? (
        <>
          <Selecao rotulo="Regime" value={d.regime} onChange={mudar("regime")}>
            <option value="">Não informado</option>
            <option value="MEI">MEI</option>
            <option value="SIMPLES_ME">Simples Nacional — ME</option>
            <option value="SIMPLES_EPP">Simples Nacional — EPP</option>
          </Selecao>
          {d.regime.startsWith("SIMPLES") ? (
            <Selecao rotulo="Anexo" value={d.anexo} onChange={mudar("anexo")}>
              <option value="">Não informado</option>
              <option value="III">Anexo III</option>
              <option value="V">Anexo V</option>
            </Selecao>
          ) : null}
          <Campo
            rotulo="CNAE principal (opcional)"
            value={d.cnae}
            onChange={mudar("cnae")}
            placeholder="6201-5/01"
          />
          <Campo
            rotulo="Data de abertura (opcional)"
            type="date"
            value={d.data_abertura}
            onChange={mudar("data_abertura")}
          />
        </>
      ) : null}
      <div className="flex justify-end gap-2 sm:col-span-2">
        <Botao variante="fantasma" onClick={aoFechar}>
          Cancelar
        </Botao>
        <Botao type="submit" carregando={salvar.isPending}>
          Salvar
        </Botao>
      </div>
    </form>
  );
}

export function DetalheEntidade() {
  const { id = "" } = useParams();
  const [editando, setEditando] = useState(false);
  const cliente = useQueryClient();
  const navegar = useNavigate();
  const comReautenticacao = useComReautenticacao();
  const { data, isPending, error } = useQuery({
    queryKey: ["entidades", id],
    queryFn: () => obter<Entidade & { acessos: Acesso[] }>(`/api/entidades/${id}`),
  });
  const [erroApagar, setErroApagar] = useState("");

  if (isPending) return <CarregandoLista />;
  if (error || !data) {
    return (
      <Pagina titulo="Entidade">
        <Aviso tipo="erro">{mensagemDe(error)}</Aviso>
      </Pagina>
    );
  }
  const dono = data.papel === "dono";

  const apagar = async () => {
    if (!confirm(`Apagar "${data.nome}" e todos os dados dela? Não dá para desfazer.`)) return;
    setErroApagar("");
    try {
      const feito = await comReautenticacao(() => api("DELETE", `/api/entidades/${id}`).then(() => true));
      if (feito) {
        await cliente.invalidateQueries({ queryKey: ["entidades"] });
        navegar("/entidades", { replace: true });
      }
    } catch (e) {
      setErroApagar(mensagemDe(e));
    }
  };

  return (
    <Pagina
      titulo={data.nome}
      subtitulo={data.tipo === "PF" ? "Pessoa física" : data.regime ? nomesRegime[data.regime] : "CNPJ"}
      acao={
        dono && !editando ? (
          <Botao variante="secundario" onClick={() => setEditando(true)}>
            Editar
          </Botao>
        ) : null
      }
    >
      {editando ? (
        <Cartao>
          <TituloCartao>Editar</TituloCartao>
          <FormEntidade
            inicial={data}
            aoFechar={() => {
              setEditando(false);
              void cliente.invalidateQueries({ queryKey: ["entidades", id] });
            }}
          />
        </Cartao>
      ) : (
        <Cartao>
          <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
            <Info rotulo={data.tipo === "PF" ? "CPF" : "CNPJ"} valor={data.documento} secreto />
            <Info rotulo="Município" valor={data.municipio} />
            {data.tipo === "PJ" ? (
              <>
                <Info rotulo="Anexo" valor={data.anexo ? `Anexo ${data.anexo}` : null} />
                <Info rotulo="CNAE" valor={data.cnae} />
                <Info rotulo="Abertura" valor={data.data_abertura?.split("-").reverse().join("/") ?? null} />
              </>
            ) : null}
            <Info rotulo="Seu papel" valor={nomesPapel[data.papel] ?? data.papel} />
          </dl>
        </Cartao>
      )}

      <Acessos entidade={data} />

      {dono ? (
        <Cartao>
          <TituloCartao>Zona de perigo</TituloCartao>
          {erroApagar ? <Aviso tipo="erro">{erroApagar}</Aviso> : null}
          <p className="mb-3 text-sm text-texto-2">
            Apaga a entidade com todas as contas, transações e acessos.
          </p>
          <Botao variante="perigo" onClick={apagar}>
            <Trash2 className="size-4" aria-hidden /> Apagar entidade
          </Botao>
        </Cartao>
      ) : null}
    </Pagina>
  );
}

function Info({ rotulo, valor, secreto }: { rotulo: string; valor: string | null; secreto?: boolean }) {
  return (
    <div>
      <dt className="text-texto-2">{rotulo}</dt>
      <dd className={`mt-0.5 font-medium ${secreto ? "valor num" : ""}`}>{valor ?? "—"}</dd>
    </div>
  );
}

function Acessos({ entidade }: { entidade: Entidade & { acessos: Acesso[] } }) {
  const cliente = useQueryClient();
  const [email, setEmail] = useState("");
  const [papel, setPapel] = useState<Acesso["papel"]>(entidade.tipo === "PJ" ? "contador" : "leitor");
  const chave = ["entidades", entidade.id];
  const dar = useMutation({
    mutationFn: () => api("POST", `/api/entidades/${entidade.id}/acessos`, { email, papel }),
    onSuccess: () => {
      setEmail("");
      return cliente.invalidateQueries({ queryKey: chave });
    },
  });
  const remover = useMutation({
    mutationFn: (usuario: string) => api("DELETE", `/api/entidades/${entidade.id}/acessos/${usuario}`),
    onSuccess: () => cliente.invalidateQueries({ queryKey: chave }),
  });

  if (entidade.papel !== "dono") return null;

  return (
    <Cartao>
      <TituloCartao>Quem mais acessa</TituloCartao>
      <p className="mb-3 text-sm text-texto-2">
        Membro gerencia; leitor só vê; contador vê e exporta só o CNPJ. Para compartilhar contas com a casa,
        use a visibilidade de cada conta.
      </p>
      {entidade.acessos.length ? (
        <ul className="mb-4 flex flex-col divide-y divide-borda">
          {entidade.acessos.map((a) => (
            <li key={a.usuario_id} className="flex min-h-12 items-center gap-3">
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium">{a.nome}</span>
                <span className="block truncate text-xs text-texto-2">{a.email}</span>
              </span>
              <Etiqueta>{nomesPapel[a.papel]}</Etiqueta>
              <Botao
                variante="fantasma"
                aria-label={`Remover ${a.nome}`}
                onClick={() => remover.mutate(a.usuario_id)}
              >
                <Trash2 className="size-4" />
              </Botao>
            </li>
          ))}
        </ul>
      ) : null}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          dar.mutate();
        }}
        className="grid gap-3 sm:grid-cols-[1fr_180px_auto] sm:items-end"
      >
        {dar.error ? (
          <div className="sm:col-span-3">
            <Aviso tipo="erro">{mensagemDe(dar.error)}</Aviso>
          </div>
        ) : null}
        <Campo
          rotulo="E-mail da pessoa"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <Selecao rotulo="Papel" value={papel} onChange={(e) => setPapel(e.target.value as Acesso["papel"])}>
          <option value="membro">Membro</option>
          <option value="leitor">Leitor</option>
          {entidade.tipo === "PJ" ? <option value="contador">Contador</option> : null}
        </Selecao>
        <Botao type="submit" carregando={dar.isPending}>
          Dar acesso
        </Botao>
      </form>
    </Cartao>
  );
}
