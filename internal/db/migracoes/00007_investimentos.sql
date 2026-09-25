-- Fase 4 — investimentos (docs/SPEC.md §3): ativos, operações, eventos, alocação alvo,
-- cotações e índices, e as regras de IR com vigência. Só acrescenta.

-- +goose Up
create type classe_ativo as enum ('acao', 'fii', 'etf', 'bdr', 'renda_fixa', 'tesouro', 'fundo', 'cripto',
  'exterior', 'previdencia', 'outro');
create type tipo_operacao as enum ('compra', 'venda', 'provento', 'juros', 'amortizacao');
create type tipo_evento as enum ('desdobramento', 'grupamento', 'bonificacao', 'ajuste');

-- Ativos são por entidade: o mesmo ticker em duas pessoas são dois ativos (cada um com
-- as suas operações); a cotação é global, pelo código de cotação.
create table ativos (
  id              uuid primary key default gen_random_uuid(),
  entidade_id     uuid not null references entidades (id) on delete cascade,
  codigo          text not null,                 -- PETR4, "CDB Banco X 110% CDI 2028", bitcoin
  nome            text,
  classe          classe_ativo not null,
  emissor         text,
  indexador       text check (indexador in ('prefixado', 'cdi', 'ipca')),
  taxa            numeric(12, 8),                -- prefixado/IPCA+: a.a. (0.12); CDI: percentual (1.10)
  vencimento      date,
  moeda           char(3) not null default 'BRL',
  isento_ir       boolean not null default false,
  cotacao_codigo  text,                          -- código na fonte de cotação (vazio = sem cotação)
  criado_em       timestamptz not null default now(),
  unique (entidade_id, codigo)
);

create table operacoes (
  id                  uuid primary key default gen_random_uuid(),
  entidade_id         uuid not null references entidades (id) on delete cascade,
  ativo_id            uuid not null references ativos (id) on delete cascade,
  conta_id            uuid references contas (id) on delete set null,  -- corretora
  data                date not null,
  tipo                tipo_operacao not null,
  quantidade          numeric(24, 8) not null default 0,
  preco               numeric(24, 8) not null default 0,
  valor_centavos      bigint not null default 0,   -- provento, juros, amortização
  taxas_centavos      bigint not null default 0,
  ir_retido_centavos  bigint not null default 0,
  descricao           text,
  origem              text not null default 'manual',
  chave_dedup         text,
  criada_em           timestamptz not null default now()
);
create index operacoes_ativo on operacoes (ativo_id, data);
create unique index operacoes_dedup on operacoes (entidade_id, chave_dedup);

create table eventos_corporativos (
  id              uuid primary key default gen_random_uuid(),
  entidade_id     uuid not null references entidades (id) on delete cascade,
  ativo_id        uuid not null references ativos (id) on delete cascade,
  data            date not null,
  tipo            tipo_evento not null,
  fator           numeric(24, 8) not null,        -- desdobramento/grupamento/bonificação: multiplica; ajuste: soma
  custo_unitario  numeric(24, 8) not null default 0,
  chave_dedup     text,
  criado_em       timestamptz not null default now()
);
create unique index eventos_dedup on eventos_corporativos (entidade_id, chave_dedup);

create table alocacao_alvo (
  id           uuid primary key default gen_random_uuid(),
  entidade_id  uuid not null references entidades (id) on delete cascade,
  dimensao     text not null default 'classe',
  chave        text not null,
  percentual   numeric(7, 4) not null check (percentual >= 0 and percentual <= 100),
  unique (entidade_id, dimensao, chave)
);

-- ---------------------------------------------------------------------------
-- Referência global: cotações e índices. Todos leem; só o worker (sem usuário na
-- sessão) grava.
-- ---------------------------------------------------------------------------
create table cotacoes (
  codigo  text not null,
  data    date not null,
  preco   numeric(24, 8) not null,
  moeda   char(3) not null default 'BRL',
  fonte   text not null,
  primary key (codigo, data)
);

create table indices (
  serie  text not null,     -- cdi (a.d.), ipca (a.m.), usd_brl
  data   date not null,
  valor  numeric(24, 12) not null,
  primary key (serie, data)
);

-- +goose StatementBegin
do $$
declare
  t text;
begin
  foreach t in array array['ativos', 'operacoes', 'eventos_corporativos', 'alocacao_alvo'] loop
    execute format('alter table %I enable row level security', t);
    execute format('create policy %I on %I for select using (app_pode_ver_entidade(entidade_id))', t || '_ver', t);
    execute format('create policy %I on %I for all using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id))', t || '_escrever', t);
  end loop;
  foreach t in array array['cotacoes', 'indices'] loop
    execute format('alter table %I enable row level security', t);
    execute format('create policy %I on %I for select using (true)', t || '_ler', t);
    execute format('create policy %I on %I for all using (app_usuario_id() is null) with check (app_usuario_id() is null)', t || '_worker', t);
  end loop;
end
$$;
-- +goose StatementEnd

-- Regras de IR dos investimentos, vigentes em 2026 (a MP 1.303/2025 caducou em
-- 08/10/2025 e as regras anteriores continuam valendo).
insert into regras_fiscais (chave, valor_json, vigente_de, fonte) values
('ir_renda_variavel', '{"comum": "0.15", "day_trade": "0.20", "fii": "0.20",
  "isencao_vendas_acoes_centavos": 2000000, "darf_minimo_centavos": 1000, "codigo_darf": "6015"}', '2005-01-01',
 'Lei 11.033/2004 e IN RFB 1.585/2015; https://www.camara.leg.br/noticias/1209479-MP-SOBRE-TRIBUTACAO-DE-INVESTIMENTOS-E-RETIRADA-DE-PAUTA-E-PERDE-A-VALIDADE'),
('ir_regressivo_renda_fixa', '[{"ate_dias": 180, "aliquota": "0.225"}, {"ate_dias": 360, "aliquota": "0.20"},
  {"ate_dias": 720, "aliquota": "0.175"}, {"ate_dias": 0, "aliquota": "0.15"}]', '2005-01-01',
 'Lei 11.033/2004, art. 1º');

-- +goose Down
delete from regras_fiscais where chave in ('ir_renda_variavel', 'ir_regressivo_renda_fixa');
drop table if exists indices, cotacoes, alocacao_alvo, eventos_corporativos, operacoes, ativos cascade;
drop type if exists tipo_evento, tipo_operacao, classe_ativo;
