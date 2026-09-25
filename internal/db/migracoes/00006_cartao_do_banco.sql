-- Cartão pelo Open Finance: o limite disponível informado pelo banco (o usado sai
-- dele, já contando parcelas futuras). Só acrescenta.

-- +goose Up
alter table contas_pluggy add column disponivel_centavos bigint;

-- +goose Down
alter table contas_pluggy drop column if exists disponivel_centavos;
