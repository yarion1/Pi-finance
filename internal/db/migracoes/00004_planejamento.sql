-- Fase 2 — planejamento: faturas e parcelas, recorrências, orçamento, metas, patrimônio
-- e agenda (docs/SPEC.md §2). Só acrescenta.

-- +goose Up

-- Transação de cartão sabe em que fatura está (data de vencimento da fatura).
alter table transacoes add column fatura_em date;
create index transacoes_fatura on transacoes (conta_id, fatura_em) where fatura_em is not null;
create index transacoes_compra on transacoes (compra_id) where compra_id is not null;

-- ---------------------------------------------------------------------------
-- Recorrências: contas fixas e assinaturas (detectadas ou cadastradas)
-- ---------------------------------------------------------------------------
create type frequencia as enum ('mensal', 'anual', 'semanal');

create table recorrencias (
  id                       uuid primary key default gen_random_uuid(),
  entidade_id              uuid not null references entidades (id) on delete cascade,
  conta_id                 uuid references contas (id) on delete set null,
  descricao                text not null,
  chave                    text not null,           -- core.ChaveHistorico da descrição
  categoria_id             uuid references categorias (id) on delete set null,
  valor_centavos           bigint not null,         -- último valor (negativo = saída)
  valor_anterior_centavos  bigint,
  frequencia               frequencia not null default 'mensal',
  dia                      smallint not null check (dia between 1 and 31),
  ultima_data              date,
  tipo                     text not null default 'conta_fixa' check (tipo in ('assinatura', 'conta_fixa', 'receita')),
  ativa                    boolean not null default true,
  detectada                boolean not null default false,
  criada_em                timestamptz not null default now(),
  unique (entidade_id, chave)
);

-- ---------------------------------------------------------------------------
-- Orçamento: limite por categoria por mês; modo por entidade
-- ---------------------------------------------------------------------------
create table orcamentos (
  id               uuid primary key default gen_random_uuid(),
  entidade_id      uuid not null references entidades (id) on delete cascade,
  mes              date not null check (extract(day from mes) = 1),
  categoria_id     uuid not null references categorias (id) on delete cascade,
  limite_centavos  bigint not null check (limite_centavos >= 0),
  unique (entidade_id, mes, categoria_id)
);

create table orcamento_config (
  entidade_id  uuid primary key references entidades (id) on delete cascade,
  -- categoria: limites por categoria; envelopes: o que sobra (ou falta) passa para o mês seguinte;
  -- 50_30_20: necessidades, desejos e poupança sobre a renda do mês
  modo         text not null default 'categoria' check (modo in ('categoria', 'envelopes', '50_30_20')),
  grupos       jsonb not null default '{}'  -- categoria_id → necessidades | desejos
);

-- ---------------------------------------------------------------------------
-- Metas
-- ---------------------------------------------------------------------------
create type tipo_meta as enum ('reserva', 'viagem', 'compra', 'aposentadoria', 'outra');

create table metas (
  id                          uuid primary key default gen_random_uuid(),
  entidade_id                 uuid not null references entidades (id) on delete cascade,
  nome                        text not null,
  tipo                        tipo_meta not null default 'outra',
  alvo_centavos               bigint not null check (alvo_centavos > 0),
  data_alvo                   date,
  contas_vinculadas           uuid[] not null default '{}',  -- o saldo delas é o progresso
  valor_manual_centavos       bigint not null default 0,     -- guardado fora das contas
  aporte_planejado_centavos   bigint,
  criada_em                   timestamptz not null default now(),
  concluida_em                timestamptz
);

-- ---------------------------------------------------------------------------
-- Patrimônio: bens (ativos fora das contas) e dívidas com tabela Price ou SAC
-- ---------------------------------------------------------------------------
create type tipo_bem as enum ('imovel', 'veiculo', 'outro');

create table bens (
  id              uuid primary key default gen_random_uuid(),
  entidade_id     uuid not null references entidades (id) on delete cascade,
  nome            text not null,
  tipo            tipo_bem not null default 'outro',
  valor_centavos  bigint not null check (valor_centavos >= 0),
  atualizado_em   date not null default current_date,
  fipe_codigo     text,
  notas           text,
  criado_em       timestamptz not null default now()
);

create table dividas (
  id                    uuid primary key default gen_random_uuid(),
  entidade_id           uuid not null references entidades (id) on delete cascade,
  nome                  text not null,
  tipo                  text not null default 'financiamento' check (tipo in ('financiamento', 'emprestimo', 'outro')),
  sistema               text not null default 'price' check (sistema in ('price', 'sac')),
  principal_centavos    bigint not null check (principal_centavos > 0),
  taxa_mensal           numeric(12, 8) not null check (taxa_mensal >= 0 and taxa_mensal < 1),
  prazo_meses           integer not null check (prazo_meses between 1 and 600),
  primeiro_vencimento   date not null,
  conta_id              uuid references contas (id) on delete set null,
  bem_id                uuid references bens (id) on delete set null,
  criada_em             timestamptz not null default now()
);

-- ---------------------------------------------------------------------------
-- Agenda: contas a pagar e a receber avulsas (as recorrentes vêm das recorrências)
-- ---------------------------------------------------------------------------
create table compromissos (
  id              uuid primary key default gen_random_uuid(),
  entidade_id     uuid not null references entidades (id) on delete cascade,
  conta_id        uuid references contas (id) on delete set null,
  descricao       text not null,
  valor_centavos  bigint not null check (valor_centavos <> 0),  -- negativo = a pagar
  vencimento      date not null,
  categoria_id    uuid references categorias (id) on delete set null,
  pago_em         date,
  criado_em       timestamptz not null default now()
);
create index compromissos_vencimento on compromissos (entidade_id, vencimento);

-- ---------------------------------------------------------------------------
-- RLS: ver quem vê a entidade; escrever quem edita
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
do $$
declare
  t text;
begin
  foreach t in array array['recorrencias', 'orcamentos', 'orcamento_config', 'metas', 'bens', 'dividas', 'compromissos'] loop
    execute format('alter table %I enable row level security', t);
    execute format('create policy %I on %I for select using (app_pode_ver_entidade(entidade_id))', t || '_ver', t);
    execute format('create policy %I on %I for all using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id))', t || '_escrever', t);
  end loop;
end
$$;
-- +goose StatementEnd

-- +goose Down
drop table if exists compromissos, dividas, bens, metas, orcamento_config, orcamentos, recorrencias cascade;
drop type if exists tipo_bem, tipo_meta, frequencia;
drop index if exists transacoes_fatura, transacoes_compra;
alter table transacoes drop column if exists fatura_em;
