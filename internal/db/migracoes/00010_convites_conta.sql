-- Convite de conta: a pessoa cria a própria conta sem entrar na casa de quem convidou
-- (para amigos testarem; cada um com os próprios dados). Só acrescenta.

-- +goose Up
create table convites_conta (
  id          uuid primary key default gen_random_uuid(),
  token_hash  bytea not null unique,
  criado_por  uuid not null references usuarios (id) on delete cascade,
  criado_em   timestamptz not null default now(),
  expira_em   timestamptz not null,
  aceito_por  uuid references usuarios (id) on delete set null,
  aceito_em   timestamptz
);
alter table convites_conta enable row level security;
create policy convites_conta_dono on convites_conta for all
  using (criado_por = app_usuario_id()) with check (criado_por = app_usuario_id());

-- Quem abre o link ainda não tem conta: a consulta devolve só o nome de quem convidou.
-- +goose StatementBegin
create function consultar_convite_conta(p_token_hash bytea)
returns table (convidado_por text, expira_em timestamptz, valido boolean)
language sql stable security definer set search_path = public, pg_temp as $$
  select u.nome, c.expira_em, (c.aceito_em is null and c.expira_em > now())
  from convites_conta c join usuarios u on u.id = c.criado_por
  where c.token_hash = p_token_hash
$$;
-- +goose StatementEnd

-- Marca o convite como usado pelo usuário da sessão (uso único, validade conferida aqui).
-- +goose StatementBegin
create function aceitar_convite_conta(p_token_hash bytea) returns void
language plpgsql security definer set search_path = public, pg_temp as $$
declare
  v_usuario uuid := app_usuario_id();
  v_id uuid;
begin
  if v_usuario is null then
    raise exception 'sem usuário na sessão' using errcode = 'insufficient_privilege';
  end if;
  select id into v_id from convites_conta
  where token_hash = p_token_hash and aceito_em is null and expira_em > now() for update;
  if not found then
    raise exception 'convite inválido ou expirado' using errcode = 'no_data_found';
  end if;
  update convites_conta set aceito_por = v_usuario, aceito_em = now() where id = v_id;
end
$$;
-- +goose StatementEnd

-- +goose Down
drop function if exists aceitar_convite_conta(bytea);
drop function if exists consultar_convite_conta(bytea);
drop table if exists convites_conta;
