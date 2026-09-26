# Checklist de segurança

Revisada antes de abrir o painel pela internet (túnel da Cloudflare, v0.5.2). Cada item
diz onde está a proteção no código e como é testada. Rever a cada fase nova.

| # | Ponto | Como está | Onde / teste |
| --- | --- | --- | --- |
| 1 | `.env` exposto | Segredos só em `/etc/financas/.env` (root:docker, 640) e `/etc/financas/master.key` (0600); `.env*` no `.gitignore`; o front não lê variável de ambiente nenhuma (nada de `VITE_*`); o servidor só entrega o `web/dist` embutido. | `deploy/env.exemplo`, `.gitignore` |
| 2 | Sem validação no front | Formulários validam antes de enviar (datas, valores, campos obrigatórios), mas a regra que vale é a do servidor. | `web/src/lib/formato.ts` (testes) |
| 3 | Sem validação no back | Todo handler valida tipo, faixa e tamanho; corpo JSON limitado; campos desconhecidos recusados. | `lerJSON`, `limitarCorpo`, testes `internal/api` |
| 4 | SQL injection | Só consultas parametrizadas (pgx `$1`); o único nome de tabela montado vem de uma lista fixa no código. | `grep Sprintf` nas consultas |
| 5 | Autenticação fraca | Senha de 12 a 128 caracteres em argon2id; **2FA obrigatório** (TOTP ou passkey); reautenticação nas ações sensíveis; cadastro só por convite. | `internal/auth`, e2e fase 0 |
| 6 | IDOR | Todo dado passa pelo RLS do Postgres com o usuário da sessão (`db.ComUsuario`); o app conecta sem BYPASSRLS. Toda rota GET com dados entra no teste de isolamento. | `TestIsolamentoPorRota`, `TestToda_tabela_tem_RLS` |
| 7 | Senha direto no banco | argon2id (64 MiB, sal aleatório); tokens de sessão, convite e recuperação guardados só como hash; credenciais do Pluggy, CPF/CNPJ e segredo TOTP cifrados (AES-256-GCM). | `auth/senha.go`, `cripto` |
| 8 | Força bruta | Limite por IP nas rotas de login e bloqueio progressivo por conta; no máximo 3 hashes de senha ao mesmo tempo (a memória não estoura com rajada). | `TestBloqueioProgressivo`, `limitado`, `vagasArgon` |
| 9 | Envio duplo | Botões ficam desabilitados enquanto a requisição corre; importações deduplicam por chave; a sincronização tem trava de 10 s. | `Botao carregando`, dedup |
| 10 | CSRF | Cookie `SameSite=Strict` e escrita só com `Origin` igual à `PUBLIC_URL`. | `origemConfere` (testes) |
| 11 | Upload sem validação | Arquivos em base64 com teto de 8 MB, lidos só como OFX/CSV/XLSX/XML; o `.xlsx` abre com limite de descompactação (contra zip bomb); o XML da NFS-e não resolve entidade externa (XXE) e não é guardado; nada é salvo em disco nem servido de volta. A planilha do contador escapa texto que começaria uma fórmula. | `decodificarArquivo`, `importadores`, `TestLerNFSe`, `celulaSegura` |
| 12 | Erro revelando informação | Erro inesperado vira "erro interno" (o detalhe só no log); login não diz se o e-mail existe; tokens de convite não aparecem no log. | `falhar`, `rotaParaLog` |
| 13 | Dependências vulneráveis | CI roda `govulncheck` e `npm audit --audit-level=high` a cada push. | `.github/workflows/ci.yml` |
| 14 | Tokens | Sessão, convite e recuperação com 256 bits aleatórios, hash no banco, expiração curta (sessão parcial minutos; convite 48 h, uso único) e troca do token ao completar o 2FA. | `internal/auth` |
| 15 | Rate limit | Por IP: 600 req/min na API toda e 120 escritas/min; mais apertado no login e nos convites. | `TestLimiteGeralDaAPI` |
| 16 | Dados sensíveis expostos | A API nunca devolve segredo (do Client ID só os 4 últimos); CPF/CNPJ mascarados; nada de dado financeiro no log. | teste de isolamento procura os segredos nas respostas |
| 17 | SSRF | O servidor só chama endereços fixos (Pluggy, brapi, CoinGecko, Banco Central); nenhuma URL vem do usuário. | `internal/pluggy`, `internal/cotacoes` |
| 18 | Cookies inseguros | `__Host-`, `HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/`; HSTS com HTTPS. | `gravarCookie` |
| 19 | CORS | Nenhum cabeçalho `Access-Control-*`: só a própria origem usa a API; CSP estrita sem terceiros, `frame-ancestors 'none'`. | `cabecalhos` |

Na frente de tudo, o **Cloudflare Access** só deixa chegar ao painel quem está na lista de
e-mails (DECISOES D11 e D29).
