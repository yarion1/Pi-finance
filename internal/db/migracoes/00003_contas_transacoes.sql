-- Fase 1 — contas, transações, importação, categorias e regras (docs/SPEC.md §2, §5).
-- Só acrescenta: dados da fase 0 continuam valendo.

-- +goose Up

create type tipo_categoria as enum ('gasto', 'receita', 'transferencia');

-- ---------------------------------------------------------------------------
-- Contas: saldo inicial (o saldo é calculado a partir dele e das transações)
-- ---------------------------------------------------------------------------
alter table contas
  add column saldo_inicial_centavos bigint not null default 0,
  add column arquivada boolean not null default false;

-- ---------------------------------------------------------------------------
-- Categorias: árvore de 2 níveis. entidade_id nulo = padrão, igual para todos.
-- ---------------------------------------------------------------------------
create table categorias (
  id           uuid primary key default gen_random_uuid(),
  entidade_id  uuid references entidades (id) on delete cascade,
  pai_id       uuid references categorias (id) on delete cascade,
  nome         text not null,
  icone        text,
  cor          text,
  tipo         tipo_categoria not null,
  ordem        smallint not null default 0
);
create index categorias_entidade on categorias (entidade_id);
create unique index categorias_nome_unico on categorias
  (coalesce(entidade_id, '00000000-0000-0000-0000-000000000000'::uuid),
   coalesce(pai_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(nome));

-- Só dois níveis, e a filha tem o tipo da mãe.
-- +goose StatementBegin
create function checa_categoria() returns trigger
language plpgsql security definer set search_path = public, pg_temp as $$
declare
  v_pai categorias%rowtype;
begin
  if new.pai_id is null then
    return new;
  end if;
  select * into v_pai from categorias where id = new.pai_id;
  if v_pai.pai_id is not null then
    raise exception 'categorias têm só dois níveis' using errcode = 'check_violation';
  end if;
  if v_pai.entidade_id is not null and v_pai.entidade_id is distinct from new.entidade_id then
    raise exception 'subcategoria de outra entidade' using errcode = 'check_violation';
  end if;
  new.tipo := v_pai.tipo;
  return new;
end
$$;
-- +goose StatementEnd
create trigger categorias_niveis before insert or update on categorias
  for each row execute function checa_categoria();

alter table transacoes
  add constraint transacoes_categoria_fk foreign key (categoria_id) references categorias (id) on delete set null;

-- ---------------------------------------------------------------------------
-- Regras de categorização: "descrição contém X" → categoria (maior prioridade primeiro)
-- ---------------------------------------------------------------------------
create table regras_categoria (
  id            uuid primary key default gen_random_uuid(),
  entidade_id   uuid not null references entidades (id) on delete cascade,
  texto         text not null check (length(trim(texto)) > 0),
  conta_id      uuid references contas (id) on delete cascade,
  categoria_id  uuid not null references categorias (id) on delete cascade,
  prioridade    integer not null default 0,
  criada_em     timestamptz not null default now()
);
create index regras_categoria_entidade on regras_categoria (entidade_id, prioridade desc);

-- ---------------------------------------------------------------------------
-- Importações: cada uma pode ser desfeita por inteiro
-- ---------------------------------------------------------------------------
create table importacoes (
  id             uuid primary key default gen_random_uuid(),
  entidade_id    uuid not null references entidades (id) on delete cascade,
  conta_id       uuid not null references contas (id) on delete cascade,
  fonte          text not null,
  arquivo        text not null,
  linhas         integer not null default 0,
  novas          integer not null default 0,
  duplicadas     integer not null default 0,
  transferencias integer not null default 0,
  criada_por     uuid references usuarios (id) on delete set null,
  criada_em      timestamptz not null default now(),
  desfeita_em    timestamptz
);
create index importacoes_entidade on importacoes (entidade_id, criada_em desc);

alter table transacoes
  add constraint transacoes_importacao_fk foreign key (importacao_id) references importacoes (id) on delete set null,
  -- conta + data + valor + descrição normalizada + ordem da linha repetida (hash)
  add column chave_dedup text;
create unique index transacoes_dedup on transacoes (conta_id, chave_dedup);
create unique index transacoes_id_externo on transacoes (conta_id, id_externo);
create index transacoes_categoria on transacoes (categoria_id);
create index transacoes_importacao on transacoes (importacao_id);

-- Mapeamentos de CSV salvos por quem importa (colunas do banco X)
create table mapeamentos_csv (
  id          uuid primary key default gen_random_uuid(),
  usuario_id  uuid not null references usuarios (id) on delete cascade,
  nome        text not null,
  config      jsonb not null,
  criado_em   timestamptz not null default now(),
  unique (usuario_id, nome)
);

-- ---------------------------------------------------------------------------
-- Saldo da conta para quem pode ver a conta, inclusive "só saldo" pela casa
-- (que não enxerga as transações).
-- +goose StatementBegin
create function app_saldo_conta(p_conta uuid, p_ate date default null) returns bigint
language sql stable security definer set search_path = public, pg_temp as $$
  select c.saldo_inicial_centavos + coalesce((
      select sum(t.valor_centavos) from transacoes t
      where t.conta_id = c.id and (p_ate is null or t.data <= p_ate)), 0)
  from contas c
  where c.id = p_conta
    and (app_pode_ver_entidade(c.entidade_id)
         or (c.visibilidade <> 'privada' and app_conta_na_minha_casa(c.entidade_id, c.casa_id)))
$$;
-- +goose StatementEnd

-- Nome de quem é a conta ("cada número mostra de quem vem"), sem expor a entidade.
-- +goose StatementBegin
create function app_dono_conta(p_conta uuid) returns text
language sql stable security definer set search_path = public, pg_temp as $$
  select u.nome
  from contas c join entidades e on e.id = c.entidade_id join usuarios u on u.id = e.dono_id
  where c.id = p_conta
    and (app_pode_ver_entidade(c.entidade_id)
         or (c.visibilidade <> 'privada' and app_conta_na_minha_casa(c.entidade_id, c.casa_id)))
$$;
-- +goose StatementEnd

-- ---------------------------------------------------------------------------
-- RLS
-- ---------------------------------------------------------------------------
alter table categorias enable row level security;
create policy categorias_ver on categorias for select
  using (entidade_id is null or app_pode_ver_entidade(entidade_id));
create policy categorias_escrever on categorias for all
  using (entidade_id is not null and app_pode_editar_entidade(entidade_id))
  with check (entidade_id is not null and app_pode_editar_entidade(entidade_id));

alter table regras_categoria enable row level security;
create policy regras_ver on regras_categoria for select using (app_pode_ver_entidade(entidade_id));
create policy regras_escrever on regras_categoria for all
  using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id));

alter table importacoes enable row level security;
create policy importacoes_ver on importacoes for select using (app_pode_ver_entidade(entidade_id));
create policy importacoes_escrever on importacoes for all
  using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id));

alter table mapeamentos_csv enable row level security;
create policy mapeamentos_dono on mapeamentos_csv for all
  using (usuario_id = app_usuario_id()) with check (usuario_id = app_usuario_id());

-- ---------------------------------------------------------------------------
-- Categorias padrão e instituições
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
do $$
declare
  arvore jsonb := '[
    ["gasto", "Moradia", "home", "#60a5fa", ["Aluguel", "Condomínio", "Energia", "Água", "Internet e telefone", "Gás", "Manutenção"]],
    ["gasto", "Alimentação", "utensils", "#f59e0b", ["Mercado", "Restaurante", "Delivery", "Padaria e café"]],
    ["gasto", "Transporte", "car", "#22d3ee", ["Combustível", "App", "Transporte público", "Estacionamento e pedágio", "Manutenção do veículo"]],
    ["gasto", "Saúde", "heart-pulse", "#f472b6", ["Farmácia", "Consultas e exames", "Plano de saúde", "Academia"]],
    ["gasto", "Educação", "graduation-cap", "#a78bfa", ["Cursos", "Livros", "Escola"]],
    ["gasto", "Lazer", "party-popper", "#fb923c", ["Streaming", "Viagem", "Passeios e eventos", "Jogos"]],
    ["gasto", "Compras", "shopping-bag", "#e879f9", ["Roupas", "Eletrônicos", "Casa e decoração", "Presentes"]],
    ["gasto", "Serviços", "wrench", "#94a3b8", ["Assinaturas", "Tarifas bancárias", "Juros e multas", "IOF"]],
    ["gasto", "Impostos", "landmark", "#fbbf24", ["DAS", "Imposto de renda", "IPTU e IPVA", "Outros impostos"]],
    ["gasto", "Pessoal", "user", "#fda4af", ["Cuidados pessoais", "Pets", "Doações"]],
    ["gasto", "Outros gastos", "circle-help", "#9ca3af", []],
    ["receita", "Salário", "wallet", "#34d399", []],
    ["receita", "Pró-labore", "briefcase", "#34d399", []],
    ["receita", "Distribuição de lucros", "piggy-bank", "#34d399", []],
    ["receita", "Freelas e serviços", "laptop", "#34d399", []],
    ["receita", "Rendimentos", "trending-up", "#34d399", []],
    ["receita", "Reembolsos e estornos", "rotate-ccw", "#34d399", []],
    ["receita", "Outras receitas", "plus-circle", "#34d399", []],
    ["transferencia", "Transferência entre contas", "arrow-left-right", "#94a3b8", []],
    ["transferencia", "Pagamento de fatura", "credit-card", "#94a3b8", []],
    ["transferencia", "Aplicação e resgate", "arrow-up-down", "#a78bfa", []]
  ]';
  item jsonb;
  filha text;
  v_id uuid;
  i int := 0;
  j int;
begin
  for item in select * from jsonb_array_elements(arvore) loop
    i := i + 1;
    insert into categorias (tipo, nome, icone, cor, ordem)
      values ((item->>0)::tipo_categoria, item->>1, item->>2, item->>3, i) returning id into v_id;
    j := 0;
    for filha in select * from jsonb_array_elements_text(item->4) loop
      j := j + 1;
      insert into categorias (tipo, nome, pai_id, ordem) values ((item->>0)::tipo_categoria, filha, v_id, j);
    end loop;
  end loop;
end
$$;
-- +goose StatementEnd

insert into instituicoes (nome, codigo_compe) values
  ('Nubank', '260'), ('Banco Inter', '077'), ('C6 Bank', '336'), ('Itaú', '341'),
  ('Banco do Brasil', '001'), ('Bradesco', '237'), ('Caixa', '104'), ('Santander', '033'),
  ('BTG Pactual', '208'), ('XP Investimentos', '102'), ('Mercado Pago', '323'), ('PicPay', '380'),
  ('Sicoob', '756'), ('Sicredi', '748'), ('Banco Original', '212'), ('Neon', '536'),
  ('PagBank', '290'), ('Wise', null), ('Nomad', null), ('Avenue', null), ('Dinheiro', null);

-- +goose Down
drop table if exists mapeamentos_csv, importacoes, regras_categoria cascade;
alter table transacoes drop constraint if exists transacoes_categoria_fk,
  drop constraint if exists transacoes_importacao_fk, drop column if exists chave_dedup;
drop table if exists categorias cascade;
drop function if exists checa_categoria, app_saldo_conta, app_dono_conta cascade;
drop type if exists tipo_categoria;
alter table contas drop column if exists saldo_inicial_centavos, drop column if exists arquivada;
delete from instituicoes;
