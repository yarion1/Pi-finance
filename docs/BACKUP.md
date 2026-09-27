# Backups e restauração

O serviço `backup` do `deploy/docker-compose.yml` (imagem `financas-backup:local`, construída
pelo `deploy/deploy.sh`) roda `deploy/backup/backup.sh` a cada 6 horas:

| O quê | Quando | Onde | Guarda |
| --- | --- | --- | --- |
| Dump do Postgres num repositório **restic** cifrado | a cada 6 h | `BACKUP_DIR/restic` (padrão `/srv/financas/backups`) | 4 últimos, 7 diários, 4 semanais, 12 mensais |
| Cópia do repositório na nuvem (rclone) | 1× por dia, se `BACKUP_NUVEM` estiver definida | o remoto do rclone | 30 dias |
| Teste de restore | 1× por semana | banco temporário `financas_restore_teste` no mesmo Postgres, apagado no fim | — |

O teste restaura o último dump e compara os totais (usuários, contas, transações, soma dos
valores, operações e versão das migrações) com os do banco na hora do dump. O resultado
aparece no `/api/health` (`backup` e `restore`), que o monitor do PiControl já consulta, e em
**Mais › Segurança › Backups do servidor**.

Antes de cada deploy, o `deploy.sh` ainda faz um `pg_dump` simples em
`/srv/financas/backups/pre-deploy-*.dump` (os 3 últimos), usado para voltar um deploy que
falhou.

## Senhas e chaves: guarde fora do Pi

- **Senha do restic**: criada no primeiro deploy em `/srv/financas/segredos/restic.senha`
  (o PiControl avisa). Sem ela nenhum backup abre. Guarde no gerenciador de senhas.
- **Chave mestra** (`/etc/financas/master.key`): sem ela o banco restaurado abre, mas as
  credenciais do Pluggy, CPF/CNPJ, segredos do 2FA e inscrições de notificação não
  decifram. Também no gerenciador de senhas.

## Disco USB

Monte o disco (por exemplo em `/mnt/backup`, com entrada no `/etc/fstab`) e ponha no
`/etc/financas/.env`:

```sh
BACKUP_DIR=/mnt/backup/financas
```

O próximo deploy passa a gravar lá (o repositório novo é criado sozinho; o antigo em
`/srv/financas/backups/restic` pode ser copiado para lá antes, com `cp -a`).

## Nuvem (opcional)

1. No Pi, crie o remoto: `rclone config --config /srv/financas/segredos/rclone.conf`
   (Google Drive, Backblaze B2, etc.). O arquivo fica só no Pi, fora do repositório.
2. No `/etc/financas/.env`: `BACKUP_NUVEM=nome-do-remoto:financas`.
3. Rode o deploy de novo. A primeira cópia sai no próximo ciclo.

A cópia usa o mesmo cifrado do restic: o provedor só vê blocos cifrados.

## Restaurar

Para conferir o que existe:

```sh
C="env VERSAO=$(cat /srv/financas/estado/versao-atual) docker compose -p financas \
  -f deploy/docker-compose.yml --env-file /etc/financas/.env"
$C run --rm -T --entrypoint restic backup snapshots
```

Para voltar o banco inteiro para um snapshot (apaga o banco atual; pare o app antes):

```sh
$C stop web worker backup
$C exec -T postgres dropdb -U financas --force financas
$C exec -T postgres createdb -U financas -O financas financas
$C run --rm -T --entrypoint restic backup dump latest financas.dump \
  | $C exec -T postgres pg_restore -U financas -d financas --exit-on-error
$C up -d web worker backup
```

Troque `latest` pelo id de um snapshot para voltar a um ponto específico. Para ler direto
da nuvem, acrescente `-e RESTIC_REPOSITORY=rclone:remoto:financas` ao `run`.

Num Pi novo: instale como no primeiro deploy, copie a chave mestra para
`/etc/financas/master.key`, a senha do restic para `/srv/financas/segredos/restic.senha`
(e o `rclone.conf`, se usar a nuvem), ligue o disco USB no mesmo `BACKUP_DIR` e siga os
passos acima.
