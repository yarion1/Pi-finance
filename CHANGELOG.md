# Changelog

## v0.1.1

- Acesso pelo Cloudflare Tunnel (DECISOES D11): IP real do visitante via `CF-Connecting-IP`,
  aceito só de proxy confiável, para o limite de tentativas por IP funcionar.
- Modo rede de casa (D12): com `PUBLIC_URL` em http, cookies sem `Secure`/`__Host-`, sem
  HSTS e passkeys desligadas; `ESCUTAR` no `.env` escolhe onde a porta 3100 escuta.
- Deploy: a chave mestra só precisa existir para o runner (quem lê é o container).
- Testes: migrações dos testes em série (o papel do app é do servidor inteiro).

## v0.1.0 — Fase 0: fundação

- Backend em Go (um binário: `serve`, `worker`, `migrar`, `saude`) e front em React + Vite
  embutido; ver `docs/DECISOES.md` D1.
- PostgreSQL com RLS por entidade e por casa; o app conecta como `financas_app`, sem
  superuser nem BYPASSRLS. Testes: toda tabela com RLS, isolamento entre usuários no banco
  e **por todas as rotas de leitura**, casa, contador só em PJ, casa sempre com dono.
- Autenticação: argon2id, sessão parcial até o segundo fator, TOTP com anti-replay,
  passkeys (WebAuthn), 10 códigos de recuperação, bloqueio progressivo, limite por IP,
  reautenticação para ações sensíveis, alerta de login em aparelho novo.
- Casas com convite por link (48 h, uso único), papéis dono/membro/leitor; entidades PF e
  CNPJ (MEI, Simples ME/EPP) com CPF/CNPJ validado, cifrado (AES-256-GCM) e mascarado;
  acessos membro/leitor/contador.
- Regras fiscais de 2026 com vigência (teto do MEI, DAS-MEI, Anexos III e V, Fator R…).
- Segurança HTTP: CSP estrita sem inline, cookies `__Host-` `SameSite=Strict`, checagem de
  `Origin` em toda escrita, cabeçalhos de segurança.
- Front mobile-first: tema escuro e claro por tokens, modo privacidade, barra inferior no
  celular, telas de login, 2FA, início, entidades, casa, convite e segurança.
- `/api/health` no formato do PiControl; worker com sinal de vida por minuto.
- Deploy: Docker Compose (Postgres em rede sem saída, limites de memória), backup a cada
  6 h, `deploy.sh` com backup pré-migração e volta automática de imagem e banco;
  workflows de CI (Go, front, e2e com Playwright, imagem) e release para o runner do Pi.
