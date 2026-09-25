-- Open Finance: a categoria que o banco dá passa a ser usada (DECISOES D24). Para as
-- transações já importadas sem categoria ganharem a do banco, a próxima sincronização
-- relê o histórico inteiro de cada conta (a deduplicação impede repetir transações).

-- +goose Up
update contas_pluggy set ultima_sync = null;

-- +goose Down
select 1;
