#!/usr/bin/env bash
# Desliga o Finanças no Pi sem perder nada, ou religa (DECISOES D46).
#
#   deploy/desligar.sh desligar   último backup cifrado + teste de restore, para tudo
#   deploy/desligar.sh ligar      sobe de novo a última versão e confere o /api/health
#
# Desligado, nada é apagado: banco (/srv/financas/postgres), backups, segredos, imagens e o
# .env ficam onde estão. Os containers só param (e não voltam sozinhos num reboot). O
# deploy.sh não sobe nada enquanto existir /srv/financas/estado/desligado.
set -euo pipefail

ACAO="${1:?uso: desligar.sh desligar|ligar}"
RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ENV_FILE:-/etc/financas/.env}"
DADOS="${DADOS:-/srv/financas}"
ESTADO="$DADOS/estado"
SAUDE_URL="${SAUDE_URL:-http://127.0.0.1:3100/api/health}"

[ -r "$ENV_FILE" ] || { echo "sem leitura em $ENV_FILE" >&2; exit 1; }
set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a
VERSAO="$(cat "$ESTADO/versao-atual" 2>/dev/null || true)"
[ -n "$VERSAO" ] || { echo "nenhuma versão instalada em $ESTADO/versao-atual" >&2; exit 1; }
export VERSAO COMPOSE_PROFILES=tunel # o perfil do túnel entra para o stop pegar o cloudflared também

compose() { docker compose -p financas -f "$RAIZ/deploy/docker-compose.yml" --env-file "$ENV_FILE" "$@"; }
log() { echo "[$(date '+%F %T')] $*"; }
notificar() {
  local texto="Finanças: $1"
  if [ -n "${PICONTROL_URL:-}" ] && [ -n "${PICONTROL_TOKEN:-}" ]; then
    curl -fsS -m 10 -X POST "$PICONTROL_URL/api/notify" \
      -H "Authorization: Bearer $PICONTROL_TOKEN" -H "Content-Type: application/json" \
      -d "{\"text\":\"$(printf '%s' "$texto" | sed 's/\\/\\\\/g; s/"/\\"/g')\",\"key\":\"financas-deploy\"}" >/dev/null \
      || log "aviso ao PiControl falhou"
  fi
  log "$texto"
}
rodando() { docker ps -q --filter label=com.docker.compose.project=financas | wc -l; }

case "$ACAO" in
desligar)
  if [ -e "$ESTADO/desligado" ]; then
    log "já estava desligado desde $(cat "$ESTADO/desligado")"
  fi
  # 1. para o app (nada mais escreve no banco) e faz o último backup cifrado, com teste de
  #    restore, enquanto o banco ainda está de pé
  compose stop web worker cloudflared backup
  if [ "$(docker ps -q --filter label=com.docker.compose.project=financas --filter label=com.docker.compose.service=postgres | wc -l)" -gt 0 ]; then
    log "último backup (restic) e teste de restore"
    inicio="$(date -u +%FT%TZ)"
    compose run --rm --no-deps -e UMA_VEZ=1 -e FORCAR_RESTORE=1 backup
    ok="$(compose exec -T postgres psql -U financas -d financas -Atc \
      "select dados->>'ok' from sinais_vida where servico = 'restore_teste' and visto_em >= '$inicio'::timestamptz")"
    if [ "$ok" != true ]; then
      echo "o backup final ou o teste de restore falhou; o painel volta a subir e nada foi desligado" >&2
      SERVICOS=(web worker backup)
      [ -n "${CLOUDFLARE_TUNNEL_TOKEN:-}" ] && SERVICOS+=(cloudflared)
      compose up -d "${SERVICOS[@]}"
      exit 1
    fi
    log "backup final cifrado e teste de restore conferidos"
  else
    log "o banco já estava parado: sem backup novo (o último continua em $DADOS/backups)"
  fi
  # 2. para tudo (os dados ficam em $DADOS)
  compose stop
  date -Iseconds > "$ESTADO/desligado"
  [ "$(rodando)" = 0 ] || { echo "ainda há containers do financas rodando" >&2; docker ps --filter label=com.docker.compose.project=financas; exit 1; }
  notificar "desligado ($VERSAO). Dados e backups guardados em $DADOS; para religar, workflow pi com acao=ligar"
  ;;
ligar)
  rm -f "$ESTADO/desligado"
  compose up -d --wait postgres
  SERVICOS=(web worker backup)
  [ -n "${CLOUDFLARE_TUNNEL_TOKEN:-}" ] && SERVICOS+=(cloudflared)
  compose up -d "${SERVICOS[@]}"
  for ((i = 0; i < 90; i += 3)); do
    if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 3 "$SAUDE_URL")" = 200 ]; then
      log "saúde: $(curl -s -m 3 "$SAUDE_URL")"
      notificar "religado ($VERSAO)"
      exit 0
    fi
    sleep 3
  done
  echo "o /api/health não respondeu 200 em 90 s" >&2
  compose logs --tail 60 web worker || true
  exit 1
  ;;
*)
  echo "uso: desligar.sh desligar|ligar" >&2
  exit 2
  ;;
esac
