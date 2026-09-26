-- Fase 6 — IA: cada pessoa liga ou desliga a IA para as próprias finanças e tem um
-- teto de gasto mensal com a API; o consumo fica registrado por mês (SPEC §6, custos).

-- +goose Up
create table ia_config (
  usuario_id               uuid primary key references usuarios (id) on delete cascade,
  ativa                    boolean not null default false,
  teto_mensal_microdolares bigint not null default 5000000 check (teto_mensal_microdolares between 0 and 1000000000),
  atualizada_em            timestamptz not null default now()
);

create table ia_uso (
  usuario_id          uuid not null references usuarios (id) on delete cascade,
  mes                 date not null check (extract(day from mes) = 1),
  chamadas            bigint not null default 0,
  tokens_entrada      bigint not null default 0,
  tokens_saida        bigint not null default 0,
  custo_microdolares  bigint not null default 0,
  primary key (usuario_id, mes)
);

-- categoria escolhida pela IA (a tela mostra; corrigir à mão desmarca)
alter table transacoes add column categorizada_por_ia boolean not null default false;

alter table ia_config enable row level security;
create policy ia_config_dono on ia_config for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());
alter table ia_uso enable row level security;
create policy ia_uso_dono on ia_uso for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());

-- o worker (sem usuário na sessão) precisa saber quem ligou a IA para categorizar
-- +goose StatementBegin
create function app_usuarios_com_ia() returns setof uuid
language sql stable security definer set search_path = public, pg_temp as $$
  select usuario_id from ia_config where ativa
$$;
-- +goose StatementEnd

-- +goose Down
drop function if exists app_usuarios_com_ia();
drop table if exists ia_uso, ia_config;
alter table transacoes drop column if exists categorizada_por_ia;
