-- Holerites lidos de PDF ou foto (fase 6): rendimentos e retenções da pessoa física,
-- base para o IR anual.

-- +goose Up
create table holerites (
  id                         uuid primary key default gen_random_uuid(),
  entidade_id                uuid not null references entidades (id) on delete cascade,
  competencia                date not null,  -- primeiro dia do mês
  empregador                 text not null default '',
  bruto_centavos             bigint not null check (bruto_centavos > 0),
  inss_centavos              bigint not null default 0,
  irrf_centavos              bigint not null default 0,
  outros_descontos_centavos  bigint not null default 0,
  liquido_centavos           bigint not null check (liquido_centavos > 0),
  criado_em                  timestamptz not null default now(),
  unique (entidade_id, competencia, empregador)
);

alter table holerites enable row level security;
create policy holerites_ver on holerites for select using (app_pode_ver_entidade(entidade_id));
create policy holerites_escrever on holerites for all
  using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id));

-- +goose Down
drop table if exists holerites;
