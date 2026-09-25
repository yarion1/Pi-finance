# Finanças — regras do projeto

Painel de finanças pessoais, investimentos e CNPJ para várias pessoas da mesma casa,
auto-hospedado num Raspberry Pi 5. **`docs/SPEC.md` é a fonte da verdade**; o que
mudou em relação a ela está em `docs/DECISOES.md` (leia as duas antes de começar).

## Como trabalhar

- Siga as fases da seção 12 da SPEC, em ordem. Ao fim de cada fase: testes verdes,
  CHANGELOG, tag `vX.Y.Z`, deploy pelo runner do Pi e conferência do `/api/health`.
- Quando algo da SPEC estiver ambíguo, escolha a opção mais simples, registre em
  `docs/DECISOES.md` e siga.
- Interface em português do Brasil, primeiro no celular. Código, nomes e comentários
  também em português.

## Stack (DECISOES.md D1)

- Backend em **Go** (`cmd/financas`, `internal/...`): um binário com os subcomandos
  `serve`, `worker`, `migrar`, `saude`. `net/http` puro, pgx, goose (migrações em
  `internal/db/migracoes`), River (fila no Postgres).
- Front em **React + Vite + TypeScript + Tailwind** (`web/`), compilado e embutido no
  binário. Gráficos com Apache ECharts (a partir da fase 7).
- PostgreSQL 17. Nada de Redis nem serviços extras.

## Regras que não se negociam

- **Privacidade:** todo dado financeiro é lido e gravado por `db.ComUsuario`, que põe o
  usuário em `app.usuario_id`; as políticas RLS fazem o resto. O app conecta como
  `financas_app` (sem superuser, sem BYPASSRLS, não é dono das tabelas).
  - Tabela nova com `entidade_id` precisa de RLS na mesma migração;
    `TestToda_tabela_tem_RLS` falha se esquecer.
  - Rota GET nova com dados entra em `api.RotasLeitura`; `TestIsolamentoPorRota`
    confere que o usuário B nunca lê nada privado de A.
- **Dinheiro:** centavos inteiros (`bigint` / `int64`) mais o código da moeda. Nunca float.
  Quantidades e cotações em `numeric(24,8)`. Arredondar só na exibição, em pt-BR.
- **Regras fiscais** (tetos, faixas, alíquotas, partilha, IR) ficam em `regras_fiscais`
  com `vigente_de`/`vigente_ate`, nunca no código. Os cálculos recebem a data e usam
  `core.RegraVigente`.
- **Segredos** (credenciais, CPF/CNPJ, segredo TOTP) cifrados com `cripto.Cifrador`
  (AES-256-GCM, chave em `/etc/financas/master.key`), com contexto = nome da coluna.
- **Segurança:** CSP estrita (nada inline, nada de terceiros), cookies `__Host-`,
  `HttpOnly`, `Secure`, `SameSite=Strict`, escrita só com `Origin` igual à `PUBLIC_URL`.
  2FA obrigatório. Ações sensíveis pedem reautenticação. Acesso de fora só pelo túnel
  com Cloudflare Access na frente (DECISOES D11).
- Cálculos puros vão em `internal/core` e são 100 % testados.
- Telas de impostos e investimentos mostram que o painel organiza e simula, e não
  substitui contador nem assessor.

## Comandos

```sh
go test ./...                     # precisa de TEST_DATABASE_URL (Postgres com permissão de criar banco)
gofmt -l cmd internal web && go vet ./...
cd web && npm run lint && npm run typecheck && npm test && npm run build
cd web && npx playwright test     # e2e contra um servidor de pé (docs/DESENVOLVIMENTO.md)
```
