# Changelog

## v0.5.2 — revisão de segurança antes de abrir pela internet

- Checklist dos 19 pontos em `docs/SEGURANCA.md` (onde cada proteção está e como é testada).
- Limite geral por IP na API (600 requisições e 120 escritas por minuto), além do login.
- No máximo 3 hashes de senha ao mesmo tempo (rajada de logins não estoura a memória) e
  parâmetros do hash conferidos.
- O `.xlsx` da B3 abre com limite de descompactação (contra zip bomb).
- Tokens de convite não aparecem mais no log.
- Editar ativo recusa campo desconhecido, como o resto da API.
- CI confere dependências vulneráveis (`govulncheck` e `npm audit`).

## v0.5.1 — túnel da Cloudflare no deploy

- O deploy sobe o `cloudflared` junto (como no fitness-hub) quando o
  `/etc/financas/.env` tem `CLOUDFLARE_TUNNEL_TOKEN`; o painel fica em
  `https://financas.pebasrunners.com`, atrás do Cloudflare Access. Sem token, nada muda.

## v0.5.0 — fase 4: investimentos

- **Cotações automáticas** a cada 6 horas: ações, FIIs, ETFs e BDRs pela brapi, cripto pela
  CoinGecko, CDI, IPCA e dólar pelo Banco Central. Para cotar além de PETR4, VALE3, ITUB4 e
  MGLU3, crie um token grátis em brapi.dev e ponha `BRAPI_TOKEN=` no `/etc/financas/.env`.
- **Renda fixa na curva**: CDB, LCI e Tesouro lançados à mão rendem pela taxa contratada
  (% do CDI, prefixado ou IPCA+) até ter cotação.
- **Rentabilidade ao ano** da carteira e de cada ativo (XIRR, com proventos e vendas) ao
  lado do CDI e do IPCA do mesmo período.
- Fecha a fase 4 junto com a v0.4.6 (B3, ativos à mão, IR mensal e DARF). Decisões em
  `docs/DECISOES.md` D28.

## v0.4.6 — fase 4: B3, ativos e IR mensal

- **Importar da B3**: em Investimentos, o extrato de negociação (compras e vendas) e o de
  movimentação (proventos, desdobros, bonificações) da Área do Investidor, em .xlsx ou
  .csv, com prévia. Os ativos que faltam são criados; importar de novo não duplica.
- **Ativos à mão**: ações, FIIs, Tesouro, CDB (indexador, taxa, vencimento, isenção),
  cripto; cada ativo abre com quantidade, preço médio, custo, proventos e a lista de
  operações, para lançar compra, venda, provento, juros ou amortização e trocar a classe.
  Venda maior que a posição é recusada.
- **IR mensal**: ganho de capital de ações, ETFs, BDRs e FIIs mês a mês, com a isenção de
  R$ 20 mil em ações, compensação de prejuízo por grupo, IR retido abatido, DARF abaixo de
  R$ 10 somado ao mês seguinte e o vencimento (código 6015), com as regras vigentes.

## v0.4.5

- **Convidar amigos**: em Mais › Convidar pessoas sai um link de uso único (48 h) para
  alguém criar a **própria conta**, separada da sua casa: cada um vê só os próprios dados e
  conecta o próprio Meu Pluggy. O convite de casa continua para quem divide as finanças.
- **Início alinhado**: todos os blocos de números seguem o mesmo padrão (ícone, rótulo,
  valor e detalhe) e a mesma altura, no celular e no computador.
- Deploy: o build no Pi usa a rede do host, porque o DNS antigo do Tailscale derrubava o
  `go mod download` (a v0.4.3 e a v0.4.4 não chegaram ao Pi; esta leva as duas).

## v0.4.4

- Inclui tudo da v0.4.3, que não chegou ao Pi (o build parou no DNS antigo do Tailscale).
- **Gastos como fluxo de caixa**: despesas do mês por categoria, despesas futuras por mês
  (parcelas, financiamentos e contas fixas), entradas e saídas ao lado do mês, alternar
  Todos/Entradas/Saídas e a lista agrupada por dia; linhas mais compactas no celular.
- **Investimentos**: tela nova com a carteira por classe e cada ativo com instituição,
  vencimento e rendimento. Os investimentos do Open Finance (CDB, LCI, Tesouro, fundos)
  entram sozinhos com o saldo do banco; resgatados ficam como encerrados; LCI e LCA
  marcadas como isentas. A carteira entra no patrimônio e no bloco de Investimentos do
  Início.

## v0.4.3

- **Categoria do banco**: as transações do Open Finance usam a categoria que o banco dá
  (mercado, restaurante, app de transporte, farmácia…) quando não há regra nem histórico;
  pagamento de fatura e transferência para si mesmo saem do gasto. As transações que já
  estavam sem categoria são completadas na próxima sincronização.
- **Visual novo**, inspirado no Meu Pluggy: fundo quase preto com destaque rosa; Início
  com contas bancárias por banco, cartões com o % do limite usado, investimentos e a
  evolução do patrimônio; cartões do Início com a mesma altura.
- Base da fase 4 (ainda sem tela): tabelas de ativos, operações e cotações, regras de IR
  dos investimentos com vigência e leitura dos extratos da B3.

## v0.4.2

- Cartões pelo Open Finance: o limite usado vem do banco (limite − disponível, com as
  parcelas futuras), o limite é atualizado a cada sincronização e os dias de fechamento e
  vencimento são preenchidos quando a conta não tem, recalculando as faturas. Antes o
  limite usado saía só das transações importadas e o cartão ligado a uma conta existente
  ficava sem dias.
- A tela de Open Finance mostra limite e disponível de cada cartão.

## v0.4.1

- Open Finance: a busca de transações recusada pela Pluggy (400) no primeiro uso real. O
  pedido à v2 vai só com `accountId` e `dateFrom`, e se a Pluggy recusar a v2 o painel usa a
  v1. Erro 400 agora mostra a mensagem da própria Pluggy.
- Texto de ajuda da tela de Open Finance com o caminho certo: credenciais e id do item vêm do
  Dashboard da Pluggy, conectando pela Demo com o conector MeuPluggy.

## v0.4.0 — Fase 3: Open Finance

- **Meu Pluggy por pessoa**: cada um cola o próprio Client ID e Secret (conferidos na Pluggy,
  guardados cifrados, pedem a senha de novo) e adiciona os bancos conectados lá, escolhendo
  de quem são as contas. Ninguém mais vê as credenciais nem os bancos de outra pessoa.
- **Contas**: cada conta que o banco devolve é ligada a uma conta que já existe, vira conta
  nova (com banco, limite e dias do cartão) ou é ignorada.
- **Sincronização** diária pelo worker e botão "Sincronizar agora": transações pela mesma
  fila dos arquivos (deduplicar, categorizar, transferências, recorrências), pendentes só
  quando confirmadas, parcelas do cartão com "(n/N)".
- **Arquivo e Open Finance juntos não duplicam**: a mesma transação vinda do OFX e da Pluggy
  (mesmo valor, até 1 dia de diferença) vira uma só, nos dois sentidos.
- **Conciliação**: saldo do banco × saldo calculado por dia; diferença vira alerta, aparece
  na conta e se resolve com "Acertar pelo banco".
- `/api/health` com `pluggy` (ok, erro, atrasada) e `last_sync`.
- Testes e e2e contra uma API da Pluggy falsa (`cmd/pluggy-falsa`), que não vai para a imagem.

## v0.3.0 — Fase 2: planejamento

- **Cartões**: faturas por fechamento e vencimento (aberta, futuras e anteriores), limite
  usado e disponível, melhor dia de compra e parcelas futuras mês a mês. Compra parcelada
  lançada à mão (2 a 72×) cai uma parcela em cada fatura; parcelas importadas ("Parcela
  3/12") projetam as que faltam. Mudar os dias do cartão recalcula as faturas.
- **Recorrências**: assinaturas, contas fixas e receitas, cadastradas ou **detectadas** no
  histórico (3 meses seguidos), com aviso quando o valor sobe e quando a cobrança esperada
  não aparece.
- **Orçamento**: limite por categoria, envelopes (a sobra passa para o mês seguinte) ou
  regra 50/30/20; barra com a marca do ritmo ideal do dia, situação (no ritmo, acima do
  ritmo, estourou), média de 3 meses como sugestão e cópia do mês anterior.
- **Metas**: alvo, prazo, contas vinculadas, quanto guardar por mês, data prevista no ritmo
  planejado e, na reserva de emergência, quantos meses de custo de vida ela cobre.
- **Patrimônio**: contas, investimentos, bens (imóvel, veículo), cartões, cheque especial e
  dívidas com tabela Price ou SAC e saldo devedor; variação no mês e no ano e os últimos 12
  meses.
- **Agenda**: o que vence em 30, 60 ou 90 dias (faturas, recorrências, prestações, contas a
  pagar e receber avulsas) com o saldo projetado dia a dia e o menor saldo do período.
- **Início**: patrimônio líquido, custo de vida e meses de reserva, saldo em 30 dias,
  comprometido do mês que vem e gasto × orçamento. Alertas de recorrência com texto.
- Correções: parcelas com data futura não aparecem mais como "últimas transações" nem na
  contagem de "sem categoria"; nunca "-R$ 0,00".

## v0.2.0 — Fase 1: contas e transações

- **Contas e cartões**: tipo, banco, moeda, saldo inicial, limite, fechamento e vencimento,
  visibilidade (privada, só saldo, compartilhada) e arquivamento. Saldo calculado; a casa
  vê o saldo de contas "só saldo" sem ver as transações, e de quem é cada conta.
- **Importação** de OFX (1.x e 2.x, Windows-1252, vírgula ou ponto) e CSV (Nubank conta,
  Nubank cartão, Inter e genérico com mapeamento de colunas salvo). Prévia antes de gravar,
  conferência do saldo com o do banco e histórico com **desfazer** por inteiro.
- **Deduplicação** por conta + data + valor + descrição normalizada + id externo: o mesmo
  arquivo (ou o mesmo extrato em outro formato) não duplica nada; duas compras iguais no
  mesmo dia continuam sendo duas.
- **Transferências internas**: saída numa conta e entrada do mesmo valor em outra conta da
  mesma entidade, em até 2 dias, viram transferência e não entram como gasto nem receita.
- **Categorias** padrão em dois níveis e personalizadas por entidade; **regras** "descrição
  contém X" com prioridade; categorização pelo histórico de descrições parecidas; estorno
  (categoria de gasto em entrada) abate o gasto.
- **Telas**: Início com saldo, gasto do mês comparado ao mesmo período do mês passado,
  receitas, taxa de poupança, gastos por categoria e últimas transações; Gastos e
  transações com busca, filtros, edição em massa (com criação de regra), lançamento manual
  e atalhos `/` e `n`; Contas; Importar; Categorias e regras.
- Limite de tentativas de login por IP configurável (`LIMITE_AUTH_POR_MINUTO`, padrão 10);
  consulta de convite com limite próprio.

## v0.1.1

- Acesso pelo Cloudflare Tunnel (DECISOES D11): IP real do visitante via `CF-Connecting-IP`,
  aceito só de proxy confiável, para o limite de tentativas por IP funcionar.
- Modo rede de casa (D12): com `PUBLIC_URL` em http, cookies sem `Secure`/`__Host-`, sem
  HSTS e passkeys desligadas; `ESCUTAR` no `.env` escolhe onde a porta 3100 escuta.
- Deploy: a chave mestra só precisa existir para o runner (quem lê é o container).
- Testes: migrações dos testes em série (o papel do app é do servidor inteiro).

## v0.1.0 — Fase 0: fundação

- Backend em Go (um binário: `serve`, `worker`, `migrar`, `saude`) e front em React + Vite
  embutido; ver `docs/DECISOES.md` D1.
- PostgreSQL com RLS por entidade e por casa; o app conecta como `financas_app`, sem
  superuser nem BYPASSRLS. Testes: toda tabela com RLS, isolamento entre usuários no banco
  e **por todas as rotas de leitura**, casa, contador só em PJ, casa sempre com dono.
- Autenticação: argon2id, sessão parcial até o segundo fator, TOTP com anti-replay,
  passkeys (WebAuthn), 10 códigos de recuperação, bloqueio progressivo, limite por IP,
  reautenticação para ações sensíveis, alerta de login em aparelho novo.
- Casas com convite por link (48 h, uso único), papéis dono/membro/leitor; entidades PF e
  CNPJ (MEI, Simples ME/EPP) com CPF/CNPJ validado, cifrado (AES-256-GCM) e mascarado;
  acessos membro/leitor/contador.
- Regras fiscais de 2026 com vigência (teto do MEI, DAS-MEI, Anexos III e V, Fator R…).
- Segurança HTTP: CSP estrita sem inline, cookies `__Host-` `SameSite=Strict`, checagem de
  `Origin` em toda escrita, cabeçalhos de segurança.
- Front mobile-first: tema escuro e claro por tokens, modo privacidade, barra inferior no
  celular, telas de login, 2FA, início, entidades, casa, convite e segurança.
- `/api/health` no formato do PiControl; worker com sinal de vida por minuto.
- Deploy: Docker Compose (Postgres em rede sem saída, limites de memória), backup a cada
  6 h, `deploy.sh` com backup pré-migração e volta automática de imagem e banco;
  workflows de CI (Go, front, e2e com Playwright, imagem) e release para o runner do Pi.
