-- Fase 4 — cotações: o worker (sem usuário na sessão, o RLS não mostra ativo nenhum)
-- precisa saber quais códigos cotar. A função devolve só código e classe, sem de quem é.

-- +goose Up
-- +goose StatementBegin
create function app_codigos_para_cotar() returns table (codigo text, classe text)
language sql stable security definer set search_path = public, pg_temp as $$
  select distinct cotacao_codigo, classe::text from ativos
  where cotacao_codigo is not null and not encerrado and origem <> 'pluggy'
  order by 1
$$;
-- +goose StatementEnd

-- +goose Down
drop function if exists app_codigos_para_cotar();
