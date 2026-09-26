import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Sparkles, Wand2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { Barra } from "../components/extras";
import { useComReautenticacao } from "../components/reautenticacao";
import { Aviso, Botao, Campo, Cartao, Pagina, TituloCartao } from "../components/ui";
import { api, mensagemDe } from "../lib/api";
import { dolares, useConfigIA } from "../lib/dados";

export function ConfiguracaoIA() {
  const cliente = useQueryClient();
  const config = useConfigIA();
  const comReautenticacao = useComReautenticacao();
  const c = config.data;
  const [teto, setTeto] = useState("5");
  useEffect(() => {
    if (c) setTeto(String(c.teto_mensal_microdolares / 1_000_000).replace(".", ","));
  }, [c]);
  const tetoMicro = Math.round(Number(teto.replace(",", ".")) * 1_000_000);
  const tetoValido = Number.isFinite(tetoMicro) && tetoMicro >= 0 && tetoMicro <= 1_000_000_000;

  const salvar = useMutation({
    mutationFn: (ativa: boolean) =>
      comReautenticacao(() =>
        api("PUT", "/api/ia", { ativa, teto_mensal_microdolares: tetoMicro }).then(() => true),
      ),
    onSuccess: () => cliente.invalidateQueries({ queryKey: ["ia"] }),
  });
  const categorizar = useMutation({
    mutationFn: () =>
      api<{ analisadas: number; categorizadas: number; parou?: string }>("POST", "/api/ia/categorizar"),
    onSuccess: () =>
      Promise.all(
        ["ia", "transacoes", "resumo", "indicadores"].map((k) =>
          cliente.invalidateQueries({ queryKey: [k] }),
        ),
      ),
  });

  return (
    <Pagina
      titulo="Inteligência artificial"
      subtitulo="Categorias automáticas e perguntas sobre as suas finanças."
    >
      {config.error ? <Aviso tipo="erro">{mensagemDe(config.error)}</Aviso> : null}
      {c && !c.servidor ? (
        <Aviso tipo="alerta">
          A IA ainda não está configurada neste servidor (falta a chave da API do Claude no Pi).
        </Aviso>
      ) : null}

      <Cartao>
        <TituloCartao>
          <span className="flex items-center gap-2">
            <Sparkles className="size-4 text-destaque" aria-hidden /> Usar IA nas minhas finanças
          </span>
        </TituloCartao>
        <div className="flex flex-col gap-3 text-sm text-texto-2">
          <p>
            Com a IA ligada, o painel manda para o Claude (Anthropic) as{" "}
            <strong className="text-texto">descrições e valores</strong> das transações que precisam de
            categoria e, no chat, os números que as ferramentas calculam. Antes de enviar, tira CPF, CNPJ,
            e-mails, números de conta e o nome de quem mandou ou recebeu Pix. O servidor não guarda as
            conversas.
          </p>
          <p>Só vale para os seus dados; cada pessoa decide por si.</p>
        </div>
        <div className="mt-4 grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
          <Campo
            rotulo="Teto de gasto por mês (US$)"
            inputMode="decimal"
            value={teto}
            onChange={(e) => setTeto(e.target.value)}
            erro={tetoValido ? undefined : "Entre 0 e 1.000"}
            dica="Passou do teto, a IA para até o mês que vem."
          />
          <div className="flex gap-2">
            {c?.ativa ? (
              <>
                <Botao
                  variante="secundario"
                  onClick={() => salvar.mutate(true)}
                  carregando={salvar.isPending}
                  disabled={!tetoValido}
                >
                  Salvar teto
                </Botao>
                <Botao variante="perigo" onClick={() => salvar.mutate(false)} carregando={salvar.isPending}>
                  Desligar
                </Botao>
              </>
            ) : (
              <Botao
                onClick={() => salvar.mutate(true)}
                carregando={salvar.isPending}
                disabled={!tetoValido || !c?.servidor}
              >
                Ligar IA
              </Botao>
            )}
          </div>
        </div>
        {salvar.error ? (
          <div className="mt-3">
            <Aviso tipo="erro">{mensagemDe(salvar.error)}</Aviso>
          </div>
        ) : null}
      </Cartao>

      {c ? (
        <Cartao>
          <TituloCartao>Consumo deste mês</TituloCartao>
          <p className="valor num text-2xl font-bold">
            {dolares(c.uso_mes.custo_microdolares)}{" "}
            <span className="text-sm font-normal text-texto-2">de {dolares(c.teto_mensal_microdolares)}</span>
          </p>
          <Barra
            fracao={
              c.teto_mensal_microdolares ? c.uso_mes.custo_microdolares / c.teto_mensal_microdolares : 0
            }
            cor="var(--destaque)"
            rotulo="Consumo da IA no mês"
          />
          <p className="mt-2 text-xs text-texto-2">
            {c.uso_mes.chamadas} {c.uso_mes.chamadas === 1 ? "chamada" : "chamadas"} ·{" "}
            {c.uso_mes.tokens_entrada.toLocaleString("pt-BR")} tokens enviados ·{" "}
            {c.uso_mes.tokens_saida.toLocaleString("pt-BR")} recebidos
          </p>
        </Cartao>
      ) : null}

      {c?.ativa ? (
        <Cartao>
          <TituloCartao>
            <span className="flex items-center gap-2">
              <Wand2 className="size-4 text-destaque" aria-hidden /> Categorias
            </span>
          </TituloCartao>
          <p className="text-sm text-texto-2">
            A cada hora, o que as suas regras e o histórico não resolveram ganha uma categoria (dá para
            corrigir; a correção vale para as próximas). Se quiser agora:
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-3">
            <Botao onClick={() => categorizar.mutate()} carregando={categorizar.isPending}>
              Categorizar agora
            </Botao>
            <Link to="/perguntar" className="text-sm font-semibold text-primaria">
              Perguntar às minhas finanças
            </Link>
          </div>
          {categorizar.data ? (
            <div className="mt-3">
              <Aviso tipo="sucesso">
                {categorizar.data.categorizadas} de {categorizar.data.analisadas} transações ganharam
                categoria.
                {categorizar.data.parou ? ` ${categorizar.data.parou}.` : ""}
              </Aviso>
            </div>
          ) : null}
          {categorizar.error ? (
            <div className="mt-3">
              <Aviso tipo="erro">{mensagemDe(categorizar.error)}</Aviso>
            </div>
          ) : null}
        </Cartao>
      ) : null}
    </Pagina>
  );
}
