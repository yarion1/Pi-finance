# Decisões

Escolhas feitas onde a especificação (docs/SPEC.md) estava ambígua ou foi alterada.
A mais recente vale.

## D1 — Backend em Go, front em React + Vite (fase 0)

A SPEC (§8) pedia TypeScript de ponta a ponta (Next.js, Drizzle, Better Auth, pg-boss).
**Pedido do Pablo: backend em Go, front a critério.** Ficou assim:

| SPEC | Agora | Por quê |
| --- | --- | --- |
| Next.js (telas + API) | Go `net/http` (API) + React/Vite (SPA estática embutida no binário) | um binário só; menos memória no Pi; CSP estrita sem nonce |
| Drizzle ORM | pgx + SQL escrito à mão; migrações com goose | RLS e funções SQL ficam explícitas |
| Better Auth | autenticação própria: argon2id, sessões no banco, TOTP (pquerna/otp), passkeys (go-webauthn) | não existe Better Auth para Go |
| pg-boss | River (fila e cron no Postgres) | mesmo papel, em Go |
| Zod | validação no handler (Go) e Zod disponível no front | — |
| Vitest e Playwright | `go test` + Vitest (front) + Playwright (e2e) | — |
| imagens financas-web e financas-worker | uma imagem `financas:vX.Y.Z` com subcomandos `serve`, `worker`, `migrar` | um build por deploy |

Organização: `cmd/financas`, `internal/{core,db,auth,api,worker,cripto,config}`, `web/`,
`deploy/`. `internal/core` é o antigo `packages/core`; `internal/db/migracoes` + as
sementes fiscais são o `packages/db`. `packages/importers` vira `internal/importadores`
na fase 1.

## D2 — Modelo de sessão e 2FA

- Senha certa gera **sessão parcial** (30 min) que só serve para confirmar o segundo fator
  ou, se o usuário ainda não tem nenhum, para configurar o primeiro. Nada de dados.
- Segundo fator: TOTP, passkey ou código de recuperação (10 códigos, uso único).
- Login só com passkey já é completo (a passkey é multifator).
- Quem só tem passkey e entra com senha confirma com a passkey na tela seguinte.
- Sessão completa: **7 dias no celular, 12 h no navegador**, decidido pelo User-Agent,
  sem renovação automática. O token é trocado ao completar o login.
- Reautenticação (senha) vale 10 min: apagar entidade, remover passkey, gerar novos
  códigos, trocar o aplicativo autenticador.

## D3 — Cadastro

O primeiro usuário se cadastra livremente e vira dono da primeira casa que criar.
Depois disso, cadastro **só com convite** (link de 48 h, uso único). Convite de casa tem
papel membro ou leitor; dono se promove depois.

## D4 — Bloqueio progressivo

Por conta: a partir da 5ª falha seguida (senha, TOTP ou reautenticação), 15 min de
bloqueio, dobrando a cada nova falha até 24 h. Mais um limite de 10 tentativas por minuto
por IP nas rotas de login. O aviso de **login em aparelho novo** vira um registro em
`alertas` já na fase 0; o envio por Telegram e e-mail entra com as notificações (fase 7).

## D5 — Compartilhamento com a casa

- A conta aponta para uma casa (`contas.casa_id`) quando a visibilidade não é privada.
- Um membro da casa vê a conta **só se o dono da conta também estiver na casa**: quem sai
  leva as contas compartilhadas junto, sem precisar mexer nelas.
- "Só saldo": a casa vê a conta (e o saldo, a partir da fase 1), nunca as transações.
- Papel na entidade (membro, leitor, contador) é separado do papel na casa. Contador só
  em entidade PJ (garantido por gatilho). Acesso a entidade é dado por e-mail de quem já
  tem conta.

## D6 — Tabelas sem RLS

Autenticação (`usuarios`, `sessoes`, `dois_fatores`, `codigos_recuperacao`, `passkeys`,
`desafios_webauthn`), `sinais_vida` e as tabelas do River/goose. Só o código de
autenticação e o worker tocam nelas. Todas as outras têm RLS, conferido por teste.

## D7 — Chave mestra e permissões no Pi

A imagem roda sem root (uid 65532). Para ela ler a chave mestra mantendo `0600`, o dono
do arquivo é esse uid (não root, como dizia a SPEC):
`sudo chown 65532:65532 /etc/financas/master.key && sudo chmod 600 /etc/financas/master.key`.
O `.env` fica `0640` com grupo `docker`, para o runner ler.

## D8 — Backup na fase 0

Já existe um serviço de backup simples (`pg_dump` a cada 6 h, guarda 28) e o deploy faz
dump antes de migrar. Restic, cópia na nuvem e teste de restore semanal são a fase 8.
O `/api/health` diz `backup=pendente` até o primeiro dump.

## D9 — /api/health

Responde 200 quando banco e worker estão de pé; 503 caso contrário (é o que o deploy
confere). `pluggy` segue a D23 (não derruba o 200). `FORCAR_FALHA_HEALTH=1`
força 503 — é assim que o deploy quebrado de propósito é testado.

## D10 — Regras fiscais semeadas

Entraram só os valores citados na SPEC §4 (teto do MEI, DAS-MEI 2026, salário mínimo,
teto do Simples, sublimite, Fator R, faixas dos Anexos III e V, lucro presumido do MEI,
limite de dividendos). Partilha, tabela do IRPF e IR regressivo entram nas fases 4 e 5,
com fonte, para não inventar números.

## D11 — Acesso pelo Cloudflare Tunnel em vez do Tailscale

A SPEC (§10, §11) pedia só LAN e tailnet, sem Cloudflare Tunnel. **Pedido do Pablo:
publicar pelo Cloudflare Tunnel, como o site dos Pebas**, para ninguém precisar instalar
app. Condições para compensar a exposição na internet:

- **Cloudflare Access obrigatório** na frente do hostname (e-mails autorizados, código por
  e-mail). O login do Finanças (senha + 2FA obrigatório, bloqueio progressivo) fica atrás.
- `PUBLIC_URL` passa a ser o hostname público (`https://financas.<domínio>`): passkeys e a
  checagem de `Origin` usam uma origem só, então o endereço do Tailscale deixa de valer.
- O IP do visitante vem de `CF-Connecting-IP`, aceito só de proxy confiável (cloudflared).
- Custo aceito: a Cloudflare termina o TLS e vê o tráfego na borda dela.

## D12 — Modo rede de casa (http) como alternativa

Com `PUBLIC_URL=http://192.168.x.x:3100` e `ESCUTAR=0.0.0.0` no `.env`, o painel abre direto
pelo Wi-Fi de casa, sem túnel. O servidor se adapta à URL: sem HTTPS, o cookie perde
`Secure` e o prefixo `__Host-` (o navegador descartaria), não há HSTS e as passkeys ficam
desligadas (exigem HTTPS e nome de domínio); o 2FA fica só com o aplicativo autenticador.
A checagem de `Origin`, o RLS e o resto não mudam. Com `PUBLIC_URL` em HTTPS (túnel,
D11) tudo volta ao modo seguro automaticamente.

## D13 — Transferência interna só dentro da mesma entidade (fase 1)

O pareamento automático (mesmo valor com sinal oposto, contas diferentes, até 2 dias) só
acontece entre contas da **mesma entidade**. Dinheiro que sai do CNPJ para a PF não é
"transferência": é pró-labore, lucro ou reembolso (SPEC §4) e será classificado na fase 5.
Escolher uma categoria de gasto ou receita numa ponta desfaz o par.

## D14 — Deduplicação e estorno (fase 1)

- Chave: hash de data + valor + descrição normalizada (minúsculas, sem acento, espaços
  simples) + ordem entre linhas idênticas do mesmo arquivo; e, à parte, o id externo (FITID
  do OFX, identificador do CSV do Nubank). Qualquer um que já exista na conta marca a linha
  como repetida. Transações manuais não têm chave e nunca são deduplicadas.
- Tipo vem do sinal (saída = gasto, entrada = receita). Categoria de transferência marca
  transferência; categoria de gasto numa entrada vira **estorno** (abate o gasto da
  categoria); categoria de receita numa saída é recusada.

## D15 — Arquivo de importação em JSON (fase 1)

Para manter a regra "toda escrita é JSON com Origin conferida" (proteção CSRF), o arquivo
vai em base64 dentro do JSON, até 8 MB. A importação roda na própria requisição (extratos
são pequenos); a fila do worker entra com o Open Finance (fase 3).

## D16 — Cartão e parcelas (fase 2)

- A fatura de cada compra sai só dos dias de fechamento e vencimento da conta
  (`core.FaturaDaCompra`): compra **no dia do fechamento ou depois** cai na fatura seguinte,
  por isso o "melhor dia de compra" mostrado é o próprio dia do fechamento. O vencimento é no
  mesmo mês do fechamento se o dia for maior; senão, no mês seguinte. A fatura fica gravada
  em `transacoes.fatura_em` e é recalculada quando os dias do cartão mudam.
- Compra parcelada lançada à mão (2 a 72×) grava **uma transação por parcela**, a k-ésima
  datada `compra + (k−1) meses`, descrição "Loja (k/n)" e o mesmo `compra_id`. A sobra dos
  centavos vai para as primeiras parcelas. Assim cada fatura soma sozinha, sem tabela à parte.
- Parcela importada ("Parcela 3/12", "PARC 03/10", "(1/12)", "Parcela 2 de 6") projeta as
  que faltam a partir da última importada; as projeções aparecem como "prevista" e somem
  quando a parcela real chega no extrato.
- Para o limite e o patrimônio, a dívida do cartão inclui as parcelas futuras já lançadas
  (é compromisso assumido). Saldo abaixo de zero em conta que não é cartão conta como
  "contas no negativo" (cheque especial), separado dos cartões.
- "Últimas transações" e "sem categoria" no Início ignoram as parcelas com data futura.

## D17 — Recorrências detectadas (fase 2)

- Mensal: a mesma descrição (chave do histórico) em **3 meses diferentes ou mais**, com
  intervalos de 25 a 35 dias e valores até 15 % da mediana. Anual: intervalo de 350 a 380 dias.
- "Subiu" quando o último valor passa o anterior em 5 % ou mais; gera o alerta
  `recorrencia_subiu`. Assinatura nova gera `assinatura_detectada`. Os alertas vão para quem
  estava na sessão ao importar (os alertas são por usuário).
- A detecção roda ao fim de cada importação e no botão "Procurar no histórico". Só as
  detectadas ficam "atrasadas" quando a cobrança esperada não aparece (sinal de
  cancelamento). Para parar de acompanhar, desativa-se: apagar uma detectada faz ela voltar
  na próxima detecção.

## D18 — Agenda e saldo projetado (fase 2)

- Saldo inicial: contas corrente, poupança, carteira, dinheiro e benefício, na data de hoje
  (fuso de São Paulo). Investimentos ficam de fora (não pagam boleto).
- Entram: recorrências (as do cartão aparecem, mas marcadas "no cartão" e sem mexer no saldo,
  porque já estão na fatura), faturas a pagar, parcelas de dívidas, contas a pagar e receber
  avulsas (atrasadas se venceram sem pagamento) e lançamentos futuros fora do cartão.
- Custo de vida: média dos gastos de 3, 6 e 12 **meses fechados**; a reserva conta os meses
  pelo de 6. "Comprometido" soma, por mês, parcelas, prestações, contas fixas e avulsas.

## D19 — Orçamento, metas e patrimônio: o que ficou simples (fase 2)

- Orçamento por entidade (a PF por padrão). Ritmo ideal = limite × dia ÷ dias do mês;
  "acima do ritmo" quando o gasto passa disso sem estourar o limite. Nos envelopes, a sobra
  (ou o estouro) de cada mês desde o primeiro limite passa para o seguinte. Na 50/30/20, as
  categorias mães têm um grupo padrão (Alimentação, Lazer, Compras, Pessoal e Outros =
  desejo, com Mercado e Farmácia como necessidade; o resto é necessidade), ajustável.
- Metas por entidade: o valor atual é o saldo das contas vinculadas mais um valor manual.
  Metas **conjuntas da casa**, com contribuição por membro, ficam para a fase 7 (casa e
  despesas divididas).
- Veículo guarda o código FIPE, mas a atualização automática do valor fica para quando os
  conectores externos entrarem (fase 3 em diante); até lá o valor é manual.
- Taxa de dívida: digitada em % ao mês na tela, guardada como fração em `numeric(12,8)`,
  convertida sem ponto flutuante.

## D20 — Open Finance pelo Meu Pluggy (fase 3)

- **Uma conexão por pessoa**: Client ID e Client Secret do Meu Pluggy dela, os dois cifrados
  (`cripto.Cifrador`, contexto = nome da coluna); a tela só mostra os 4 últimos caracteres do
  Client ID e nunca o segredo. As tabelas `conexoes_pluggy`, `itens_pluggy` e `contas_pluggy`
  têm RLS por `usuario_id`: nem a casa nem quem tem acesso à entidade veem as credenciais ou
  os bancos conectados. Cadastrar, trocar e apagar as credenciais pedem reautenticação.
- As credenciais são conferidas na Pluggy antes de gravar; o **item** (banco conectado no Meu
  Pluggy) é conferido com as credenciais da própria pessoa, então o item de outra pessoa é
  recusado. Cada item diz para qual entidade vão as contas dele.
- Nenhuma conta da Pluggy importa nada sozinha: a pessoa decide se **liga a uma conta que já
  existe** (da mesma entidade e do mesmo tipo), **cria uma nova** ou **ignora**. Conta criada
  pela Pluggy tem o saldo inicial acertado com o do banco na primeira sincronização.
- Transações pela `v2/transactions` (a v1 é desligada em 31/12/2026), paginadas por cursor.
  Pendentes ficam de fora até serem confirmadas. Sinal: numa conta, `DEBIT` sai e `CREDIT`
  entra; num cartão a Pluggy manda a compra positiva e o pagamento negativo. Valores viram
  centavos sem float. Parcela de cartão ganha "(n/N)" e a data do mês da parcela (D16).
- Janela: 365 dias na primeira leitura de cada conta; depois, desde a última sincronização
  menos 10 dias. Sincronização sem nada novo não entra no histórico de importações.
- **Quando sincroniza**: o worker roda de hora em hora e sincroniza quem está há mais de
  20 h sem sincronizar (o Meu Pluggy atualiza os bancos a cada 24 h); o botão "Sincronizar
  agora" roda na hora, com trava de 10 s. O erro de um banco (login pedido de novo, banco
  fora) fica gravado no item e aparece na tela; não impede os outros bancos.
- Investimentos que a Pluggy também traz ficam para a fase 4.

## D21 — Arquivo e Open Finance na mesma conta viram uma transação só (fase 3)

A descrição do OFX e a da Pluggy quase nunca são iguais, então a chave de deduplicação não
basta. Uma linha nova **casa** com uma transação que já está na mesma conta, veio pela outra
fonte e ainda não tem par: **mesmo valor e até 1 dia de diferença** (a mais próxima primeiro,
uma para uma). Linha da Pluggy casa com o que veio de arquivo ou foi lançado à mão; linha de
arquivo casa com o que veio da Pluggy. A existente guarda o id da nova fonte (`id_pluggy`,
ou a chave e o FITID do arquivo), então as próximas importações reconhecem direto. O id da
Pluggy fica numa coluna própria para não disputar lugar com o FITID.

## D22 — Conciliação de saldo (fase 3)

- A cada sincronização, para cada **conta** ligada (corrente, poupança), grava-se o saldo do
  banco e o calculado até hoje (`conciliacoes`, um registro por conta por dia). Se diferem,
  vira o alerta `saldo_divergente` para quem sincronizou, sem repetir a mesma diferença no
  mesmo dia.
- **Cartão fica fora** da comparação: o saldo que a Pluggy dá (fatura) e o do painel
  (compras lançadas, com parcelas futuras) medem coisas diferentes. O saldo do banco do
  cartão aparece na tela de Open Finance.
- A conta mostra o saldo do banco quando difere e o botão **Acertar pelo banco**, que muda o
  saldo inicial para fechar a diferença (útil quando o histórico importado começa no meio).

## D23 — /api/health com o Open Finance (fase 3)

`pluggy` é `ok` sem conexões ou com todas em dia, `erro` se alguma credencial ou banco
falhou na última sincronização e `atrasada` se a última passou de 36 h. `last_sync` é a
sincronização mais recente de qualquer pessoa. Nenhum dos dois derruba o 200 (o deploy só
depende de banco e worker); é informação para o monitor do PiControl. A leitura usa uma
função `SECURITY DEFINER` que devolve só contagens e datas, nunca credenciais.
