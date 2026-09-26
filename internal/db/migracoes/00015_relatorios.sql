-- Relatório do mês (dia 1) e resumo da semana (domingo à noite), por pessoa.

-- +goose Up
create table relatorios (
  id          uuid primary key default gen_random_uuid(),
  usuario_id  uuid not null references usuarios (id) on delete cascade,
  tipo        text not null check (tipo in ('mes', 'semana')),
  periodo     text not null,  -- '2026-09' (mês) ou '2026-09-21' (segunda-feira da semana)
  dados       jsonb not null,
  texto       text,           -- resumo escrito pela IA, quando a pessoa ligou
  criado_em   timestamptz not null default now(),
  unique (usuario_id, tipo, periodo)
);

alter table relatorios enable row level security;
create policy relatorios_dono on relatorios for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());

-- o worker (sem usuário na sessão) gera os relatórios de quem tem alguma entidade
-- +goose StatementBegin
create function app_usuarios_com_entidade() returns setof uuid
language sql stable security definer set search_path = public, pg_temp as $$
  select distinct dono_id from entidades
$$;
-- +goose StatementEnd

-- +goose Down
drop function if exists app_usuarios_com_entidade();
drop table if exists relatorios;
