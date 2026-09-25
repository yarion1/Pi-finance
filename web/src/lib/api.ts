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

type Metodo = "GET" | "POST" | "PATCH" | "DELETE";

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

export function mensagemDe(erro: unknown): string {
  return erro instanceof Error ? erro.message : "Algo deu errado.";
}

export const ehErro = (erro: unknown, codigo: string) => erro instanceof ErroApi && erro.codigo === codigo;
