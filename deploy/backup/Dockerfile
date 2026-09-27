# Backup do Finanças: as ferramentas do Postgres da mesma versão maior do banco, com
# restic (repositório cifrado) e rclone (cópia na nuvem). O script entra por volume.
FROM postgres:17-alpine
RUN apk add --no-cache restic rclone && restic version && rclone version | head -n 1
ENTRYPOINT ["/bin/sh", "/backup.sh"]
