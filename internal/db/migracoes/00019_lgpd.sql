-- Fase 8 — LGPD: exclusão da conta com remoção real depois de 30 dias e o histórico do
-- que foi enviado à IA (SPEC §10).

-- +goose Up
alter table usuarios add column apagar_em timestamptz; -- pedido de exclusão: some nessa data

-- "o que a IA viu": um registro por envio ao Claude (tipo e resumo, sem o conteúdo)
create table ia_envios (
  id                  uuid primary key default gen_random_uuid(),
  usuario_id          uuid not null references usuarios (id) on delete cascade,
  quando              timestamptz not null default now(),
  tipo                text not null,  -- categorizar, chat, relatorio, documento
  resumo              text not null,
  tokens_entrada      bigint not null default 0,
  tokens_saida        bigint not null default 0,
  custo_microdolares  bigint not null default 0
);
create index ia_envios_usuario on ia_envios (usuario_id, quando desc);
alter table ia_envios enable row level security;
create policy ia_envios_dono on ia_envios for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());

-- +goose StatementBegin
-- quem pediu para apagar e já passou do prazo (o worker não tem usuário na sessão)
create function app_usuarios_para_apagar() returns setof uuid
language sql stable security definer set search_path = public, pg_temp as $$
  select id from usuarios where apagar_em is not null and apagar_em <= now()
$$;

-- apaga de verdade um usuário vencido: entidades, contas, transações e o resto vão em
-- cascata. Casa em que ele era o único dono passa para o membro mais antigo; casa que
-- fica vazia some.
create function app_apagar_usuario(p_usuario uuid) returns boolean
language plpgsql volatile security definer set search_path = public, pg_temp as $$
declare
  v_casa uuid;
  v_herdeiro uuid;
begin
  if not exists (select 1 from usuarios where id = p_usuario and apagar_em is not null and apagar_em <= now()) then
    return false;
  end if;
  for v_casa in select casa_id from membros_casa where usuario_id = p_usuario and papel = 'dono' loop
    if not exists (select 1 from membros_casa where casa_id = v_casa and papel = 'dono' and usuario_id <> p_usuario) then
      select usuario_id into v_herdeiro from membros_casa
        where casa_id = v_casa and usuario_id <> p_usuario
        order by papel = 'membro' desc, entrou_em limit 1;
      if v_herdeiro is null then
        delete from casas where id = v_casa;
      else
        update membros_casa set papel = 'dono' where casa_id = v_casa and usuario_id = v_herdeiro;
      end if;
    end if;
  end loop;
  delete from usuarios where id = p_usuario;
  return true;
end
$$;
-- +goose StatementEnd

-- +goose Down
drop function if exists app_apagar_usuario(uuid), app_usuarios_para_apagar();
drop table if exists ia_envios;
alter table usuarios drop column if exists apagar_em;
