-- Sementes fiscais vigentes em 2026 (docs/SPEC.md §4). Valores em centavos; alíquotas
-- como texto decimal para não passar por float. Nova regra = nova linha com vigente_de,
-- fechando a anterior com vigente_ate. Nunca editar valores no código.
-- Partilha, tabela do IRPF e IR regressivo entram nas fases 4 e 5.

-- +goose Up
insert into regras_fiscais (chave, valor_json, vigente_de, fonte) values
('salario_minimo', '{"centavos": 162100}', '2026-01-01',
 'https://www8.receita.fazenda.gov.br/simplesnacional/noticias/NoticiaCompleta.aspx?id=c3b2044c-ff97-432a-b33c-ecf2a3df6dc3'),

('teto_mei', '{"anual_centavos": 8100000, "mensal_proporcional_centavos": 675000, "tolerancia": "0.20"}', '2018-01-01',
 'https://www.contabilizei.com.br/contabilidade-online/faturamento-mei-2026/'),

('das_mei', '{"inss_centavos": 8105, "iss_centavos": 500, "icms_centavos": 100, "vencimento_dia": 20}', '2026-01-01',
 'https://www8.receita.fazenda.gov.br/simplesnacional/noticias/NoticiaCompleta.aspx?id=c3b2044c-ff97-432a-b33c-ecf2a3df6dc3'),

('lucro_presumido_mei', '{"servicos": "0.32", "comercio": "0.08"}', '2018-01-01',
 'docs/SPEC.md §4 — parcela isenta do lucro do MEI no IRPF'),

('teto_simples', '{"anual_centavos": 480000000}', '2018-01-01',
 'https://www.contabilizei.com.br/contabilidade-online/anexo-3-simples-nacional/'),

('sublimite_iss', '{"anual_centavos": 360000000}', '2018-01-01',
 'https://www.contabilizei.com.br/contabilidade-online/anexo-3-simples-nacional/'),

('fator_r_minimo', '{"percentual": "0.28"}', '2018-01-01',
 'https://blog.esimplesauditoria.com.br/anexo-5-do-simples-nacional/'),

('faixas_anexo_iii', '[
  {"ate_centavos": 18000000,  "aliquota": "0.06",  "deduzir_centavos": 0},
  {"ate_centavos": 36000000,  "aliquota": "0.112", "deduzir_centavos": 936000},
  {"ate_centavos": 72000000,  "aliquota": "0.135", "deduzir_centavos": 1764000},
  {"ate_centavos": 180000000, "aliquota": "0.16",  "deduzir_centavos": 3564000},
  {"ate_centavos": 360000000, "aliquota": "0.21",  "deduzir_centavos": 12564000},
  {"ate_centavos": 480000000, "aliquota": "0.33",  "deduzir_centavos": 64800000}
]', '2018-01-01', 'https://www.contabilizei.com.br/contabilidade-online/anexo-3-simples-nacional/'),

('faixas_anexo_v', '[
  {"ate_centavos": 18000000,  "aliquota": "0.155", "deduzir_centavos": 0},
  {"ate_centavos": 36000000,  "aliquota": "0.18",  "deduzir_centavos": 450000},
  {"ate_centavos": 72000000,  "aliquota": "0.195", "deduzir_centavos": 990000},
  {"ate_centavos": 180000000, "aliquota": "0.205", "deduzir_centavos": 1710000},
  {"ate_centavos": 360000000, "aliquota": "0.23",  "deduzir_centavos": 6210000},
  {"ate_centavos": 480000000, "aliquota": "0.305", "deduzir_centavos": 54000000}
]', '2018-01-01', 'https://blog.esimplesauditoria.com.br/anexo-5-do-simples-nacional/'),

('limite_dividendos_mes', '{"centavos": 5000000, "aliquota_irrf": "0.10"}', '2026-01-01',
 'https://www.demarest.com.br/receita-federal-divulga-perguntas-e-respostas-sobre-a-nova-tributacao-de-dividendos-e-altas-rendas/');

-- +goose Down
delete from regras_fiscais where chave in ('salario_minimo', 'teto_mei', 'das_mei', 'lucro_presumido_mei',
  'teto_simples', 'sublimite_iss', 'fator_r_minimo', 'faixas_anexo_iii', 'faixas_anexo_v',
  'limite_dividendos_mes');
