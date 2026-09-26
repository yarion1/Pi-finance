-- Extras do Open Finance pelo Meu Pluggy: faturas do cartão como o banco fechou e
-- empréstimos/financiamentos (saldo devedor e parcelas). As movimentações dos
-- investimentos do banco entram em `operacoes` (origem 'pluggy').

-- +goose Up
create table faturas_banco (
  id                 uuid primary key default gen_random_uuid(),
  entidade_id        uuid not null references entidades (id) on delete cascade,
  conta_id           uuid not null references contas (id) on delete cascade,
  pluggy_id          text not null,
  vencimento         date not null,
  fechamento         date,
  total_centavos     bigint not null,
  minimo_centavos    bigint,
  encargos_centavos  bigint not null default 0,  -- juros, multa, IOF cobrados na fatura
  moeda              char(3) not null default 'BRL',
  atualizada_em      timestamptz not null default now(),
  unique (conta_id, pluggy_id)
);
create index faturas_banco_conta on faturas_banco (conta_id, vencimento desc);

create table emprestimos_banco (
  id                         uuid primary key default gen_random_uuid(),
  entidade_id                uuid not null references entidades (id) on delete cascade,
  item_id                    uuid not null references itens_pluggy (id) on delete cascade,
  pluggy_id                  text not null,
  nome                       text not null,
  modalidade                 text not null,  -- LOAN, FINANCING, INVOICE_FINANCING, UNARRANGED_ACCOUNT_OVERDRAFT
  valor_contratado_centavos  bigint,
  saldo_devedor_centavos     bigint,
  moeda                      char(3) not null default 'BRL',
  parcelas_total             integer,
  parcelas_pagas             integer,
  parcelas_restantes         integer,
  parcelas_atrasadas         integer,
  taxa                       numeric(12, 8),  -- pré-fixada, na periodicidade abaixo
  taxa_periodicidade         text,            -- MONTHLY, YEARLY
  cet                        numeric(12, 8),
  sistema                    text,            -- SAC, PRICE...
  contratado_em              date,
  vencimento_final           date,
  atualizado_em              timestamptz not null default now(),
  unique (entidade_id, pluggy_id)
);

-- +goose StatementBegin
do $$
declare
  t text;
begin
  foreach t in array array['faturas_banco', 'emprestimos_banco'] loop
    execute format('alter table %I enable row level security', t);
    execute format('create policy %I on %I for select using (app_pode_ver_entidade(entidade_id))', t || '_ver', t);
    execute format('create policy %I on %I for all using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id))', t || '_escrever', t);
  end loop;
end
$$;
-- +goose StatementEnd

-- +goose Down
drop table if exists emprestimos_banco, faturas_banco;
