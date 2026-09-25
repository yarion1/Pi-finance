# Decisões

Escolhas feitas onde a especificação (docs/SPEC.md) estava ambígua ou foi alterada.
A mais recente vale.

## D1 — Backend em Go, front em React + Vite (fase 0)

A SPEC (§8) pedia TypeScript de ponta a ponta (Next.js, Drizzle, Better Auth, pg-boss).
**Pedido do Pablo: backend em Go, front a critério.** Ficou assim:

| SPEC | Agora | Por quê |
| --- | --- | --- |
| Next.js (telas + API) | Go `net/http` (API) + React/Vite (SPA estática embutida no binário) | um binário só; menos memória no Pi; CSP estrita sem nonce |
| Drizzle ORM | pgx + SQL escrito à mão; migrações com goose | RLS e funções SQL ficam explícitas |
| Better Auth | autenticação própria: argon2id, sessões no banco, TOTP (pquerna/otp), passkeys (go-webauthn) | não existe Better Auth para Go |
| pg-boss | River (fila e cron no Postgres) | mesmo papel, em Go |
| Zod | validação no handler (Go) e Zod disponível no front | — |
| Vitest e Playwright | `go test` + Vitest (front) + Playwright (e2e) | — |
| imagens financas-web e financas-worker | uma imagem `financas:vX.Y.Z` com subcomandos `serve`, `worker`, `migrar` | um build por deploy |

Organização: `cmd/financas`, `internal/{core,db,auth,api,worker,cripto,config}`, `web/`,
`deploy/`. `internal/core` é o antigo `packages/core`; `internal/db/migracoes` + as
sementes fiscais são o `packages/db`. `packages/importers` vira `internal/importadores`
na fase 1.

## D2 — Modelo de sessão e 2FA

- Senha certa gera **sessão parcial** (30 min) que só serve para confirmar o segundo fator
  ou, se o usuário ainda não tem nenhum, para configurar o primeiro. Nada de dados.
- Segundo fator: TOTP, passkey ou código de recuperação (10 códigos, uso único).
- Login só com passkey já é completo (a passkey é multifator).
- Quem só tem passkey e entra com senha confirma com a passkey na tela seguinte.
- Sessão completa: **7 dias no celular, 12 h no navegador**, decidido pelo User-Agent,
  sem renovação automática. O token é trocado ao completar o login.
- Reautenticação (senha) vale 10 min: apagar entidade, remover passkey, gerar novos
  códigos, trocar o aplicativo autenticador.

## D3 — Cadastro

O primeiro usuário se cadastra livremente e vira dono da primeira casa que criar.
Depois disso, cadastro **só com convite** (link de 48 h, uso único). Convite de casa tem
papel membro ou leitor; dono se promove depois.

## D4 — Bloqueio progressivo

Por conta: a partir da 5ª falha seguida (senha, TOTP ou reautenticação), 15 min de
bloqueio, dobrando a cada nova falha até 24 h. Mais um limite de 10 tentativas por minuto
por IP nas rotas de login. O aviso de **login em aparelho novo** vira um registro em
`alertas` já na fase 0; o envio por Telegram e e-mail entra com as notificações (fase 7).

## D5 — Compartilhamento com a casa

- A conta aponta para uma casa (`contas.casa_id`) quando a visibilidade não é privada.
- Um membro da casa vê a conta **só se o dono da conta também estiver na casa**: quem sai
  leva as contas compartilhadas junto, sem precisar mexer nelas.
- "Só saldo": a casa vê a conta (e o saldo, a partir da fase 1), nunca as transações.
- Papel na entidade (membro, leitor, contador) é separado do papel na casa. Contador só
  em entidade PJ (garantido por gatilho). Acesso a entidade é dado por e-mail de quem já
  tem conta.

## D6 — Tabelas sem RLS

Autenticação (`usuarios`, `sessoes`, `dois_fatores`, `codigos_recuperacao`, `passkeys`,
`desafios_webauthn`), `sinais_vida` e as tabelas do River/goose. Só o código de
autenticação e o worker tocam nelas. Todas as outras têm RLS, conferido por teste.

## D7 — Chave mestra e permissões no Pi

A imagem roda sem root (uid 65532). Para ela ler a chave mestra mantendo `0600`, o dono
do arquivo é esse uid (não root, como dizia a SPEC):
`sudo chown 65532:65532 /etc/financas/master.key && sudo chmod 600 /etc/financas/master.key`.
O `.env` fica `0640` com grupo `docker`, para o runner ler.

## D8 — Backup na fase 0

Já existe um serviço de backup simples (`pg_dump` a cada 6 h, guarda 28) e o deploy faz
dump antes de migrar. Restic, cópia na nuvem e teste de restore semanal são a fase 8.
O `/api/health` diz `backup=pendente` até o primeiro dump.

## D9 — /api/health

Responde 200 quando banco e worker estão de pé; 503 caso contrário (é o que o deploy
confere). `pluggy` é `ok` enquanto não há conexões (fase 3). `FORCAR_FALHA_HEALTH=1`
força 503 — é assim que o deploy quebrado de propósito é testado.

## D10 — Regras fiscais semeadas

Entraram só os valores citados na SPEC §4 (teto do MEI, DAS-MEI 2026, salário mínimo,
teto do Simples, sublimite, Fator R, faixas dos Anexos III e V, lucro presumido do MEI,
limite de dividendos). Partilha, tabela do IRPF e IR regressivo entram nas fases 4 e 5,
com fonte, para não inventar números.

## D11 — Acesso pelo Cloudflare Tunnel em vez do Tailscale

A SPEC (§10, §11) pedia só LAN e tailnet, sem Cloudflare Tunnel. **Pedido do Pablo:
publicar pelo Cloudflare Tunnel, como o site dos Pebas**, para ninguém precisar instalar
app. Condições para compensar a exposição na internet:

- **Cloudflare Access obrigatório** na frente do hostname (e-mails autorizados, código por
  e-mail). O login do Finanças (senha + 2FA obrigatório, bloqueio progressivo) fica atrás.
- `PUBLIC_URL` passa a ser o hostname público (`https://financas.<domínio>`): passkeys e a
  checagem de `Origin` usam uma origem só, então o endereço do Tailscale deixa de valer.
- O IP do visitante vem de `CF-Connecting-IP`, aceito só de proxy confiável (cloudflared).
- Custo aceito: a Cloudflare termina o TLS e vê o tráfego na borda dela.
