#!/bin/sh
# Backup do Finanças (docs/SPEC.md §10, DECISOES D42). A cada ciclo (6 h):
#   1. dump do Postgres direto para um repositório restic cifrado (disco do Pi ou USB);
#      retenção: as 4 últimas, 7 diárias, 4 semanais e 12 mensais;
#   2. uma vez por dia, se NUVEM estiver definida, copia para o restic na nuvem (rclone),
#      guardando 30 dias;
#   3. uma vez por semana, restaura o último dump num banco temporário e confere os totais
#      (usuários, contas, transações, soma dos valores, operações, versão das migrações).
# O resultado vai para sinais_vida ('backup' e 'restore_teste'): o /api/health e a tela
# Segurança mostram a idade do backup e o último teste; o PiControl alerta pelo health.
#
# Variáveis: PG* (conexão), RESTIC_REPOSITORY, RESTIC_PASSWORD_FILE, INTERVALO_SEGUNDOS,
# RESTORE_DIAS (7), NUVEM (ex.: gdrive:financas; vazio = sem nuvem), RCLONE_CONFIG,
# UMA_VEZ=1 (um ciclo e sai; testes), FORCAR_RESTORE=1 e FORCAR_NUVEM=1.
set -eu
set -o pipefail

ESTADO="${ESTADO:-/backups/estado}"
export PGOPTIONS="${PGOPTIONS:--c client_min_messages=warning}"
BANCO_TESTE="financas_restore_teste"
mkdir -p "$ESTADO"

log() { echo "[$(date '+%F %T')] $*"; }

# grava o sinal de vida; os valores vão como variáveis do psql (nada é colado no SQL)
sinal() {
  psql -q -v ON_ERROR_STOP=1 -v servico="$1" -v dados="$2" -d "$PGDATABASE" <<'SQL' >/dev/null
insert into sinais_vida (servico, visto_em, dados) values (:'servico', now(), :'dados'::jsonb)
on conflict (servico) do update set visto_em = now(), dados = excluded.dados;
SQL
}

# texto seguro dentro de JSON (aspas, barras e quebras de linha)
json_texto() { printf '%s' "$1" | tr '\n\r\t' '   ' | sed 's/\\/\\\\/g; s/"/\\"/g' | cut -c1-300; }

totais() {
  psql -qAt -v ON_ERROR_STOP=1 -d "$1" -c "select concat_ws('|',
    (select count(*) from usuarios), (select count(*) from contas), (select count(*) from transacoes),
    (select coalesce(sum(valor_centavos), 0) from transacoes), (select count(*) from operacoes),
    (select max(version_id) from goose_db_version))"
}

idade_horas() { # horas desde o carimbo guardado no arquivo (ou muito, se não existe)
  if [ -f "$1" ]; then echo $(( ($(date +%s) - $(cat "$1")) / 3600 )); else echo 999999; fi
}

garantir_repositorio() {
  if restic cat config >/dev/null 2>"$ESTADO/erro"; then
    return 0
  fi
  if grep -qi "wrong password\|no key found" "$ESTADO/erro"; then
    log "a senha do restic não abre o repositório $RESTIC_REPOSITORY"
    return 1
  fi
  log "criando o repositório restic em $RESTIC_REPOSITORY"
  restic init >/dev/null || return 1
}

TOTAIS_DO_DUMP=""
SNAPSHOT=""
BYTES=0

fazer_backup() {
  SNAPSHOT=""
  TOTAIS_DO_DUMP="$(totais "$PGDATABASE")" || return 1
  saida="$(pg_dump -Fc | restic backup --stdin --stdin-filename financas.dump --tag auto --json | tail -n 1)" || {
    log "pg_dump ou restic falhou"
    return 1
  }
  SNAPSHOT="$(printf '%s' "$saida" | sed -n 's/.*"snapshot_id":"\([0-9a-f]*\)".*/\1/p')"
  BYTES="$(printf '%s' "$saida" | sed -n 's/.*"total_bytes_processed":\([0-9]*\).*/\1/p')"
  [ -n "$SNAPSHOT" ] || { log "restic não devolveu o snapshot: $saida"; return 1; }
  restic forget --tag auto --keep-last 4 --keep-daily 7 --keep-weekly 4 --keep-monthly 12 --prune >/dev/null ||
    log "restic forget falhou (o backup foi feito)"
  # os dumps sem cifra do backup antigo (antes da v0.9.0) somem depois de 7 dias
  find /backups -maxdepth 1 -name 'financas-*.dump' -mtime +7 -delete 2>/dev/null || true
  log "backup ok: snapshot ${SNAPSHOT%"${SNAPSHOT#????????}"} (${BYTES:-0} bytes)"
}

NUVEM_JSON='null'
copiar_nuvem() {
  [ -n "${NUVEM:-}" ] || return 0
  if [ "${FORCAR_NUVEM:-0}" != 1 ] && [ "$(idade_horas "$ESTADO/nuvem")" -lt 20 ]; then
    NUVEM_JSON="$(cat "$ESTADO/nuvem.json" 2>/dev/null || echo null)"
    return 0
  fi
  destino="rclone:$NUVEM"
  if erro="$(
    {
      RESTIC_FROM_PASSWORD_FILE="$RESTIC_PASSWORD_FILE"
      export RESTIC_FROM_PASSWORD_FILE
      restic -r "$destino" cat config >/dev/null 2>&1 ||
        restic -r "$destino" init --from-repo "$RESTIC_REPOSITORY" --copy-chunker-params >/dev/null
      restic -r "$destino" copy --from-repo "$RESTIC_REPOSITORY" --tag auto >/dev/null
      restic -r "$destino" forget --tag auto --keep-within 30d --prune >/dev/null
    } 2>&1
  )"; then
    date +%s >"$ESTADO/nuvem"
    NUVEM_JSON="{\"em\":\"$(date -Iseconds)\",\"ok\":true}"
    log "cópia na nuvem ok ($NUVEM)"
  else
    NUVEM_JSON="{\"em\":\"$(date -Iseconds)\",\"ok\":false,\"erro\":\"$(json_texto "$erro")\"}"
    log "cópia na nuvem FALHOU: $erro"
  fi
  printf '%s' "$NUVEM_JSON" >"$ESTADO/nuvem.json"
}

testar_restore() {
  if [ "${FORCAR_RESTORE:-0}" != 1 ] && [ "$(idade_horas "$ESTADO/restore")" -lt $((${RESTORE_DIAS:-7} * 24)) ]; then
    return 0
  fi
  inicio=$(date +%s)
  ok=false
  detalhe=""
  restaurados=""
  psql -q -d postgres -c "drop database if exists $BANCO_TESTE" >/dev/null
  if ! restic check >/dev/null 2>"$ESTADO/erro"; then
    detalhe="restic check falhou: $(cat "$ESTADO/erro")"
  elif ! psql -q -d postgres -c "create database $BANCO_TESTE" >/dev/null; then
    detalhe="não criou o banco de teste"
  elif ! restic dump "$SNAPSHOT" financas.dump | pg_restore -d "$BANCO_TESTE" --exit-on-error 2>"$ESTADO/erro"; then
    detalhe="pg_restore falhou: $(cat "$ESTADO/erro")"
  else
    restaurados="$(totais "$BANCO_TESTE")"
    # quem escreveu entre o dump e agora não é falha: aceita os totais da hora do dump
    if [ "$restaurados" = "$TOTAIS_DO_DUMP" ]; then
      ok=true
      detalhe="totais conferem"
    else
      detalhe="totais diferentes: dump $TOTAIS_DO_DUMP, restaurado $restaurados"
    fi
  fi
  psql -q -d postgres -c "drop database if exists $BANCO_TESTE" >/dev/null || true
  segundos=$(($(date +%s) - inicio))
  sinal restore_teste "{\"ok\":$ok,\"detalhe\":\"$(json_texto "$detalhe")\",\"snapshot\":\"$SNAPSHOT\",\"totais\":\"$(json_texto "$restaurados")\",\"segundos\":$segundos}"
  if [ "$ok" = true ]; then
    date +%s >"$ESTADO/restore"
    log "teste de restore ok em ${segundos}s ($restaurados)"
  else
    log "teste de restore FALHOU: $detalhe"
  fi
}

# dentro de "if ! ciclo" o set -e não vale: cada passo que importa confere o próprio erro
ciclo() {
  garantir_repositorio || return 1
  fazer_backup || return 1
  copiar_nuvem
  sinal backup "{\"snapshot\":\"$SNAPSHOT\",\"bytes\":${BYTES:-0},\"cifrado\":true,\"nuvem\":$NUVEM_JSON}"
  testar_restore
}

while true; do
  if ! ciclo; then
    log "backup FALHOU"
  fi
  [ "${UMA_VEZ:-0}" = 1 ] && break
  sleep "${INTERVALO_SEGUNDOS:-21600}"
done
