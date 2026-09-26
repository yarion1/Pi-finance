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
- De onde vem cada coisa: os bancos são conectados no **meu.pluggy.ai**; o Client ID e o
  Secret são da aplicação de desenvolvimento no **dashboard.pluggy.ai** (a conta de
  desenvolvedor continua lendo depois do teste de 15 dias); o **item** nasce no Dashboard, ao
  conectar pela Demo com o conector **MeuPluggy** (um por banco), que funciona como procuração
  para a conexão do Meu Pluggy.
- As credenciais são conferidas na Pluggy antes de gravar; o **item** é conferido com as
  credenciais da própria pessoa, então o item de outra pessoa é recusado. Cada item diz para
  qual entidade vão as contas dele.
- Nenhuma conta da Pluggy importa nada sozinha: a pessoa decide se **liga a uma conta que já
  existe** (da mesma entidade e do mesmo tipo), **cria uma nova** ou **ignora**. Conta criada
  pela Pluggy tem o saldo inicial acertado com o do banco na primeira sincronização.
- Transações pela `v2/transactions` (a v1 é desligada em 31/12/2026), paginadas por cursor, só
  com `accountId` e `dateFrom`; se a Pluggy recusar a v2, cai para a v1 (páginas numeradas).
  Erro 400 mostra a mensagem da Pluggy na tela.
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
- No cartão ligado, o **limite usado** mostrado é o do banco (limite − disponível, que já
  conta as parcelas futuras); o limite vem do banco a cada sincronização e os dias de
  fechamento e vencimento, se a conta ainda não tiver, antes de importar (para cada compra
  cair na fatura certa).
- A conta mostra o saldo do banco quando difere e o botão **Acertar pelo banco**, que muda o
  saldo inicial para fechar a diferença (útil quando o histórico importado começa no meio).

## D23 — /api/health com o Open Finance (fase 3)

`pluggy` é `ok` sem conexões ou com todas em dia, `erro` se alguma credencial ou banco
falhou na última sincronização e `atrasada` se a última passou de 36 h. `last_sync` é a
sincronização mais recente de qualquer pessoa. Nenhum dos dois derruba o 200 (o deploy só
depende de banco e worker); é informação para o monitor do PiControl. A leitura usa uma
função `SECURITY DEFINER` que devolve só contagens e datas, nunca credenciais.

## D24 — Categoria que o banco dá (Open Finance)

A Pluggy categoriza cada transação ("Eating out", "Groceries", com um id de 8 dígitos
cuja família são os 2 primeiros). `core.CategoriaPluggy` traduz pelo nome (inglês ou
português) e, sem casar, pela família, para as categorias padrão do painel. Ordem na
importação: **regra da pessoa → histórico → categoria do banco**. Transferência para
terceiros (Pix, TED), empréstimos e "outros" ficam sem sugestão. Pagamento de fatura e
transferência para si mesmo entram como **transferência** (fora do gasto). Transações que
já estavam no painel sem categoria recebem a do banco quando voltam numa sincronização; a
migração 00008 faz a próxima sincronização reler o histórico uma vez para completar.

## D25 — Visual inspirado no Meu Pluggy

Fundo quase preto, cartões um tom acima, destaque rosa nos ícones e gráficos; cada bloco
da visão geral tem cabeçalho com ícone e rótulo em maiúsculas e o número grande. O Início
abre com contas bancárias por banco (cores de marca em siglas, sem imagens de terceiros por
causa da CSP), cartões com o % do limite usado segundo o banco, investimentos e a evolução
do patrimônio em SVG próprio (ECharts continua previsto para a fase 7). O tema claro
acompanha com o mesmo destaque.

## D26 — Investimentos pelo Open Finance

Cada investimento que a Pluggy devolve (CDB, LCI, Tesouro, fundos, previdência, ações) vira
um **ativo** da entidade do banco conectado, com o **saldo informado pela instituição** e o
valor aplicado (para o rendimento). A SPEC pede a carteira a partir das operações: isso vale
para os ativos manuais e os da B3 (posição × última cotação); para os do banco, o valor da
instituição é mais fiel do que uma curva estimada, e as operações dele entram depois se
fizerem falta. Resgatados por inteiro ficam como **encerrados**. LCI, LCA, CRI e CRA são
marcados como isentos de IR. A carteira entra no patrimônio (além das contas do tipo
investimento); a série histórica do patrimônio ainda não tem a carteira dos meses passados.

## D27 — Convite de conta para amigos

A SPEC fala em "várias pessoas da mesma casa". **Pedido do Pablo: amigos também vão usar**,
cada um com os próprios dados. Além do convite de casa (entra na casa com um papel), existe
o **convite de conta**: link de uso único, 48 h, guardado só como hash, que deixa a pessoa
se cadastrar sem entrar em casa nenhuma (ela ganha só a entidade PF dela). O cadastro
continua fechado para quem não tem convite, e o Cloudflare Access (D11) precisa liberar o
e-mail do amigo antes. Cada um conecta o próprio Meu Pluggy (credenciais cifradas por
usuário); o painel não compartilha as credenciais de ninguém.

## D28 — Cotações e benchmarks da fase 4

- **Fontes**: brapi (ações, FIIs, ETFs, BDRs; uma requisição por ticker, como o plano
  gratuito pede, com `BRAPI_TOKEN` opcional no `.env`), CoinGecko (cripto, pelo id:
  `bitcoin`) e o SGS do Banco Central (CDI série 12, IPCA 433, PTAX 1). O worker roda a
  cada 6 h; só o código do ativo sai do Pi (a função `app_codigos_para_cotar` devolve
  código e classe, sem dono). Falha de uma fonte fica no log e no sinal de vida
  `cotacoes`, sem parar as outras.
- **Tesouro Direto e CDB sem cotação** valem **na curva** (taxa contratada com o CDI e o
  IPCA do Banco Central), antes do IR. O CSV de preços do Tesouro Transparente (dezenas de
  MB) fica para depois; o Tesouro do Open Finance já vem com o valor do banco.
- **Rentabilidade** mostrada é o **XIRR ao ano** (ativo e carteira, com proventos e vendas),
  comparado ao CDI e ao IPCA acumulados no mesmo período e anualizados na mesma convenção
  (dias corridos/365). Só aparece com 30 dias ou mais de história. Os ativos do Open
  Finance ficam fora (não têm operações). O TWR já está no `core` e entra com o gráfico da
  cota na fase 7; o Ibovespa entra junto (a brapi pede token para índices).
- **Critério de aceite**: `TestRentabilidadeContraCDI` (CDB a 100 % do CDI rende o CDI do
  período até 0,01 p.p. ao ano) e a planilha de referência de `core/testdata`.
- A alocação alvo (tabela `alocacao_alvo`) ganha tela na fase 7, com os gráficos.

## D29 — Túnel da Cloudflare dentro do compose (igual ao fitness-hub)

O `cloudflared` roda como serviço do compose (perfil `tunel`), com o token de um túnel
próprio (`financas`) em `CLOUDFLARE_TUNNEL_TOKEN` no `/etc/financas/.env`, como o
fitness-hub faz. O hostname público (`financas.pebasrunners.com`) aponta para
`http://web:3100` pela rede do compose; nenhuma porta nova abre no Pi e a 3100 continua só
em 127.0.0.1 para o deploy conferir o `/api/health`. Sem token, nada muda. Com token, o
deploy exige `PUBLIC_URL` em https. O Cloudflare Access (D11) continua obrigatório na
frente; o IP do visitante vem de `CF-Connecting-IP`, aceito porque o cloudflared está na
rede do Docker (172.16.0.0/12, em `PROXIES_CONFIAVEIS`).

## D30 — Fase 5: CNPJ

- **Receita = notas** (`notas_fiscais`), com ou sem número: cobre NFS-e emitida e venda
  sem nota do MEI. Conta pela data de emissão (competência), que é o que vale para o teto
  do MEI e para o DAS do Simples. Nota cancelada sai de tudo.
- **XML da NFS-e nacional** (Emissor Nacional): lidos só número, chave, datas, tomador,
  descrição e valor; o XML não é guardado (tem CPF/CNPJ do tomador, que vai cifrado para
  `clientes.documento_cifrado`). Tomador com país diferente de BR ou `comExt` marca
  exportação — a tela avisa para confirmar com o contador (SPEC §4).
- **Moeda estrangeira**: valor em reais = valor na moeda × câmbio informado; sem câmbio,
  em dólar, usa a PTAX guardada pelo worker (até 7 dias antes).
- **DAS do Simples**: receita × (RBT12 × nominal − deduzir) ÷ RBT12 em frações exatas,
  arredondado ao centavo no total; a partilha soma exatamente o total (maior resto). ISS
  efetivo limitado a 5 % com a sobra repartida entre os federais. RBT12 proporcional no
  início de atividade (LC 123, art. 18, §§ 2º e 3º). Anexo pelo Fator R (folha dos 12
  meses anteriores ÷ RBT12, mínimo 28 %). Critério de aceite: `TestDASDozeCasos` e
  `TestExemploDaSPEC` (core) e `TestSimplesExemploDaSPEC` (API).
- **MEI**: teto proporcional no ano de abertura; faixas 70/90/100/120 %; data prevista
  para bater o teto pelo ritmo diário desde 1º de janeiro (ou da abertura); DAS-MEI de
  cada competência desde a abertura (sem data de abertura, desde janeiro); lucro isento
  = 32 % de serviços + 8 % de comércio, limitado ao lucro (receita − gastos lançados nas
  contas do CNPJ).
- **IRRF 2026** do pró-labore: tabela da Lei 15.191/2025 com a redução da Lei 15.270/2025
  (até R$ 5 mil zera; até R$ 7.350, redução decrescente); desconto simplificado quando é
  maior que INSS + dependentes. INSS de 11 % até o teto do ano. Regras de 2025 também
  semeadas para a folha dos meses passados.
- **Lucros**: acima de R$ 50 mil no mês, da mesma empresa para a mesma pessoa, IRRF de
  10 % sobre o total do mês (não só o excedente); a retirada registra o que falta reter.
- **Simulador**: receita mensal constante nos próximos 12 meses; ME no Anexo V com 1
  salário mínimo de pró-labore contra ME no Anexo III com o pró-labore mínimo do Fator R,
  mais contador. Comércio (Anexo I) ainda não é calculado; a tela avisa.
- **Pacote do contador**: CSV do mês (DRE simplificada, receitas, folha, DAS), com `;` e
  BOM para abrir no Excel e proteção contra fórmula em texto. O PDF, a separação PF/PJ
  automática das transferências e a provisão de impostos por recebimento ficam para a
  fase 7 (telas e gráficos), que já vai mexer nessas telas.

## D31 — IA da fase 6, parte 1

- **Chave única no servidor** (`ANTHROPIC_API_KEY` no `.env`), mas **cada pessoa liga a IA
  para si** (ligar pede a senha de novo) e tem o **próprio teto mensal** em dólares
  (padrão US$ 5), com o consumo registrado por mês em `ia_uso`. A SPEC fala em teto por
  casa; como amigos usam o painel sem casa, o teto é por pessoa.
- **Modelos**: Claude Haiku 4.5 para categorizar em lote (barato) e Claude Opus 5 para o
  chat, com pensamento adaptativo. Preços para o consumo: tabela em `internal/ia`.
- **O que sai do Pi**: só descrição e valor das transações a categorizar (com ids curtos,
  nunca os uuids) e, no chat, a pergunta e os números que as ferramentas devolvem. Antes
  de enviar, `ia.Anonimizar` troca CPF, CNPJ, e-mail, sequências de 5+ dígitos (conta,
  agência, cartão) e o nome em "PIX enviado/recebido FULANO". O servidor não guarda as
  conversas; o histórico fica na tela.
- **Ferramentas fechadas e só de leitura**, cada chamada numa transação com o RLS de quem
  pergunta: gastos por categoria, receitas e gastos do período, busca de transações,
  saldos, próximas contas, carteira, patrimônio e CNPJ. `gastos_por_categoria` e a tela
  Gastos usam a mesma função (`financeiro.GastosPorCategoria`): critério de aceite
  `TestChatIgualATela`. Orçamento, metas, simulações e Monte Carlo entram na parte 2.
- **Categorização**: regras → histórico → IA só para o que sobrou, sem passar por cima de
  categoria escolhida pela pessoa; `categorizada_por_ia` marca a escolha (corrigir à mão
  desmarca, e a correção vira histórico para as próximas). Roda de hora em hora no worker
  e sob demanda. A meta de 90 % certo num mês real (critério da SPEC) só dá para medir com
  a chave de verdade: conferir no primeiro mês e ajustar o prompt se ficar abaixo.
- Parte 2 (v0.7.x): relatório do mês, resumo da semana, alertas inteligentes, Monte Carlo
  do patrimônio, simulações e leitura de PDF.

## D32 — O que mais o Meu Pluggy entrega (v0.7.1)

- **Fonte dos formatos**: a rede do ambiente de desenvolvimento não alcança docs.pluggy.ai
  nem o MCP `pluggy-docs`; os campos foram conferidos nos tipos do SDK oficial
  (`pluggy-sdk` 0.90.0 no npm: `CreditCardBills`, `Loan`, `IdentityResponse`,
  `InvestmentTransaction`). Quando a rede liberar, conferir de novo pela documentação.
- **Tudo opcional**: nem todo banco compartilha todos os produtos. 404/403 de um produto
  vira `ErrProdutoIndisponivel` e é pulado; outro erro entra no relatório da
  sincronização sem parar as transações.
- **Faturas do cartão** (`/bills`, último ano): guardadas em `faturas_banco` e mostradas
  em Cartões ao lado das faturas que o painel calcula pelas compras — são a palavra do
  banco (total, mínimo e encargos cobrados), não substituem o cálculo.
- **Empréstimos e financiamentos** (`/loans`): guardados em `emprestimos_banco`, trocados
  a cada sincronização (o quitado some). Só `LOAN` e `FINANCING` entram no passivo do
  patrimônio: cheque especial e parcelamento de fatura já estão no saldo negativo da
  conta e no saldo do cartão, e contar de novo dobraria a dívida. Se a pessoa também
  cadastrou o contrato em Dívidas, os dois contam; a tela avisa para apagar um.
- **Identidade** (`/identity`): só preenche o CPF da pessoa física do item quando está
  vazio (cifrado, contexto `entidades.documento`); nunca troca o que a pessoa digitou. Nome,
  endereço e telefone não são guardados.
- **Movimentações dos investimentos** (`/investments/{id}/transactions`): viram operações
  do ativo com `origem = 'pluggy'` (sem duplicar, chave `pluggy:<id>`). `CREDIT` é aporte
  (compra), `DEBIT` é resgate (venda, pelo valor líquido quando vem), imposto fica de
  fora; sem quantidade (renda fixa, fundos) vira 1 × valor. Com o histórico cobrindo o
  que foi aplicado, a rentabilidade anual (XIRR) do investimento do banco passa a vir dos
  fluxos de verdade em vez da estimativa por saldo e valor aplicado.

## D33 — Alertas inteligentes (fase 6, parte 2)

- **Regras fixas e explicáveis, sem IA** (`core.DetectarAlertas`, 100 % testado): rodam a
  cada importação e sincronização, só nos gastos novos dos últimos 45 dias (importar um
  ano de histórico não vira enxurrada), no máximo 10 por rodada, os maiores primeiro.
  - **Gasto fora do padrão**: a compra é pelo menos 3× a mediana dos gastos da mesma
    categoria nos últimos 180 dias (com 5 ou mais) e passa dela em R$ 100 ou mais.
  - **Cobrança duplicada**: mesma conta, mesmo valor, descrição parecida
    (`ChaveHistorico`) até 3 dias depois; a partir de R$ 50 (dois cafés iguais não) e fora
    de compras parceladas. O alerta diz "possível": pode ser proposital.
  - **Tarifa bancária** e **juros/IOF**: palavras inteiras na descrição (tarifa, anuidade,
    cesta/pacote de serviços, manutenção de conta; juros, IOF, encargo, multa, mora,
    rotativo).
  - "Assinatura que subiu" já vinha da detecção de recorrências.
- Os limites são constantes no `core` (não são regra fiscal); se incomodarem, ajustar lá.
- **Dispensar** marca o alerta como lido: some do início e não é criado de novo (a
  repetição é checada pela transação). A lista completa continua em `?todos=1`.
