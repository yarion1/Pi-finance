-- Investimentos pelo Open Finance: cada investimento que a Pluggy devolve vira um ativo
-- com o saldo informado pelo banco (a instituição avalia melhor que a curva estimada).
-- Só acrescenta.

-- +goose Up
alter table ativos
  add column origem              text not null default 'manual',  -- manual, b3, pluggy
  add column pluggy_id           text,
  add column instituicao         text,
  add column subtipo             text,
  add column saldo_centavos      bigint,       -- valor informado pela instituição
  add column aplicado_centavos   bigint,       -- quanto foi aplicado (para o rendimento)
  add column saldo_em            timestamptz,
  add column encerrado           boolean not null default false;
create unique index ativos_pluggy on ativos (entidade_id, pluggy_id) where pluggy_id is not null;

-- +goose Down
drop index if exists ativos_pluggy;
alter table ativos drop column if exists origem, drop column if exists pluggy_id, drop column if exists instituicao,
  drop column if exists subtipo, drop column if exists saldo_centavos, drop column if exists aplicado_centavos,
  drop column if exists saldo_em, drop column if exists encerrado;
