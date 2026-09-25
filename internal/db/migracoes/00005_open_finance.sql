-- Fase 3 — Open Finance pelo Meu Pluggy (docs/SPEC.md §5): conexão por usuário,
-- sincronização diária e conciliação de saldo. Só acrescenta.

-- +goose Up

-- ---------------------------------------------------------------------------
-- Credenciais do Meu Pluggy: uma por pessoa, cifradas (cripto.Cifrador, contexto =
-- nome da coluna) e visíveis só para a própria pessoa.
-- ---------------------------------------------------------------------------
create table conexoes_pluggy (
  id             uuid primary key default gen_random_uuid(),
  usuario_id     uuid not null unique references usuarios (id) on delete cascade,
  client_id      text not null,   -- cifrado
  client_secret  text not null,   -- cifrado
  client_id_fim  text not null,   -- últimos 4 caracteres, só para reconhecer na tela
  criada_em      timestamptz not null default now(),
  ultima_sync    timestamptz,
  ultimo_erro    text
);

-- Cada item é um banco conectado no Meu Pluggy; as contas dele entram numa entidade.
create table itens_pluggy (
  id             uuid primary key default gen_random_uuid(),
  conexao_id     uuid not null references conexoes_pluggy (id) on delete cascade,
  usuario_id     uuid not null references usuarios (id) on delete cascade,
  item_id        text not null,
  entidade_id    uuid not null references entidades (id) on delete cascade,
  instituicao    text,
  status         text,
  atualizado_em  timestamptz,  -- quando a Pluggy leu o banco pela última vez
  ultima_sync    timestamptz,  -- quando o painel buscou os dados
  ultimo_erro    text,
  criado_em      timestamptz not null default now(),
  unique (conexao_id, item_id)
);

-- Contas que a Pluggy devolve. Só as vinculadas a uma conta do painel importam
-- transações; as outras esperam a pessoa decidir (vincular, criar ou ignorar).
create table contas_pluggy (
  id               uuid primary key default gen_random_uuid(),
  item_id          uuid not null references itens_pluggy (id) on delete cascade,
  usuario_id       uuid not null references usuarios (id) on delete cascade,
  pluggy_id        text not null,
  nome             text not null,
  tipo             text not null,      -- BANK ou CREDIT
  subtipo          text,
  numero           text,
  moeda            char(3) not null default 'BRL',
  saldo_centavos   bigint,
  limite_centavos  bigint,
  dia_fechamento   smallint,
  dia_vencimento   smallint,
  saldo_em         timestamptz,
  conta_id         uuid references contas (id) on delete set null,
  ignorada         boolean not null default false,
  acertar_saldo    boolean not null default false,  -- conta criada pela Pluggy: saldo inicial sai do banco
  ultima_sync      timestamptz,                     -- última importação de transações desta conta
  unique (item_id, pluggy_id)
);
create unique index contas_pluggy_conta on contas_pluggy (conta_id) where conta_id is not null;

-- ---------------------------------------------------------------------------
-- Conciliação: saldo do banco x saldo calculado, um registro por conta por dia
-- ---------------------------------------------------------------------------
create table conciliacoes (
  id                        uuid primary key default gen_random_uuid(),
  entidade_id               uuid not null references entidades (id) on delete cascade,
  conta_id                  uuid not null references contas (id) on delete cascade,
  data                      date not null,
  saldo_banco_centavos      bigint not null,
  saldo_calculado_centavos  bigint not null,
  criada_em                 timestamptz not null default now(),
  unique (conta_id, data)
);

-- A mesma transação vinda da Pluggy e de um arquivo vira uma só (DECISOES D21)
alter table transacoes
  add column id_pluggy text,
  add column casada boolean not null default false;
create unique index transacoes_id_pluggy on transacoes (conta_id, id_pluggy);

-- ---------------------------------------------------------------------------
-- RLS
-- ---------------------------------------------------------------------------
alter table conexoes_pluggy enable row level security;
create policy conexoes_pluggy_dono on conexoes_pluggy for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());

alter table itens_pluggy enable row level security;
create policy itens_pluggy_dono on itens_pluggy for all
  using (usuario_id = app_usuario_id())
  with check (usuario_id = app_usuario_id() and app_pode_editar_entidade(entidade_id));

alter table contas_pluggy enable row level security;
create policy contas_pluggy_dono on contas_pluggy for all
  using (usuario_id = app_usuario_id())
  with check (
    usuario_id = app_usuario_id()
    and (conta_id is null
         or exists (select 1 from contas c where c.id = conta_id and app_pode_editar_entidade(c.entidade_id))));

alter table conciliacoes enable row level security;
create policy conciliacoes_ver on conciliacoes for select using (app_pode_ver_entidade(entidade_id));
create policy conciliacoes_escrever on conciliacoes for all
  using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id));

-- ---------------------------------------------------------------------------
-- O worker precisa saber de quem sincronizar, e o /api/health do estado geral,
-- sem ler credenciais: só ids e datas.
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
create function app_usuarios_para_sincronizar(p_antes timestamptz) returns setof uuid
language sql stable security definer set search_path = public, pg_temp as $$
  select usuario_id from conexoes_pluggy
  where ultima_sync is null or ultima_sync < p_antes
  order by ultima_sync nulls first
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function app_estado_pluggy(out conexoes integer, out com_erro integer, out ultima_sync timestamptz)
language sql stable security definer set search_path = public, pg_temp as $$
  select count(*)::integer,
         count(*) filter (where ultimo_erro is not null
           or exists (select 1 from itens_pluggy i where i.conexao_id = c.id and i.ultimo_erro is not null))::integer,
         max(c.ultima_sync)
  from conexoes_pluggy c
$$;
-- +goose StatementEnd

-- +goose Down
drop function if exists app_estado_pluggy();
drop function if exists app_usuarios_para_sincronizar(timestamptz);
drop index if exists transacoes_id_pluggy;
alter table transacoes drop column if exists id_pluggy, drop column if exists casada;
drop table if exists conciliacoes, contas_pluggy, itens_pluggy, conexoes_pluggy cascade;
