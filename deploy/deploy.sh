#!/usr/bin/env bash
# Deploy no Pi (docs/SPEC.md §11): build ARM64 local → backup → migrações → subir →
# /api/health em até 90 s → se falhar, volta a imagem E o banco anteriores.
#
#   deploy/deploy.sh v0.1.0            deploy normal
#   QUEBRAR=1 deploy/deploy.sh v0.1.1  força o health a falhar (teste da volta automática)
#   PULAR_BUILD=1 deploy/deploy.sh v0.1.0  reimplanta uma imagem que já existe
#
# Precisa: docker (usuário no grupo docker), /etc/financas/.env legível e /srv/financas gravável.
set -euo pipefail

VERSAO="${1:?uso: deploy.sh vX.Y.Z}"
RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ENV_FILE:-/etc/financas/.env}"
DADOS="${DADOS:-/srv/financas}"
ESTADO="$DADOS/estado"
BACKUPS="$DADOS/backups"
SAUDE_URL="${SAUDE_URL:-http://127.0.0.1:3100/api/health}"
LIMITE_SAUDE="${LIMITE_SAUDE:-90}"

[ -r "$ENV_FILE" ] || { echo "sem leitura em $ENV_FILE" >&2; exit 1; }
# a chave é do uid 65532 (0600): o runner só confere que ela existe; quem lê é o container
[ -e /etc/financas/master.key ] || { echo "chave mestra ausente em /etc/financas/master.key" >&2; exit 1; }
mkdir -p "$ESTADO" "$BACKUPS"

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

compose() { docker compose -p financas -f "$RAIZ/deploy/docker-compose.yml" --env-file "$ENV_FILE" "$@"; }
log() { echo "[$(date '+%F %T')] $*"; }

# Avisos pelo PiControl (POST /api/notify com Bearer) e, sem ele, direto no Telegram.
notificar() {
  local texto="Finanças: $1"
  if [ -n "${PICONTROL_URL:-}" ] && [ -n "${PICONTROL_TOKEN:-}" ]; then
    curl -fsS -m 10 -X POST "$PICONTROL_URL/api/notify" \
      -H "Authorization: Bearer $PICONTROL_TOKEN" -H "Content-Type: application/json" \
      -d "{\"text\":\"$(printf '%s' "$texto" | sed 's/\\/\\\\/g; s/"/\\"/g')\",\"key\":\"financas-deploy\"}" >/dev/null \
      || log "aviso ao PiControl falhou"
  elif [ -n "${TELEGRAM_BOT_TOKEN:-}" ] && [ -n "${TELEGRAM_CHAT_ID:-}" ]; then
    curl -fsS -m 10 "https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN/sendMessage" \
      --data-urlencode "chat_id=$TELEGRAM_CHAT_ID" --data-urlencode "text=$texto" >/dev/null \
      || log "aviso ao Telegram falhou"
  fi
  log "$texto"
}

ANTERIOR="$(cat "$ESTADO/versao-atual" 2>/dev/null || true)"
log "deploy $VERSAO (anterior: ${ANTERIOR:-nenhuma})"

# 1. build ARM64 no próprio Pi (PULAR_BUILD=1 reaproveita a imagem financas:VERSAO já existente)
if [ "${PULAR_BUILD:-0}" = 1 ]; then
  docker image inspect "financas:$VERSAO" >/dev/null || { echo "imagem financas:$VERSAO não existe" >&2; exit 1; }
else
  docker build --pull -t "financas:$VERSAO" --build-arg "VERSAO=$VERSAO" "$RAIZ"
fi

# 2. banco de pé (com a imagem que já roda, se houver)
VERSAO="${ANTERIOR:-$VERSAO}" compose up -d --wait postgres

# 3. backup antes de migrar
DUMP="$BACKUPS/pre-deploy-$VERSAO-$(date +%Y%m%d-%H%M%S).dump"
compose exec -T postgres pg_dump -U financas -Fc financas > "$DUMP.parcial"
mv "$DUMP.parcial" "$DUMP"
log "backup pré-deploy: $DUMP ($(stat -c %s "$DUMP") bytes)"

MIGROU=0
voltar() {
  local motivo="$1"
  log "FALHOU ($motivo); voltando"
  compose logs --tail 80 web worker 2>/dev/null || true
  if [ -n "$ANTERIOR" ]; then
    compose stop web worker || true
    if [ "$MIGROU" = 1 ]; then
      log "restaurando o banco de $DUMP"
      # recria o banco inteiro: tabelas novas da migração não podem sobrar
      compose exec -T postgres dropdb -U financas --force financas
      compose exec -T postgres createdb -U financas -O financas financas
      compose exec -T postgres pg_restore -U financas -d financas --single-transaction < "$DUMP"
    fi
    VERSAO="$ANTERIOR" FORCAR_FALHA_HEALTH=0 compose up -d web worker backup
    notificar "deploy $VERSAO falhou ($motivo); voltou para $ANTERIOR"
  else
    compose stop web worker || true
    notificar "primeiro deploy $VERSAO falhou ($motivo); nada para voltar"
  fi
  exit 1
}

# 4. migrações com o papel dono (a senha não fica no ambiente dos serviços)
export DATABASE_URL_MIGRACAO="postgres://financas:${POSTGRES_PASSWORD}@postgres:5432/financas?sslmode=disable"
MIGROU=1
VERSAO="$VERSAO" compose run --rm --no-deps -e DATABASE_URL_MIGRACAO -e APP_DB_PASSWORD web migrar || voltar "migrações"

# 5. versão nova
FORCAR=0
[ "${QUEBRAR:-0}" = 1 ] && FORCAR=1 && log "QUEBRAR=1: o health vai falhar de propósito"
VERSAO="$VERSAO" FORCAR_FALHA_HEALTH="$FORCAR" compose up -d --remove-orphans web worker backup || voltar "compose up"

# 6. saúde em até 90 s
for ((i = 0; i < LIMITE_SAUDE; i += 3)); do
  if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 3 "$SAUDE_URL")" = 200 ]; then
    echo "$VERSAO" > "$ESTADO/versao-atual"
    date -Iseconds > "$ESTADO/ultimo-deploy"
    notificar "deploy $VERSAO ok"
    # mantém as 3 imagens mais recentes para voltar rápido
    docker images financas --format '{{.Tag}}' | grep -v '<none>' | sort -V -r | tail -n +4 \
      | xargs -r -I{} docker rmi "financas:{}" >/dev/null 2>&1 || true
    ls -1t "$BACKUPS"/pre-deploy-*.dump 2>/dev/null | tail -n +11 | xargs -r rm -f
    exit 0
  fi
  sleep 3
done
voltar "health não respondeu 200 em ${LIMITE_SAUDE}s"
