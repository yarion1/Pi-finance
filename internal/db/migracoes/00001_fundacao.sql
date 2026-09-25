-- Fase 0 — fundação: pessoas, acesso, contas mínimas para o isolamento, referência fiscal.
-- Regras (docs/SPEC.md §1, §9, §10):
--   * dinheiro em centavos (bigint); quantidades/cotações em numeric(24,8);
--   * toda tabela com entidade_id tem RLS; o papel financas_app nunca é dono das tabelas;
--   * o usuário da requisição chega em current_setting('app.usuario_id').

-- +goose Up

create extension if not exists pgcrypto;

-- ---------------------------------------------------------------------------
-- Tipos
-- ---------------------------------------------------------------------------
create type papel_casa as enum ('dono', 'membro', 'leitor');
create type papel_entidade as enum ('membro', 'leitor', 'contador');
create type tipo_entidade as enum ('PF', 'PJ');
create type regime_pj as enum ('MEI', 'SIMPLES_ME', 'SIMPLES_EPP');
create type tipo_conta as enum ('corrente', 'poupanca', 'carteira', 'dinheiro', 'beneficio', 'investimento', 'cartao');
create type visibilidade_conta as enum ('privada', 'saldo', 'compartilhada');
create type tipo_transacao as enum ('gasto', 'receita', 'transferencia', 'estorno');
create type origem_transacao as enum ('pluggy', 'ofx', 'csv', 'manual', 'ia');

-- ---------------------------------------------------------------------------
-- Usuários e autenticação (sem RLS: acessadas só pelo código de autenticação)
-- ---------------------------------------------------------------------------
create table usuarios (
  id                 uuid primary key default gen_random_uuid(),
  nome               text not null,
  email              text not null,
  senha_hash         text not null,
  idioma             text not null default 'pt-BR',
  preferencias       jsonb not null default '{}',
  ia_ativa           boolean not null default false,
  tentativas_falhas  integer not null default 0,
  bloqueado_ate      timestamptz,
  criado_em          timestamptz not null default now(),
  atualizado_em      timestamptz not null default now()
);
create unique index usuarios_email_unico on usuarios (lower(email));

create table sessoes (
  id                uuid primary key default gen_random_uuid(),
  usuario_id        uuid not null references usuarios (id) on delete cascade,
  token_hash        bytea not null unique,
  mfa_ok            boolean not null default false,
  criada_em         timestamptz not null default now(),
  expira_em         timestamptz not null,
  reautenticada_em  timestamptz,
  ip                text,
  user_agent        text
);
create index sessoes_usuario on sessoes (usuario_id);

-- segredo ativo + segredo pendente (em configuração ou troca de aplicativo)
create table dois_fatores (
  usuario_id          uuid primary key references usuarios (id) on delete cascade,
  segredo_cifrado     text,
  confirmado_em       timestamptz,
  ultimo_passo        bigint not null default 0,
  pendente_cifrado    text,
  pendente_expira_em  timestamptz,
  check ((segredo_cifrado is null) = (confirmado_em is null))
);

create table codigos_recuperacao (
  id           uuid primary key default gen_random_uuid(),
  usuario_id   uuid not null references usuarios (id) on delete cascade,
  codigo_hash  bytea not null,
  usado_em     timestamptz
);
create index codigos_recuperacao_usuario on codigos_recuperacao (usuario_id);

create table passkeys (
  id              uuid primary key default gen_random_uuid(),
  usuario_id      uuid not null references usuarios (id) on delete cascade,
  credencial_id   bytea not null unique,
  credencial      jsonb not null,
  nome            text not null,
  criada_em       timestamptz not null default now(),
  usada_em        timestamptz
);
create index passkeys_usuario on passkeys (usuario_id);

create table desafios_webauthn (
  id          uuid primary key default gen_random_uuid(),
  usuario_id  uuid references usuarios (id) on delete cascade,
  dados       jsonb not null,
  expira_em   timestamptz not null
);

-- ---------------------------------------------------------------------------
-- Casas e entidades
-- ---------------------------------------------------------------------------
create table casas (
  id          uuid primary key default gen_random_uuid(),
  nome        text not null,
  moeda_base  char(3) not null default 'BRL',
  criada_em   timestamptz not null default now()
);

create table membros_casa (
  casa_id     uuid not null references casas (id) on delete cascade,
  usuario_id  uuid not null references usuarios (id) on delete cascade,
  papel       papel_casa not null,
  entrou_em   timestamptz not null default now(),
  primary key (casa_id, usuario_id)
);
create index membros_casa_usuario on membros_casa (usuario_id);

create table convites_casa (
  id          uuid primary key default gen_random_uuid(),
  casa_id     uuid not null references casas (id) on delete cascade,
  papel       papel_casa not null check (papel <> 'dono'),
  token_hash  bytea not null unique,
  criado_por  uuid not null references usuarios (id) on delete cascade,
  criado_em   timestamptz not null default now(),
  expira_em   timestamptz not null,
  aceito_por  uuid references usuarios (id) on delete set null,
  aceito_em   timestamptz
);

create table entidades (
  id                  uuid primary key default gen_random_uuid(),
  dono_id             uuid not null references usuarios (id) on delete cascade,
  tipo                tipo_entidade not null,
  nome                text not null,
  documento_cifrado   text,
  regime              regime_pj,
  anexo               text,
  cnae                text,
  municipio           text,
  data_abertura       date,
  criada_em           timestamptz not null default now(),
  check (tipo = 'PJ' or (regime is null and anexo is null and cnae is null))
);
create index entidades_dono on entidades (dono_id);

create table acessos_entidade (
  entidade_id  uuid not null references entidades (id) on delete cascade,
  usuario_id   uuid not null references usuarios (id) on delete cascade,
  papel        papel_entidade not null,
  criado_em    timestamptz not null default now(),
  primary key (entidade_id, usuario_id)
);
create index acessos_entidade_usuario on acessos_entidade (usuario_id);

-- ---------------------------------------------------------------------------
-- Dinheiro (mínimo da fase 0; a fase 1 completa)
-- ---------------------------------------------------------------------------
create table instituicoes (
  id            uuid primary key default gen_random_uuid(),
  nome          text not null,
  codigo_compe  text,
  logo          text
);

create table contas (
  id               uuid primary key default gen_random_uuid(),
  entidade_id      uuid not null references entidades (id) on delete cascade,
  instituicao_id   uuid references instituicoes (id),
  nome             text not null,
  tipo             tipo_conta not null,
  moeda            char(3) not null default 'BRL',
  visibilidade     visibilidade_conta not null default 'privada',
  casa_id          uuid references casas (id) on delete set null,
  fechamento       smallint check (fechamento between 1 and 31),
  vencimento       smallint check (vencimento between 1 and 31),
  limite_centavos  bigint,
  criada_em        timestamptz not null default now(),
  unique (id, entidade_id),
  check (visibilidade = 'privada' or casa_id is not null)
);
create index contas_entidade on contas (entidade_id);

create table transacoes (
  id                     uuid primary key default gen_random_uuid(),
  entidade_id            uuid not null,
  conta_id               uuid not null,
  data                   date not null,
  descricao_original     text not null,
  descricao              text not null,
  valor_centavos         bigint not null,
  moeda                  char(3) not null default 'BRL',
  taxa_cambio            numeric(24, 8),
  categoria_id           uuid,
  tipo                   tipo_transacao not null,
  transferencia_par_id   uuid references transacoes (id) on delete set null,
  parcela_n              smallint,
  parcela_total          smallint,
  compra_id              uuid,
  origem                 origem_transacao not null default 'manual',
  id_externo             text,
  importacao_id          uuid,
  notas                  text,
  criada_em              timestamptz not null default now(),
  -- a transação pertence sempre à mesma entidade da conta
  foreign key (conta_id, entidade_id) references contas (id, entidade_id) on delete cascade
);
create index transacoes_conta_data on transacoes (conta_id, data);
create index transacoes_entidade_data on transacoes (entidade_id, data);

-- ---------------------------------------------------------------------------
-- Referência global com vigência (somente leitura para o app)
-- ---------------------------------------------------------------------------
create table regras_fiscais (
  id           uuid primary key default gen_random_uuid(),
  chave        text not null,
  valor_json   jsonb not null,
  vigente_de   date not null,
  vigente_ate  date,
  fonte        text not null,
  unique (chave, vigente_de),
  check (vigente_ate is null or vigente_ate >= vigente_de)
);

-- ---------------------------------------------------------------------------
-- Operação
-- ---------------------------------------------------------------------------
create table sinais_vida (
  servico   text primary key,
  visto_em  timestamptz not null default now(),
  dados     jsonb not null default '{}'
);

create table auditoria (
  id          uuid primary key default gen_random_uuid(),
  usuario_id  uuid references usuarios (id) on delete set null,
  acao        text not null,
  alvo        text,
  ip          text,
  dados       jsonb not null default '{}',
  quando      timestamptz not null default now()
);
create index auditoria_usuario_quando on auditoria (usuario_id, quando desc);

create table alertas (
  id          uuid primary key default gen_random_uuid(),
  usuario_id  uuid not null references usuarios (id) on delete cascade,
  tipo        text not null,
  dados       jsonb not null default '{}',
  criado_em   timestamptz not null default now(),
  lido_em     timestamptz
);
create index alertas_usuario on alertas (usuario_id, criado_em desc);

-- ---------------------------------------------------------------------------
-- Funções de acesso. SECURITY DEFINER para consultar as tabelas de acesso sem
-- recursão de políticas; nunca recebem o usuário por parâmetro.
-- ---------------------------------------------------------------------------

-- +goose StatementBegin
create function app_usuario_id() returns uuid
language sql stable as $$
  select nullif(current_setting('app.usuario_id', true), '')::uuid
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function app_papel_casa(p_casa uuid) returns papel_casa
language sql stable security definer set search_path = public, pg_temp as $$
  select papel from membros_casa where casa_id = p_casa and usuario_id = app_usuario_id()
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function app_eh_membro_casa(p_casa uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select p_casa is not null and exists (
    select 1 from membros_casa where casa_id = p_casa and usuario_id = app_usuario_id()
  )
$$;
-- +goose StatementEnd

-- 'dono', 'membro', 'leitor', 'contador' ou null (sem acesso).
-- +goose StatementBegin
create function app_papel_entidade(p_entidade uuid) returns text
language sql stable security definer set search_path = public, pg_temp as $$
  select case
    when e.dono_id = app_usuario_id() then 'dono'
    else (select a.papel::text from acessos_entidade a
          where a.entidade_id = e.id and a.usuario_id = app_usuario_id())
  end
  from entidades e where e.id = p_entidade
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function app_pode_ver_entidade(p_entidade uuid) returns boolean
language sql stable as $$
  select app_papel_entidade(p_entidade) is not null
$$;
-- +goose StatementEnd

-- +goose StatementBegin
create function app_pode_editar_entidade(p_entidade uuid) returns boolean
language sql stable as $$
  select coalesce(app_papel_entidade(p_entidade) in ('dono', 'membro'), false)
$$;
-- +goose StatementEnd

-- Conta compartilhada por inteiro (transações visíveis) com uma casa do usuário.
-- +goose StatementBegin
create function app_conta_compartilhada_comigo(p_conta uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select exists (
    select 1 from contas c
    join membros_casa m on m.casa_id = c.casa_id and m.usuario_id = app_usuario_id()
    where c.id = p_conta and c.visibilidade = 'compartilhada'
  )
$$;
-- +goose StatementEnd

-- Contador só pode ser convidado para entidade CNPJ.
-- +goose StatementBegin
create function checa_acesso_contador() returns trigger
language plpgsql security definer set search_path = public, pg_temp as $$
begin
  if new.papel = 'contador' and (select tipo from entidades where id = new.entidade_id) <> 'PJ' then
    raise exception 'o papel contador só vale para entidade PJ' using errcode = 'check_violation';
  end if;
  return new;
end
$$;
-- +goose StatementEnd
create trigger acessos_entidade_contador before insert or update on acessos_entidade
  for each row execute function checa_acesso_contador();

-- Uma entidade PF não pode virar PJ (e vice-versa) com contador já convidado.
-- +goose StatementBegin
create function checa_tipo_entidade() returns trigger
language plpgsql security definer set search_path = public, pg_temp as $$
begin
  if new.tipo <> 'PJ' and exists (
    select 1 from acessos_entidade where entidade_id = new.id and papel = 'contador'
  ) then
    raise exception 'entidade com contador precisa ser PJ' using errcode = 'check_violation';
  end if;
  return new;
end
$$;
-- +goose StatementEnd
create trigger entidades_tipo before update of tipo on entidades
  for each row execute function checa_tipo_entidade();

-- Cria a casa e coloca quem criou como dono, numa operação só.
-- +goose StatementBegin
create function criar_casa(p_nome text) returns uuid
language plpgsql security definer set search_path = public, pg_temp as $$
declare
  v_usuario uuid := app_usuario_id();
  v_casa uuid;
begin
  if v_usuario is null then
    raise exception 'sem usuário na sessão' using errcode = 'insufficient_privilege';
  end if;
  insert into casas (nome) values (p_nome) returning id into v_casa;
  insert into membros_casa (casa_id, usuario_id, papel) values (v_casa, v_usuario, 'dono');
  return v_casa;
end
$$;
-- +goose StatementEnd

-- Consulta pública de um convite (antes do login/cadastro).
-- +goose StatementBegin
create function consultar_convite(p_token_hash bytea)
returns table (casa_nome text, papel papel_casa, expira_em timestamptz, valido boolean)
language sql stable security definer set search_path = public, pg_temp as $$
  select c.nome, v.papel, v.expira_em, (v.aceito_em is null and v.expira_em > now())
  from convites_casa v join casas c on c.id = v.casa_id
  where v.token_hash = p_token_hash
$$;
-- +goose StatementEnd

-- Aceita o convite para o usuário da sessão. Uso único, validade conferida aqui.
-- +goose StatementBegin
create function aceitar_convite(p_token_hash bytea) returns uuid
language plpgsql security definer set search_path = public, pg_temp as $$
declare
  v_usuario uuid := app_usuario_id();
  v_convite convites_casa%rowtype;
begin
  if v_usuario is null then
    raise exception 'sem usuário na sessão' using errcode = 'insufficient_privilege';
  end if;
  select * into v_convite from convites_casa where token_hash = p_token_hash for update;
  if not found or v_convite.aceito_em is not null or v_convite.expira_em <= now() then
    raise exception 'convite inválido ou expirado' using errcode = 'no_data_found';
  end if;
  insert into membros_casa (casa_id, usuario_id, papel)
    values (v_convite.casa_id, v_usuario, v_convite.papel)
    on conflict (casa_id, usuario_id) do nothing;
  update convites_casa set aceito_por = v_usuario, aceito_em = now() where id = v_convite.id;
  return v_convite.casa_id;
end
$$;
-- +goose StatementEnd

-- Evita que uma casa fique sem dono.
-- +goose StatementBegin
create function checa_dono_casa() returns trigger
language plpgsql security definer set search_path = public, pg_temp as $$
declare
  v_casa uuid := coalesce(old.casa_id, new.casa_id);
begin
  if exists (select 1 from casas where id = v_casa)
     and not exists (select 1 from membros_casa where casa_id = v_casa and papel = 'dono') then
    raise exception 'a casa precisa de pelo menos um dono' using errcode = 'check_violation';
  end if;
  return null;
end
$$;
-- +goose StatementEnd
create constraint trigger membros_casa_dono after update or delete on membros_casa
  deferrable initially deferred
  for each row execute function checa_dono_casa();

-- ---------------------------------------------------------------------------
-- Row Level Security
-- ---------------------------------------------------------------------------
alter table casas enable row level security;
create policy casas_ver on casas for select using (app_eh_membro_casa(id));
create policy casas_editar on casas for update using (app_papel_casa(id) = 'dono');
create policy casas_apagar on casas for delete using (app_papel_casa(id) = 'dono');

alter table membros_casa enable row level security;
create policy membros_ver on membros_casa for select using (app_eh_membro_casa(casa_id));
create policy membros_editar on membros_casa for update
  using (app_papel_casa(casa_id) = 'dono') with check (app_papel_casa(casa_id) = 'dono');
create policy membros_remover on membros_casa for delete
  using (app_papel_casa(casa_id) = 'dono' or usuario_id = app_usuario_id());

alter table convites_casa enable row level security;
create policy convites_dono on convites_casa for all
  using (app_papel_casa(casa_id) = 'dono')
  with check (app_papel_casa(casa_id) = 'dono' and criado_por = app_usuario_id());

alter table entidades enable row level security;
create policy entidades_ver on entidades for select
  using (dono_id = app_usuario_id() or app_pode_ver_entidade(id));
create policy entidades_criar on entidades for insert with check (dono_id = app_usuario_id());
create policy entidades_editar on entidades for update
  using (dono_id = app_usuario_id()) with check (dono_id = app_usuario_id());
create policy entidades_apagar on entidades for delete using (dono_id = app_usuario_id());

alter table acessos_entidade enable row level security;
create policy acessos_ver on acessos_entidade for select
  using (usuario_id = app_usuario_id() or app_papel_entidade(entidade_id) = 'dono');
create policy acessos_dono on acessos_entidade for all
  using (app_papel_entidade(entidade_id) = 'dono')
  with check (app_papel_entidade(entidade_id) = 'dono' and usuario_id <> app_usuario_id());

alter table contas enable row level security;
create policy contas_ver on contas for select
  using (app_pode_ver_entidade(entidade_id)
         or (visibilidade <> 'privada' and app_eh_membro_casa(casa_id)));
create policy contas_criar on contas for insert
  with check (app_pode_editar_entidade(entidade_id)
              and (casa_id is null or app_eh_membro_casa(casa_id)));
create policy contas_editar on contas for update
  using (app_pode_editar_entidade(entidade_id))
  with check (app_pode_editar_entidade(entidade_id)
              and (casa_id is null or app_eh_membro_casa(casa_id)));
create policy contas_apagar on contas for delete using (app_pode_editar_entidade(entidade_id));

alter table transacoes enable row level security;
create policy transacoes_ver on transacoes for select
  using (app_pode_ver_entidade(entidade_id) or app_conta_compartilhada_comigo(conta_id));
create policy transacoes_criar on transacoes for insert
  with check (app_pode_editar_entidade(entidade_id));
create policy transacoes_editar on transacoes for update
  using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id));
create policy transacoes_apagar on transacoes for delete using (app_pode_editar_entidade(entidade_id));

alter table instituicoes enable row level security;
create policy instituicoes_ver on instituicoes for select using (true);

alter table regras_fiscais enable row level security;
create policy regras_fiscais_ver on regras_fiscais for select using (true);

alter table auditoria enable row level security;
create policy auditoria_ver on auditoria for select using (usuario_id = app_usuario_id());
create policy auditoria_gravar on auditoria for insert with check (true);

alter table alertas enable row level security;
create policy alertas_ver on alertas for select using (usuario_id = app_usuario_id());
create policy alertas_ler on alertas for update
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());
create policy alertas_criar on alertas for insert with check (true);

-- +goose Down
drop table if exists alertas, auditoria, sinais_vida, regras_fiscais, transacoes, contas, instituicoes,
  acessos_entidade, entidades, convites_casa, membros_casa, casas, desafios_webauthn, passkeys,
  codigos_recuperacao, dois_fatores, sessoes, usuarios cascade;
drop function if exists app_usuario_id, app_papel_casa, app_eh_membro_casa, app_papel_entidade,
  app_pode_ver_entidade, app_pode_editar_entidade, app_conta_compartilhada_comigo,
  checa_acesso_contador, checa_tipo_entidade, criar_casa, consultar_convite, aceitar_convite,
  checa_dono_casa cascade;
drop type if exists papel_casa, papel_entidade, tipo_entidade, regime_pj, tipo_conta,
  visibilidade_conta, tipo_transacao, origem_transacao;
