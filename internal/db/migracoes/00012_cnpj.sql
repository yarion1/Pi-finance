-- Fase 5 — CNPJ (docs/SPEC.md §4 e §9): clientes, receitas e notas, folha, DAS pagos e
-- distribuição de lucros; e as regras que faltavam (partilha, ISS máximo, INSS do
-- pró-labore, tabela do IRRF de 2026). Só acrescenta.

-- +goose Up
alter table entidades add column atividade text
  check (atividade in ('servicos', 'comercio', 'ambos'));

create table clientes (
  id                 uuid primary key default gen_random_uuid(),
  entidade_id        uuid not null references entidades (id) on delete cascade,
  nome               text not null check (length(nome) between 1 and 200),
  pais               char(2) not null default 'BR',
  moeda              char(3) not null default 'BRL',
  documento_cifrado  text,          -- CPF/CNPJ do tomador, cifrado (contexto: clientes.documento)
  criado_em          timestamptz not null default now(),
  unique (entidade_id, nome)
);

-- Receita do CNPJ: nota fiscal emitida ou venda sem nota (numero vazio). O valor em
-- reais é o que conta para teto, DAS e lucro; em moeda estrangeira guarda o original e
-- o câmbio do recebimento.
create table notas_fiscais (
  id                    uuid primary key default gen_random_uuid(),
  entidade_id           uuid not null references entidades (id) on delete cascade,
  cliente_id            uuid references clientes (id) on delete set null,
  numero                text,
  chave_acesso          text,          -- NFS-e nacional: evita importar o mesmo XML duas vezes
  data_emissao          date not null,
  data_recebimento      date,
  descricao             text,
  atividade             text not null default 'servicos' check (atividade in ('servicos', 'comercio')),
  valor_centavos        bigint not null check (valor_centavos >= 0),
  moeda                 char(3) not null default 'BRL',
  valor_moeda_centavos  bigint check (valor_moeda_centavos >= 0),
  taxa_cambio           numeric(24, 8),
  exportacao            boolean not null default false,
  cancelada             boolean not null default false,
  origem                text not null default 'manual',
  criada_em             timestamptz not null default now()
);
create index notas_entidade_data on notas_fiscais (entidade_id, data_emissao);
create unique index notas_chave on notas_fiscais (entidade_id, chave_acesso) where chave_acesso is not null;

create table folha (
  entidade_id        uuid not null references entidades (id) on delete cascade,
  competencia        date not null check (extract(day from competencia) = 1),
  prolabore_centavos bigint not null default 0 check (prolabore_centavos >= 0),
  salarios_centavos  bigint not null default 0 check (salarios_centavos >= 0),
  inss_centavos      bigint not null default 0 check (inss_centavos >= 0),
  irrf_centavos      bigint not null default 0 check (irrf_centavos >= 0),
  primary key (entidade_id, competencia)
);

-- DAS de cada competência (MEI ou Simples), com o que foi calculado e quando foi pago.
create table apuracoes_simples (
  entidade_id       uuid not null references entidades (id) on delete cascade,
  competencia       date not null check (extract(day from competencia) = 1),
  rbt12_centavos    bigint not null default 0,
  anexo             text not null check (anexo in ('MEI', 'III', 'V')),
  aliquota_efetiva  numeric(12, 8),
  fator_r           numeric(12, 8),
  das_centavos      bigint not null check (das_centavos >= 0),
  pago_em           date,
  primary key (entidade_id, competencia)
);

create table distribuicoes_lucro (
  id              uuid primary key default gen_random_uuid(),
  entidade_id     uuid not null references entidades (id) on delete cascade,
  beneficiario_id uuid references usuarios (id) on delete set null,
  data            date not null,
  valor_centavos  bigint not null check (valor_centavos > 0),
  irrf_centavos   bigint not null default 0 check (irrf_centavos >= 0),
  descricao       text,
  criada_em       timestamptz not null default now()
);
create index distribuicoes_entidade_data on distribuicoes_lucro (entidade_id, data);

-- +goose StatementBegin
do $$
declare
  t text;
begin
  foreach t in array array['clientes', 'notas_fiscais', 'folha', 'apuracoes_simples', 'distribuicoes_lucro'] loop
    execute format('alter table %I enable row level security', t);
    execute format('create policy %I on %I for select using (app_pode_ver_entidade(entidade_id))', t || '_ver', t);
    execute format('create policy %I on %I for all using (app_pode_editar_entidade(entidade_id)) with check (app_pode_editar_entidade(entidade_id))', t || '_escrever', t);
  end loop;
end
$$;
-- +goose StatementEnd

-- Partilha dos Anexos III e V (LC 123/2006), por faixa; o ISS efetivo é limitado a 5 % e
-- a diferença vai aos tributos federais, na proporção deles.
insert into regras_fiscais (chave, valor_json, vigente_de, fonte) values
('partilha_anexo_iii', '[
  {"irpj": "0.04", "csll": "0.035", "cofins": "0.1282", "pis": "0.0278", "cpp": "0.434", "iss": "0.335"},
  {"irpj": "0.04", "csll": "0.035", "cofins": "0.1405", "pis": "0.0305", "cpp": "0.434", "iss": "0.32"},
  {"irpj": "0.04", "csll": "0.035", "cofins": "0.1364", "pis": "0.0296", "cpp": "0.434", "iss": "0.325"},
  {"irpj": "0.04", "csll": "0.035", "cofins": "0.1364", "pis": "0.0296", "cpp": "0.434", "iss": "0.325"},
  {"irpj": "0.04", "csll": "0.035", "cofins": "0.1282", "pis": "0.0278", "cpp": "0.434", "iss": "0.335"},
  {"irpj": "0.35", "csll": "0.15", "cofins": "0.1603", "pis": "0.0347", "cpp": "0.305", "iss": "0"}
]', '2018-01-01', 'LC 123/2006, Anexo III; https://normas.receita.fazenda.gov.br/sijut2consulta/anexoOutros.action?idArquivoBinario=48432'),
('partilha_anexo_v', '[
  {"irpj": "0.25", "csll": "0.15", "cofins": "0.141", "pis": "0.0305", "cpp": "0.2885", "iss": "0.14"},
  {"irpj": "0.23", "csll": "0.15", "cofins": "0.141", "pis": "0.0305", "cpp": "0.2785", "iss": "0.17"},
  {"irpj": "0.24", "csll": "0.15", "cofins": "0.1492", "pis": "0.0323", "cpp": "0.2385", "iss": "0.19"},
  {"irpj": "0.21", "csll": "0.15", "cofins": "0.1574", "pis": "0.0341", "cpp": "0.2385", "iss": "0.21"},
  {"irpj": "0.23", "csll": "0.125", "cofins": "0.141", "pis": "0.0305", "cpp": "0.2385", "iss": "0.235"},
  {"irpj": "0.35", "csll": "0.155", "cofins": "0.1644", "pis": "0.0356", "cpp": "0.295", "iss": "0"}
]', '2018-01-01', 'LC 123/2006, Anexo V; https://normas.receita.fazenda.gov.br/sijut2consulta/anexoOutros.action?idArquivoBinario=48446'),
('iss_maximo_simples', '{"percentual": "0.05"}', '2018-01-01', 'LC 123/2006, art. 18, § 14; notas dos Anexos III a V'),
('inss_prolabore', '{"aliquota": "0.11", "teto_centavos": 815741}', '2025-01-01',
 'teto do RGPS 2025: R$ 8.157,41'),
('tabela_irrf', '{
  "faixas": [
    {"ate_centavos": 242880, "aliquota": "0", "deduzir_centavos": 0},
    {"ate_centavos": 282665, "aliquota": "0.075", "deduzir_centavos": 18216},
    {"ate_centavos": 375105, "aliquota": "0.15", "deduzir_centavos": 39416},
    {"ate_centavos": 466468, "aliquota": "0.225", "deduzir_centavos": 67549},
    {"ate_centavos": 0, "aliquota": "0.275", "deduzir_centavos": 90873}
  ],
  "desconto_simplificado_centavos": 60720,
  "dependente_centavos": 18959
}', '2025-05-01', 'MP 1.294/2025, convertida na Lei 15.191/2025; https://www.gov.br/planalto/pt-br/acompanhe-o-planalto/noticias/2025/04/nova-tabela-do-imposto-de-renda-comeca-a-valer-em-maio-veja-o-que-muda'),
('inss_prolabore', '{"aliquota": "0.11", "teto_centavos": 847555}', '2026-01-01',
 'https://www.contabilizei.com.br/contabilidade-online/inss-pro-labore/ (teto do RGPS 2026: R$ 8.475,55)'),
('tabela_irrf', '{
  "faixas": [
    {"ate_centavos": 242880, "aliquota": "0", "deduzir_centavos": 0},
    {"ate_centavos": 282665, "aliquota": "0.075", "deduzir_centavos": 18216},
    {"ate_centavos": 375105, "aliquota": "0.15", "deduzir_centavos": 39416},
    {"ate_centavos": 466468, "aliquota": "0.225", "deduzir_centavos": 67549},
    {"ate_centavos": 0, "aliquota": "0.275", "deduzir_centavos": 90873}
  ],
  "desconto_simplificado_centavos": 60720,
  "dependente_centavos": 18959,
  "reducao": {"ate_centavos": 500000, "maxima_centavos": 31289, "ate_parcial_centavos": 735000,
              "fixo_centavos": 97862, "coeficiente": "0.133145"}
}', '2026-01-01', 'Lei 15.191/2025 (tabela) e Lei 15.270/2025 (redução); https://www.gov.br/receitafederal/pt-br/assuntos/noticias/2025/dezembro/receita-federal-orienta-fontes-pagadoras-e-contribuintes-a-calcular-a-reducao-do-imposto-de-renda-a-partir-de-1o-de-janeiro-de-2026');

update regras_fiscais set vigente_ate = '2025-12-31'
  where chave in ('inss_prolabore', 'tabela_irrf') and vigente_de < '2026-01-01';

-- +goose Down
delete from regras_fiscais where chave in ('partilha_anexo_iii', 'partilha_anexo_v', 'iss_maximo_simples',
  'inss_prolabore', 'tabela_irrf');
drop table if exists distribuicoes_lucro, apuracoes_simples, folha, notas_fiscais, clientes;
alter table entidades drop column if exists atividade;
