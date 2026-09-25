#!/bin/sh
# Dump do Postgres a cada 6 h, guardando os últimos $RETER, e sinal de vida para o /api/health.
# Fase 8: cifrar com restic, cópia na nuvem com rclone e teste de restore semanal.
set -eu
while true; do
  arquivo="/backups/financas-$(date +%Y%m%d-%H%M%S).dump"
  if pg_dump -Fc -f "$arquivo.parcial" && mv "$arquivo.parcial" "$arquivo"; then
    tamanho=$(wc -c < "$arquivo")
    psql -qc "insert into sinais_vida (servico, visto_em, dados)
      values ('backup', now(), jsonb_build_object('arquivo', '$(basename "$arquivo")', 'bytes', $tamanho))
      on conflict (servico) do update set visto_em = now(), dados = excluded.dados" || true
    echo "backup ok: $arquivo ($tamanho bytes)"
    ls -1t /backups/financas-*.dump 2>/dev/null | tail -n +"$((RETER + 1))" | xargs -r rm -f
  else
    rm -f "$arquivo.parcial"
    echo "backup FALHOU" >&2
  fi
  sleep "$INTERVALO_SEGUNDOS"
done
