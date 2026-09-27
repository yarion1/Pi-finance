import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Users } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { api, mensagemDe, obter } from "../lib/api";
import { formatarMoeda, lerCentavos } from "../lib/formato";
import type { Casa, Membro } from "../lib/tipos";
import { Aviso, Botao, Campo, Selecao } from "./ui";

type Modo = "igual" | "valor" | "percentual";

/** Divide um gasto com gente da casa (50/50, por valor ou por percentual). */
export function DividirComCasa({ transacaoId, total }: { transacaoId: string; total: number }) {
  const cliente = useQueryClient();
  const casas = useQuery({ queryKey: ["casas"], queryFn: () => obter<Casa[]>("/api/casas") });
  const minhas = (casas.data ?? []).filter((c) => c.papel !== "leitor");
  const [aberto, setAberto] = useState(false);
  const [casaId, setCasaId] = useState("");
  useEffect(() => {
    if (!casaId && minhas[0]) setCasaId(minhas[0].id);
  }, [casaId, minhas]);
  const detalhe = useQuery({
    queryKey: ["casas", casaId],
    queryFn: () => obter<Casa & { membros: Membro[] }>(`/api/casas/${casaId}`),
    enabled: aberto && !!casaId,
  });
  const membros = (detalhe.data?.membros ?? []).filter((m) => m.papel !== "leitor");
  const [escolhidos, setEscolhidos] = useState<string[]>([]);
  useEffect(() => {
    if (detalhe.data)
      setEscolhidos(detalhe.data.membros.filter((m) => m.papel !== "leitor").map((m) => m.usuario_id));
  }, [detalhe.data]);
  const [modo, setModo] = useState<Modo>("igual");
  const [partes, setPartes] = useState<Record<string, string>>({});
  const [feito, setFeito] = useState(false);

  const numero = (u: string) =>
    modo === "valor" ? lerCentavos(partes[u] ?? "") : lerPercentual(partes[u] ?? "");
  const soma = escolhidos.reduce((s, u) => s + (numero(u) ?? 0), 0);
  const fecha = modo === "igual" || (modo === "valor" ? soma === total : soma === 10000);

  const dividir = useMutation({
    mutationFn: () =>
      api("POST", `/api/casas/${casaId}/divisoes`, {
        transacao_id: transacaoId,
        modo,
        partes: escolhidos.map((u) =>
          modo === "valor"
            ? { usuario_id: u, valor_centavos: numero(u) }
            : modo === "percentual"
              ? { usuario_id: u, percentual_centesimos: numero(u) }
              : { usuario_id: u },
        ),
      }),
    onSuccess: async () => {
      setFeito(true);
      await cliente.invalidateQueries({ queryKey: ["casa-painel"] });
    },
  });

  if (!minhas.length) return null;
  if (feito) {
    return (
      <Aviso tipo="sucesso">
        Dividida. Veja quem deve quem em <Link to={`/casa/${casaId}`}>Casa</Link>.
      </Aviso>
    );
  }
  if (!aberto) {
    return (
      <Botao variante="secundario" onClick={() => setAberto(true)}>
        <Users className="size-4" aria-hidden /> Dividir com a casa
      </Botao>
    );
  }
  return (
    <fieldset className="flex flex-col gap-3 rounded-xl border border-borda p-3">
      <legend className="px-1 text-sm font-semibold">Dividir {formatarMoeda(total)}</legend>
      {minhas.length > 1 ? (
        <Selecao rotulo="Casa" value={casaId} onChange={(e) => setCasaId(e.target.value)}>
          {minhas.map((c) => (
            <option key={c.id} value={c.id}>
              {c.nome}
            </option>
          ))}
        </Selecao>
      ) : null}
      <Selecao rotulo="Como dividir" value={modo} onChange={(e) => setModo(e.target.value as Modo)}>
        <option value="igual">Partes iguais</option>
        <option value="valor">Por valor</option>
        <option value="percentual">Por percentual</option>
      </Selecao>
      <ul className="flex flex-col gap-2">
        {membros.map((m) => {
          const marcado = escolhidos.includes(m.usuario_id);
          return (
            <li key={m.usuario_id} className="flex min-h-11 items-center gap-3">
              <label className="flex flex-1 items-center gap-3 text-sm">
                <input
                  type="checkbox"
                  className="size-5 accent-[var(--primaria)]"
                  checked={marcado}
                  onChange={(e) =>
                    setEscolhidos((v) =>
                      e.target.checked ? [...v, m.usuario_id] : v.filter((u) => u !== m.usuario_id),
                    )
                  }
                />
                {m.nome}
              </label>
              {marcado && modo !== "igual" ? (
                <Campo
                  rotulo={modo === "valor" ? `Parte de ${m.nome} (R$)` : `Parte de ${m.nome} (%)`}
                  className="w-32 [&>label]:sr-only"
                  inputMode="decimal"
                  value={partes[m.usuario_id] ?? ""}
                  onChange={(e) => setPartes((p) => ({ ...p, [m.usuario_id]: e.target.value }))}
                />
              ) : null}
            </li>
          );
        })}
      </ul>
      {modo !== "igual" ? (
        <p className={`text-xs ${fecha ? "text-texto-2" : "text-saida"}`}>
          {modo === "valor"
            ? `Soma ${formatarMoeda(soma)} de ${formatarMoeda(total)}`
            : `Soma ${(soma / 100).toLocaleString("pt-BR")} % de 100 %`}
        </p>
      ) : null}
      {dividir.error ? <Aviso tipo="erro">{mensagemDe(dividir.error)}</Aviso> : null}
      <div className="flex gap-2">
        <Botao
          onClick={() => dividir.mutate()}
          carregando={dividir.isPending}
          disabled={escolhidos.length < 2 || !fecha}
        >
          Dividir
        </Botao>
        <Botao variante="fantasma" onClick={() => setAberto(false)}>
          Cancelar
        </Botao>
      </div>
    </fieldset>
  );
}

/** "33,33" → 3333 (centésimos de ponto percentual). */
function lerPercentual(texto: string): number | null {
  const t = texto.trim().replace("%", "").replace(",", ".");
  if (!/^\d{1,3}(\.\d{1,2})?$/.test(t)) return null;
  const v = Math.round(Number(t) * 100);
  return v > 0 && v <= 10000 ? v : null;
}
