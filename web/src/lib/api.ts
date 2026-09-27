// Cliente da API. Cookies HttpOnly vão sozinhos; o servidor confere a origem.

export class ErroApi extends Error {
  constructor(
    public status: number,
    public codigo: string,
    mensagem: string,
  ) {
    super(mensagem);
  }
}

type Metodo = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

export async function api<T = unknown>(metodo: Metodo, caminho: string, corpo?: unknown): Promise<T> {
  let resposta: Response;
  try {
    resposta = await fetch(caminho, {
      method: metodo,
      credentials: "same-origin",
      headers: corpo === undefined ? {} : { "Content-Type": "application/json" },
      body: corpo === undefined ? undefined : JSON.stringify(corpo),
    });
  } catch {
    throw new ErroApi(0, "rede", "Sem conexão com o servidor.");
  }
  if (resposta.status === 204) return undefined as T;
  const dados = await resposta.json().catch(() => ({}));
  if (!resposta.ok) {
    throw new ErroApi(resposta.status, dados.erro ?? "desconhecido", dados.mensagem ?? "Algo deu errado.");
  }
  return dados as T;
}

export const obter = <T>(caminho: string) => api<T>("GET", caminho);

/** POST que devolve um arquivo (exportação): baixa com o nome que o servidor mandou. */
export async function baixar(caminho: string, corpo: unknown): Promise<true> {
  let resposta: Response;
  try {
    resposta = await fetch(caminho, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(corpo),
    });
  } catch {
    throw new ErroApi(0, "rede", "Sem conexão com o servidor.");
  }
  if (!resposta.ok) {
    const dados = await resposta.json().catch(() => ({}));
    throw new ErroApi(resposta.status, dados.erro ?? "desconhecido", dados.mensagem ?? "Algo deu errado.");
  }
  const nome =
    /filename="([^"]+)"/.exec(resposta.headers.get("Content-Disposition") ?? "")?.[1] ?? "financas";
  const url = URL.createObjectURL(await resposta.blob());
  const a = document.createElement("a");
  a.href = url;
  a.download = nome;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
  return true;
}

export function mensagemDe(erro: unknown): string {
  return erro instanceof Error ? erro.message : "Algo deu errado.";
}

export const ehErro = (erro: unknown, codigo: string) => erro instanceof ErroApi && erro.codigo === codigo;
