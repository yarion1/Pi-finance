-- Casa (fase 7): despesas divididas, acertos e metas conjuntas.

-- +goose Up

-- Uma compra dividida entre pessoas da casa. Guarda a própria cópia da descrição, data e
-- valor: quem deve vê a despesa sem ganhar acesso à conta nem à transação de quem pagou.
create table despesas_divididas (
  id              uuid primary key default gen_random_uuid(),
  casa_id         uuid not null references casas (id) on delete cascade,
  transacao_id    uuid unique references transacoes (id) on delete cascade,
  pago_por        uuid not null references usuarios (id) on delete cascade,
  descricao       text not null check (length(descricao) between 1 and 200),
  data            date not null,
  total_centavos  bigint not null check (total_centavos > 0),
  moeda           char(3) not null default 'BRL',
  modo            text not null check (modo in ('igual', 'valor', 'percentual')),
  criada_em       timestamptz not null default now()
);
create index despesas_divididas_casa on despesas_divididas (casa_id, data desc);

-- A parte de cada pessoa (inclusive a de quem pagou, que já nasce acertada).
create table divisoes (
  despesa_id      uuid not null references despesas_divididas (id) on delete cascade,
  usuario_id      uuid not null references usuarios (id) on delete cascade,
  valor_centavos  bigint not null check (valor_centavos > 0),
  acertado_em     timestamptz,
  primary key (despesa_id, usuario_id)
);
create index divisoes_usuario on divisoes (usuario_id) where acertado_em is null;

-- Acerto entre duas pessoas: quem pagou (de) a quem (para), e quanto.
create table acertos (
  id              uuid primary key default gen_random_uuid(),
  casa_id         uuid not null references casas (id) on delete cascade,
  de_usuario      uuid not null references usuarios (id) on delete cascade,
  para_usuario    uuid not null references usuarios (id) on delete cascade,
  valor_centavos  bigint not null check (valor_centavos > 0),
  criado_por      uuid not null references usuarios (id) on delete cascade,
  criado_em       timestamptz not null default now()
);

-- Metas conjuntas: a meta de uma entidade vira da casa; cada membro registra quanto pôs.
alter table metas add column casa_id uuid references casas (id) on delete cascade;
create table contribuicoes_meta (
  id              uuid primary key default gen_random_uuid(),
  meta_id         uuid not null references metas (id) on delete cascade,
  usuario_id      uuid not null references usuarios (id) on delete cascade,
  valor_centavos  bigint not null check (valor_centavos > 0),
  data            date not null,
  criada_em       timestamptz not null default now()
);
create index contribuicoes_meta_meta on contribuicoes_meta (meta_id);

-- ---------------------------------------------------------------------------
-- Funções de acesso (SECURITY DEFINER para não cair em recursão de políticas)
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
create function app_escreve_na_casa(p_casa uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select coalesce(app_papel_casa(p_casa) in ('dono', 'membro'), false)
$$;

-- a pessoa (não necessariamente a da sessão) é dono ou membro da casa?
create function app_pessoa_escreve_na_casa(p_casa uuid, p_usuario uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select exists (select 1 from membros_casa where casa_id = p_casa and usuario_id = p_usuario
                 and papel in ('dono', 'membro'))
$$;

create function app_participa_despesa(p_despesa uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select exists (select 1 from despesas_divididas where id = p_despesa and pago_por = app_usuario_id())
      or exists (select 1 from divisoes where despesa_id = p_despesa and usuario_id = app_usuario_id())
$$;

-- quem pagou pode lançar as partes, só de gente da própria casa
create function app_pode_dividir_com(p_despesa uuid, p_usuario uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select exists (select 1 from despesas_divididas d where d.id = p_despesa and d.pago_por = app_usuario_id()
                 and app_pessoa_escreve_na_casa(d.casa_id, p_usuario))
$$;

-- dono do usuário da conta (para agrupar o consolidado por pessoa), com a mesma regra de
-- quem pode ver a conta
create function app_dono_conta_id(p_conta uuid) returns uuid
language sql stable security definer set search_path = public, pg_temp as $$
  select e.dono_id
  from contas c join entidades e on e.id = c.entidade_id
  where c.id = p_conta
    and (app_pode_ver_entidade(c.entidade_id)
         or (c.visibilidade <> 'privada' and app_conta_na_minha_casa(c.entidade_id, c.casa_id)))
$$;

create function app_meta_da_minha_casa(p_meta uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select exists (select 1 from metas where id = p_meta and casa_id is not null and app_eh_membro_casa(casa_id))
$$;

create function app_pode_contribuir(p_meta uuid) returns boolean
language sql stable security definer set search_path = public, pg_temp as $$
  select exists (select 1 from metas where id = p_meta and casa_id is not null and app_escreve_na_casa(casa_id))
$$;

-- meta só vai para uma casa em que a pessoa pode escrever
create function checa_casa_meta() returns trigger
language plpgsql security definer set search_path = public, pg_temp as $$
begin
  if new.casa_id is not null and (tg_op = 'INSERT' or new.casa_id is distinct from old.casa_id)
     and not app_escreve_na_casa(new.casa_id) then
    raise exception 'sem permissão nesta casa' using errcode = 'insufficient_privilege';
  end if;
  return new;
end
$$;

-- Total que cada pessoa pagou em despesas divididas no período, fora das contas que a casa
-- já vê por inteiro (para não contar duas vezes). Só totais, para o consolidado da casa.
create function app_divididas_da_casa(p_casa uuid, p_de date, p_ate date)
returns table (usuario_id uuid, total_centavos bigint)
language sql stable security definer set search_path = public, pg_temp as $$
  select d.pago_por, sum(d.total_centavos)::bigint
  from despesas_divididas d
  left join transacoes t on t.id = d.transacao_id
  left join contas c on c.id = t.conta_id
  where app_eh_membro_casa(p_casa) and d.casa_id = p_casa and d.data between p_de and p_ate
    and not coalesce(c.casa_id = p_casa and c.visibilidade = 'compartilhada', false)
  group by d.pago_por
$$;

-- Acerta tudo o que está em aberto entre a pessoa da sessão e outra, na casa: marca as
-- partes como acertadas e registra o acerto com o saldo líquido (quem pagou a quem).
create function app_acertar(p_casa uuid, p_outro uuid)
returns table (de_usuario uuid, para_usuario uuid, valor_centavos bigint)
language plpgsql security definer set search_path = public, pg_temp as $$
declare
  v_eu uuid := app_usuario_id();
  v_me_devem bigint;
  v_devo bigint;
  v_saldo bigint;
begin
  if v_eu is null or not app_escreve_na_casa(p_casa) or v_eu = p_outro then
    raise exception 'sem permissão' using errcode = 'insufficient_privilege';
  end if;
  select coalesce(sum(v.valor_centavos), 0) into v_me_devem
    from divisoes v join despesas_divididas d on d.id = v.despesa_id
    where d.casa_id = p_casa and d.pago_por = v_eu and v.usuario_id = p_outro and v.acertado_em is null;
  select coalesce(sum(v.valor_centavos), 0) into v_devo
    from divisoes v join despesas_divididas d on d.id = v.despesa_id
    where d.casa_id = p_casa and d.pago_por = p_outro and v.usuario_id = v_eu and v.acertado_em is null;
  if v_me_devem = 0 and v_devo = 0 then
    return;
  end if;
  update divisoes v set acertado_em = now()
    from despesas_divididas d
    where d.id = v.despesa_id and d.casa_id = p_casa and v.acertado_em is null
      and ((d.pago_por = v_eu and v.usuario_id = p_outro) or (d.pago_por = p_outro and v.usuario_id = v_eu));
  v_saldo := v_me_devem - v_devo;
  if v_saldo <> 0 then
    insert into acertos (casa_id, de_usuario, para_usuario, valor_centavos, criado_por)
      values (p_casa, case when v_saldo > 0 then p_outro else v_eu end,
              case when v_saldo > 0 then v_eu else p_outro end, abs(v_saldo), v_eu);
    return query select case when v_saldo > 0 then p_outro else v_eu end,
                        case when v_saldo > 0 then v_eu else p_outro end, abs(v_saldo);
  end if;
end
$$;
-- +goose StatementEnd

create trigger metas_casa before insert or update on metas
  for each row execute function checa_casa_meta();

-- ---------------------------------------------------------------------------
-- RLS
-- ---------------------------------------------------------------------------
alter table despesas_divididas enable row level security;
create policy despesas_ver on despesas_divididas for select
  using (pago_por = app_usuario_id() or app_participa_despesa(id));
create policy despesas_criar on despesas_divididas for insert
  with check (pago_por = app_usuario_id() and app_escreve_na_casa(casa_id)
              and exists (select 1 from transacoes t where t.id = transacao_id and app_pode_editar_entidade(t.entidade_id)));
create policy despesas_apagar on despesas_divididas for delete using (pago_por = app_usuario_id());

alter table divisoes enable row level security;
create policy divisoes_ver on divisoes for select using (app_participa_despesa(despesa_id));
create policy divisoes_criar on divisoes for insert with check (app_pode_dividir_com(despesa_id, usuario_id));

alter table acertos enable row level security;
create policy acertos_ver on acertos for select
  using (de_usuario = app_usuario_id() or para_usuario = app_usuario_id());

create policy metas_casa_ver on metas for select using (casa_id is not null and app_eh_membro_casa(casa_id));

alter table contribuicoes_meta enable row level security;
create policy contribuicoes_ver on contribuicoes_meta for select using (app_meta_da_minha_casa(meta_id));
create policy contribuicoes_criar on contribuicoes_meta for insert
  with check (usuario_id = app_usuario_id() and app_pode_contribuir(meta_id));
create policy contribuicoes_apagar on contribuicoes_meta for delete using (usuario_id = app_usuario_id());

-- +goose Down
drop policy if exists metas_casa_ver on metas;
drop trigger if exists metas_casa on metas;
drop table if exists contribuicoes_meta, acertos, divisoes, despesas_divididas;
alter table metas drop column if exists casa_id;
drop function if exists app_acertar(uuid, uuid), app_divididas_da_casa(uuid, date, date), checa_casa_meta(), app_pode_contribuir(uuid),
  app_meta_da_minha_casa(uuid), app_dono_conta_id(uuid), app_pode_dividir_com(uuid, uuid),
  app_participa_despesa(uuid), app_pessoa_escreve_na_casa(uuid, uuid), app_escreve_na_casa(uuid);
