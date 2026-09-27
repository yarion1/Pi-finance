-- Notificações (fase 7): push no aparelho (PWA) e Telegram, por tipo, por pessoa.

-- +goose Up
create table notificacoes_config (
  usuario_id              uuid primary key references usuarios (id) on delete cascade,
  tipos                   jsonb not null default '{}',   -- {"alertas": {"push": true, "telegram": true}, ...}
  mostrar_valores         boolean not null default false, -- valores na tela bloqueada: desligado por padrão
  telegram_chat_cifrado   text,                           -- chat_id (cripto, contexto notificacoes_config.telegram_chat)
  telegram_codigo_hash    text,                           -- sha256 do código do link t.me/bot?start=
  telegram_codigo_expira  timestamptz,
  atualizada_em           timestamptz not null default now()
);

create table push_inscricoes (
  id                  uuid primary key default gen_random_uuid(),
  usuario_id          uuid not null references usuarios (id) on delete cascade,
  endpoint_hash       text not null unique,  -- sha256 do endereço (o endereço fica cifrado)
  inscricao_cifrada   text not null,         -- JSON {endpoint, p256dh, auth} (contexto push_inscricoes.inscricao)
  aparelho            text not null default '',
  criada_em           timestamptz not null default now(),
  usada_em            timestamptz
);
create index push_inscricoes_usuario on push_inscricoes (usuario_id);

-- lembretes do dia (agenda) já enviados, para não repetir
create table notificacoes_enviadas (
  usuario_id  uuid not null references usuarios (id) on delete cascade,
  chave       text not null,
  enviada_em  timestamptz not null default now(),
  primary key (usuario_id, chave)
);

alter table alertas add column notificado_em timestamptz;
-- os alertas que já existiam não viram notificação de uma vez
update alertas set notificado_em = now();

-- estado do servidor que o worker guarda (ex.: até onde leu as mensagens do bot)
create table estado_servidor (
  chave  text primary key,
  valor  text not null
);

alter table notificacoes_config enable row level security;
create policy notificacoes_config_dono on notificacoes_config for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());
alter table push_inscricoes enable row level security;
create policy push_inscricoes_dono on push_inscricoes for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());
alter table notificacoes_enviadas enable row level security;
create policy notificacoes_enviadas_dono on notificacoes_enviadas for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());
alter table estado_servidor enable row level security;
create policy estado_servidor_worker on estado_servidor for all
  using (app_usuario_id() is null) with check (app_usuario_id() is null);

-- o worker (sem usuário na sessão) precisa achar o que notificar e de quem
-- +goose StatementBegin
create function app_alertas_para_notificar() returns table (id uuid, usuario_id uuid)
language sql stable security definer set search_path = public, pg_temp as $$
  select a.id, a.usuario_id from alertas a
  where a.notificado_em is null and a.lido_em is null and a.criado_em > now() - interval '2 days'
  order by a.criado_em limit 200
$$;

create function app_usuarios_notificaveis() returns setof uuid
language sql stable security definer set search_path = public, pg_temp as $$
  select usuario_id from push_inscricoes
  union
  select usuario_id from notificacoes_config where telegram_chat_cifrado is not null
$$;

-- vincula o chat pelo código de uso único; devolve o usuário (ou nada)
create function app_vincular_telegram(p_codigo_hash text, p_chat_cifrado text) returns uuid
language sql volatile security definer set search_path = public, pg_temp as $$
  update notificacoes_config set telegram_chat_cifrado = p_chat_cifrado, telegram_codigo_hash = null,
    telegram_codigo_expira = null, atualizada_em = now()
  where telegram_codigo_hash = p_codigo_hash and telegram_codigo_expira > now()
  returning usuario_id
$$;
-- +goose StatementEnd

-- +goose Down
drop function if exists app_vincular_telegram(text, text);
drop function if exists app_usuarios_notificaveis();
drop function if exists app_alertas_para_notificar();
drop table if exists estado_servidor, notificacoes_enviadas, push_inscricoes, notificacoes_config;
alter table alertas drop column if exists notificado_em;
