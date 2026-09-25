# Finanças

Painel de finanças auto-hospedado no Raspberry Pi 5: contas, cartões, investimentos e o
CNPJ de várias pessoas da mesma casa, com privacidade garantida no banco (RLS),
2FA obrigatório e acesso só pela LAN e pela tailnet.

- Especificação: [`docs/SPEC.md`](docs/SPEC.md) · Decisões: [`docs/DECISOES.md`](docs/DECISOES.md)
- Desenvolvimento local: [`docs/DESENVOLVIMENTO.md`](docs/DESENVOLVIMENTO.md) · Histórico: [`CHANGELOG.md`](CHANGELOG.md)

> O painel organiza e simula; não substitui contador nem assessor de investimentos.

## Estado

- **Fase 0 (fundação)**: login com senha + TOTP ou passkey, códigos de recuperação, casas,
  convites, entidades PF/CNPJ com papéis, RLS com teste de isolamento, `/api/health`,
  worker, backup e deploy com volta automática.
- **Fase 1 (contas e transações)**: contas e cartões, importação de OFX e CSV (Nubank, Inter,
  genérico) sem duplicar, transferências internas fora do gasto, categorias e regras,
  telas de Início e Gastos.

As próximas fases estão na seção 12 da SPEC.

## Arquitetura

```
navegador (PWA) ──HTTPS──> Cloudflare Access + Tunnel ──> cloudflared no Pi ──> 127.0.0.1:3100
                                                              │
                              financas serve  (API Go + front React embutido)
                              financas worker (River: sinal de vida, limpeza…)
                                                              │
                                        PostgreSQL 17 (RLS por entidade e casa)
```

Uma imagem (`financas:vX.Y.Z`) roda como `serve`, `worker` ou `migrar`. O Postgres fica
numa rede Docker sem saída. Veja `deploy/docker-compose.yml`.

## Preparar o Pi (uma vez)

1. **Runner**: em Settings → Actions → Runners, registre um runner self-hosted no Pi com a
   etiqueta `financas` (como o do PiControl). O usuário do runner precisa estar no grupo
   `docker`. Crie a variável `PI_DEPLOY=true` em Settings → Secrets and variables → Actions
   → Variables.
2. **Chave mestra** (dono = usuário da imagem, uid 65532; ver DECISOES D7):
   ```sh
   sudo mkdir -p /etc/financas
   sudo sh -c 'openssl rand -base64 32 > /etc/financas/master.key'
   sudo chown 65532:65532 /etc/financas/master.key && sudo chmod 600 /etc/financas/master.key
   ```
   Guarde uma cópia da chave no gerenciador de senhas: sem ela, CPF/CNPJ e segredos
   cifrados não abrem, nem restaurando o backup.
3. **Ambiente**: copie `deploy/env.exemplo` para `/etc/financas/.env` e preencha.
   ```sh
   sudo chown root:docker /etc/financas/.env && sudo chmod 640 /etc/financas/.env
   ```
4. **Dados**: `sudo mkdir -p /srv/financas && sudo chown "$USER_DO_RUNNER":docker /srv/financas`.
5. **Acesso** (escolha um):
   - **Cloudflare Tunnel** (D11): no túnel, hostname `financas.<domínio>` → `http://localhost:3100`,
     com **Cloudflare Access** na frente (só os e-mails da casa). `PUBLIC_URL=https://financas.<domínio>`.
   - **Só rede de casa** (D12): `ESCUTAR=0.0.0.0` e `PUBLIC_URL=http://<ip-do-pi>:3100`
     (sem passkeys; libere a porta 3100 no firewall só para a rede de casa).

## Deploy

```sh
git tag v0.1.0 && git push origin v0.1.0
```

O workflow `release` roda o CI completo e, no runner do Pi, `deploy/deploy.sh`:
build ARM64 → backup do banco → migrações → sobe web, worker e backup → espera
`/api/health` = 200 por até 90 s. Se falhar, **volta a imagem e o banco anteriores** e avisa.

Testar a volta automática: `gh workflow run release.yml --ref v0.1.0 -f quebrar=true`
(o health da versão nova responde 503 de propósito; o Pi deve voltar sozinho).

## PiControl

- **Sites e APIs**: monitor `http://127.0.0.1:3100/api/health`, JSON esperado
  `db=up, worker=up, pluggy=ok, backup=ok`.
- **Projetos**: repo `yarion1/Pi-finance`, compose `financas`, monitor acima.
- **Avisos**: crie um token de API no PiControl e ponha em `PICONTROL_TOKEN`; o deploy
  chama `POST /api/notify`.

## Primeiro acesso

Abra `https://financas.<domínio>`, crie a primeira conta, configure a passkey ou o
aplicativo autenticador (obrigatório) e guarde os códigos de recuperação. Depois crie a
sua entidade PF, a casa, e convide as outras pessoas pelo link.
