# Desenvolvimento local

Requisitos: Go 1.26+, Node 22, PostgreSQL 16+.

```sh
# banco
createdb financas
head -c 32 /dev/urandom | base64 > /tmp/master.key && chmod 600 /tmp/master.key

# migrações (papel dono) — cria o papel financas_app com a senha dada
DATABASE_URL_MIGRACAO=postgres://postgres@127.0.0.1:5432/financas?sslmode=disable \
APP_DB_PASSWORD=dev go run ./cmd/financas migrar

# API + worker (o front em modo dev fala com a API pelo proxy do Vite)
export DATABASE_URL=postgres://financas_app:dev@127.0.0.1:5432/financas?sslmode=disable
export MASTER_KEY_FILE=/tmp/master.key PUBLIC_URL=http://localhost:5173 ENDERECO=127.0.0.1:3100
go run ./cmd/financas serve &
go run ./cmd/financas worker &

cd web && npm install && npm run dev   # http://localhost:5173
```

Passkeys funcionam em `localhost` sem HTTPS. Os cookies são `Secure`; navegadores
aceitam isso em `localhost`.

## Testes

```sh
# Go: cria um banco novo por pacote de teste
TEST_DATABASE_URL=postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable go test ./...

# front
cd web && npm run lint && npm run typecheck && npm test

# e2e: precisa de API + worker servindo o front compilado, com banco vazio
cd web && npm run build && cd .. && go build -o /tmp/financas ./cmd/financas
PUBLIC_URL=http://localhost:3100 ... /tmp/financas serve   # (mesmas variáveis de cima)
cd web && npx playwright test          # CHROMIUM_PATH=... para usar um Chromium já instalado
```

O e2e cria a primeira conta, então precisa de um banco recém-migrado.
