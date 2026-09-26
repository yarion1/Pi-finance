import { createHmac } from "node:crypto";
import { type BrowserContext, expect, type Page, test } from "@playwright/test";

// TOTP (RFC 6238) sem dependências, para o teste digitar o código do "aplicativo".
function totp(segredoBase32: string, momento = Date.now()): string {
  const alfabeto = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const c of segredoBase32.replace(/=+$/, "").toUpperCase())
    bits += alfabeto.indexOf(c).toString(2).padStart(5, "0");
  const chave = Buffer.from(bits.match(/.{8}/g)?.map((b) => Number.parseInt(b, 2)) ?? []);
  const contador = Buffer.alloc(8);
  contador.writeBigUInt64BE(BigInt(Math.floor(momento / 1000 / 30)));
  const h = createHmac("sha1", chave).update(contador).digest();
  const o = (h[h.length - 1] ?? 0) & 0xf;
  const n = (h.readUInt32BE(o) & 0x7fffffff) % 1_000_000;
  return n.toString().padStart(6, "0");
}

async function autenticadorVirtual(contexto: BrowserContext, pagina: Page) {
  const cdp = await contexto.newCDPSession(pagina);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });
}

function vigiarCSP(pagina: Page, violacoes: string[]) {
  pagina.on("console", (m) => {
    if (m.type() === "error" && /Content Security Policy/i.test(m.text())) violacoes.push(m.text());
  });
}

async function semRolagemHorizontal(pagina: Page) {
  const largura = await pagina.evaluate(() => document.documentElement.scrollWidth);
  expect(largura, `rolagem horizontal em ${pagina.url()}`).toBeLessThanOrEqual(360);
}

const senha = "uma frase longa de teste";
let linkConvite = "";
let codigosAna: string[] = [];

test.describe
  .serial("fase 0", () => {
    test("primeira conta: cadastro, TOTP obrigatório, entidade e casa", async ({ page }) => {
      const violacoes: string[] = [];
      vigiarCSP(page, violacoes);

      await page.goto("/");
      await expect(page).toHaveURL(/\/entrar/);
      await page.getByRole("link", { name: "Crie a primeira" }).click();
      await page.getByLabel("Nome").fill("Ana Teste");
      await page.getByLabel("E-mail").fill("ana@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Criar conta" }).click();

      await expect(page).toHaveURL(/\/configurar-2fa/);
      // sem segundo fator, nada de dados
      const bloqueado = await page.evaluate(() => fetch("/api/entidades").then((r) => r.status));
      expect(bloqueado).toBe(403);

      await page.getByRole("button", { name: /Aplicativo autenticador/ }).click();
      const segredo = (await page.locator("p.font-mono").innerText()).replace(/\s/g, "");
      await page.getByLabel("Código de 6 dígitos").fill(totp(segredo));
      await page.getByRole("button", { name: "Ativar" }).click();

      const codigos = page.locator("ul.font-mono li");
      await expect(codigos).toHaveCount(10);
      codigosAna = await codigos.allInnerTexts();
      await page.getByLabel("Guardei os códigos em lugar seguro").check();
      await page.getByRole("button", { name: "Continuar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Ana");

      // entidade PF com CPF cifrado e mascarado
      await page.goto("/entidades");
      await page.getByRole("button", { name: "Nova entidade" }).click();
      await page.getByLabel("Nome", { exact: true }).fill("Ana PF");
      await page.getByLabel("CPF (opcional)").fill("52998224725");
      await expect(page.getByLabel("CPF (opcional)")).toHaveValue("529.982.247-25");
      await page.getByRole("button", { name: "Salvar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("Ana PF");
      await expect(page.getByText("***.982.247-**")).toBeVisible();

      // casa e convite
      await page.goto("/casa");
      await page.getByLabel("Nome").fill("Casa Teste");
      await page.getByRole("button", { name: "Criar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("Casa Teste");
      await page.getByRole("button", { name: "Gerar link" }).click();
      linkConvite = await page.locator("code").innerText();
      expect(linkConvite).toMatch(/\/convite\/[\w-]{20,}$/);

      // celular de 360 px: sem rolagem horizontal
      await page.setViewportSize({ width: 360, height: 740 });
      for (const rota of ["/", "/entidades", "/casa", "/mais", "/seguranca"]) {
        await page.goto(rota);
        await page.waitForLoadState("networkidle");
        await semRolagemHorizontal(page);
      }
      await expect(page.getByRole("navigation", { name: "Principal" }).last()).toBeVisible();

      expect(violacoes).toEqual([]);
    });

    test("segunda pessoa: convite, passkey e login só com passkey", async ({ browser }) => {
      const contexto = await browser.newContext();
      const page = await contexto.newPage();
      const violacoes: string[] = [];
      vigiarCSP(page, violacoes);
      await autenticadorVirtual(contexto, page);

      await page.goto(new URL(linkConvite).pathname);
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("Convite para Casa Teste");
      await page.getByRole("button", { name: "Criar minha conta" }).click();
      await page.getByLabel("Nome").fill("Bruno Teste");
      await page.getByLabel("E-mail").fill("bruno@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Criar conta" }).click();

      await page.getByRole("button", { name: /Passkey/ }).click();
      await page.getByRole("button", { name: "Criar passkey" }).click();
      await expect(page.locator("ul.font-mono li")).toHaveCount(10);
      await page.getByLabel("Guardei os códigos em lugar seguro").check();
      await page.getByRole("button", { name: "Continuar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Bruno");

      // entrou na casa pelo convite, e o convite não vale de novo
      await page.goto("/casa");
      await expect(page.getByRole("link", { name: /Casa Teste/ })).toBeVisible();
      const reuso = await page.evaluate(
        (u) => fetch(u).then((r) => r.json()),
        `/api/convites/${linkConvite.split("/").pop()}`,
      );
      expect(reuso.valido).toBe(false);

      // B não enxerga a entidade de A
      await page.goto("/entidades");
      await expect(page.getByText("Nenhuma entidade ainda")).toBeVisible();

      // sair e voltar só com a passkey
      await page.goto("/mais");
      await page.getByRole("button", { name: "Sair" }).last().click();
      await expect(page).toHaveURL(/\/entrar/);
      await page.getByRole("button", { name: "Entrar com passkey" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Bruno");

      expect(violacoes).toEqual([]);
      await contexto.close();
    });

    test("login com senha pede o segundo fator; código de recuperação vale uma vez", async ({ page }) => {
      await page.goto("/entrar");
      await page.getByLabel("E-mail").fill("ana@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Continuar" }).click();
      await expect(page).toHaveURL(/\/verificar/);
      await page.getByRole("button", { name: "Usar código de recuperação" }).click();
      await page.getByLabel("Código de recuperação").fill(codigosAna[0] ?? "");
      await page.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Ana");

      // tema claro e modo privacidade
      await page.getByRole("button", { name: "Tema claro" }).first().click();
      await expect(page.locator("html")).toHaveAttribute("data-theme", "claro");
      await page.getByRole("button", { name: "Esconder valores" }).first().click();
      await expect(page.locator("html")).toHaveAttribute("data-privado", "sim");
    });
  });

test.describe
  .serial("fase 1", () => {
    const hoje = new Intl.DateTimeFormat("en-CA", { timeZone: "America/Sao_Paulo" })
      .format(new Date())
      .replaceAll("-", "");
    // um mês atrás (fora do mês corrente, para não mexer nos totais das outras fases)
    const mesPassado = new Intl.DateTimeFormat("en-CA", { timeZone: "America/Sao_Paulo" })
      .format(new Date(Date.now() - 35 * 86_400_000))
      .replaceAll("-", "");
    const ofx = (...linhas: [string, string, string, string?][]) =>
      Buffer.from(
        `OFXHEADER:100\nDATA:OFXSGML\n\n<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><CURDEF>BRL<BANKTRANLIST>\n${linhas
          .map(
            ([valor, id, desc, quando]) =>
              `<STMTTRN>\n<DTPOSTED>${quando ?? hoje}\n<TRNAMT>${valor}\n<FITID>${id}\n<MEMO>${desc}\n</STMTTRN>\n`,
          )
          .join("")}</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>\n`,
      );

    test("contas, importação sem duplicar, transferência fora do gasto e categorização em massa", async ({
      page,
    }) => {
      const violacoes: string[] = [];
      vigiarCSP(page, violacoes);

      await page.goto("/entrar");
      await page.getByLabel("E-mail").fill("ana@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Continuar" }).click();
      await page.getByRole("button", { name: "Usar código de recuperação" }).click();
      await page.getByLabel("Código de recuperação").fill(codigosAna[1] ?? "");
      await page.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Ana");

      // duas contas da mesma pessoa
      await page.goto("/contas");
      for (const [nome, saldo] of [
        ["Nubank", "1.000,00"],
        ["Inter", "0,00"],
      ]) {
        await page.getByRole("button", { name: "Nova conta" }).click();
        const d = page.getByRole("dialog");
        await d.getByLabel("Nome").fill(nome ?? "");
        await d.getByLabel("Saldo inicial").fill(saldo ?? "");
        await d.getByRole("button", { name: "Salvar" }).click();
        await expect(page.getByRole("link", { name: new RegExp(`^${nome}`) })).toBeVisible();
      }

      // importar o mesmo OFX duas vezes não duplica
      const extratoNubank = ofx(
        ["-45.90", "n1", "Padaria Pao Quente"],
        ["-45.90", "n2", "Padaria Pao Quente"],
        ["-1200.00", "n3", "Pix enviado Ana Inter"],
      );
      await page.goto("/importar");
      await page.getByLabel("Conta").selectOption({ label: "Nubank — Ana PF" });
      await page
        .locator('input[type="file"]')
        .setInputFiles({ name: "nubank.ofx", mimeType: "application/x-ofx", buffer: extratoNubank });
      await page.getByRole("button", { name: "Ver prévia" }).click();
      await expect(page.getByText("3 novas · 0 já importadas")).toBeVisible();
      await page.getByRole("button", { name: "Importar 3 transações" }).click();
      await expect(page.getByText("Importação concluída")).toBeVisible();

      await page
        .locator('input[type="file"]')
        .setInputFiles({ name: "nubank.ofx", mimeType: "application/x-ofx", buffer: extratoNubank });
      await page.getByRole("button", { name: "Ver prévia" }).click();
      await expect(page.getByText("0 novas · 3 já importadas")).toBeVisible();
      await expect(page.getByRole("button", { name: /Importar 0/ })).toBeDisabled();

      // a outra ponta do Pix, no Inter, vira transferência
      await page.getByLabel("Conta").selectOption({ label: "Inter — Ana PF" });
      await page.locator('input[type="file"]').setInputFiles({
        name: "inter.ofx",
        mimeType: "application/x-ofx",
        buffer: ofx(["1200.00", "i1", "Pix recebido Ana Nubank"]),
      });
      await page.getByRole("button", { name: "Ver prévia" }).click();
      await page.getByRole("button", { name: "Importar 1 transação" }).click();
      await expect(page.getByText(/1 transferências entre contas/)).toBeVisible();

      // o gasto do mês não inclui a transferência
      await page.goto("/");
      const gasto = page.locator("section", { hasText: "Gasto do mês" }).first();
      await expect(gasto).toContainText("91,80");
      await expect(gasto).not.toContainText("1.291,80");
      await expect(page.getByText(/2 transações estão sem categoria/)).toBeVisible();

      // categoriza as duas padarias de uma vez, criando regra
      await page.goto("/gastos?sem_categoria=1");
      const linhas = page.getByRole("checkbox", { name: /Selecionar Padaria/ });
      await expect(linhas).toHaveCount(2);
      await linhas.nth(0).check();
      await linhas.nth(1).check();
      await page.getByLabel("Mudar categoria para").selectOption({ label: "Alimentação" });
      await page.getByLabel(/Criar regra/).check();
      await page.getByRole("button", { name: "Aplicar" }).click();
      await expect(page.getByText("Nenhuma transação aqui")).toBeVisible();

      await page.goto("/categorias");
      await expect(page.getByText(/contém “Padaria Pao”/)).toBeVisible();

      // tarifa do banco vira alerta no início, que dá para dispensar
      await page.goto("/importar");
      await page.getByLabel("Conta").selectOption({ label: "Inter — Ana PF" });
      await page.locator('input[type="file"]').setInputFiles({
        name: "tarifa.ofx",
        mimeType: "application/x-ofx",
        buffer: ofx(["-19.90", "t1", "TARIFA PACOTE SERVICOS", mesPassado]),
      });
      await page.getByRole("button", { name: "Ver prévia" }).click();
      await page.getByRole("button", { name: "Importar 1 transação" }).click();
      await expect(page.getByText("Importação concluída")).toBeVisible();
      await page.goto("/");
      const alerta = page.getByRole("link", { name: "Tarifa bancária: TARIFA PACOTE SERVICOS" });
      await expect(alerta).toBeVisible();
      await page.getByRole("button", { name: "Dispensar: Tarifa bancária: TARIFA PACOTE SERVICOS" }).click();
      await expect(alerta).toHaveCount(0);

      // telas novas sem rolagem lateral no celular
      await page.setViewportSize({ width: 360, height: 740 });
      for (const rota of ["/", "/gastos", "/contas", "/importar", "/categorias"]) {
        await page.goto(rota);
        await page.waitForLoadState("networkidle");
        await semRolagemHorizontal(page);
      }
      expect(violacoes).toEqual([]);
    });
    test("fase 2: compra de R$ 1.200 em 12× nas 12 faturas certas e orçamento com ritmo", async ({
      page,
    }) => {
      const violacoes: string[] = [];
      vigiarCSP(page, violacoes);

      await page.goto("/entrar");
      await page.getByLabel("E-mail").fill("ana@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Continuar" }).click();
      await page.getByRole("button", { name: "Usar código de recuperação" }).click();
      await page.getByLabel("Código de recuperação").fill(codigosAna[2] ?? "");
      await page.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Ana");
      await expect(page.getByText("Patrimônio líquido")).toBeVisible();

      // cartão com fechamento e vencimento
      await page.goto("/contas");
      await page.getByRole("button", { name: "Nova conta" }).click();
      let d = page.getByRole("dialog");
      await d.getByLabel("Nome").fill("Cartão Roxo");
      await d.getByLabel("Tipo").selectOption("cartao");
      await d.getByLabel("Limite").fill("5.000,00");
      await d.getByLabel("Dia do fechamento").fill("5");
      await d.getByLabel("Dia do vencimento").fill("12");
      await d.getByRole("button", { name: "Salvar" }).click();
      await expect(page.getByRole("link", { name: /^Cartão Roxo/ })).toBeVisible();

      // compra parcelada lançada à mão
      await page.goto("/gastos");
      await page.getByRole("button", { name: "Nova", exact: true }).click();
      d = page.getByRole("dialog");
      await d.getByLabel("Valor").fill("1.200,00");
      await d.getByLabel("Descrição").fill("Geladeira");
      await d.getByLabel("Conta", { exact: true }).selectOption({ label: "Cartão Roxo" });
      await d.getByLabel("Parcelas").fill("12");
      await expect(d.getByText(/12× de cerca de R\$\s100,00/)).toBeVisible();
      await d.getByRole("button", { name: "Lançar" }).click();
      await expect(d).toBeHidden();

      // uma parcela de R$ 100 em cada uma das 12 próximas faturas
      await page.goto("/cartoes");
      const faturas = page.getByRole("list", { name: "Próximas faturas" }).getByRole("listitem");
      await expect(faturas).toHaveCount(12);
      for (const f of await faturas.all()) await expect(f).toContainText("100,00");
      await expect(page.getByText("Melhor dia de compra: 5")).toBeVisible();
      await expect(page.getByText(/Geladeira \(12\/12\)/)).toBeVisible();

      // orçamento: limite para Alimentação e a barra com o ritmo do mês
      await page.goto("/orcamento");
      await page.getByRole("button", { name: "Definir limites" }).first().click();
      d = page.getByRole("dialog");
      await d.getByLabel("Alimentação", { exact: true }).fill("100,00");
      await d.getByRole("button", { name: "Salvar limites" }).click();
      await expect(d).toBeHidden();
      const item = page.getByRole("list", { name: "Limites por categoria" }).getByRole("listitem").first();
      await expect(item).toContainText("Alimentação");
      await expect(item).toContainText("91,80");
      await expect(item.getByRole("meter")).toBeVisible();
      await expect(item.getByText(/No ritmo|Acima do ritmo|Estourou/)).toBeVisible();
      await expect(page.getByText(/Dia \d+ de \d+/)).toBeVisible();

      // meta de reserva ligada à conta
      await page.goto("/metas");
      await page.getByRole("button", { name: "Nova meta" }).click();
      d = page.getByRole("dialog");
      await d.getByLabel("Nome").fill("Reserva");
      await d.getByLabel("Quanto quer juntar").fill("10.000,00");
      await d.getByLabel(/Inter/).check();
      await d.getByRole("button", { name: "Salvar" }).click();
      await expect(page.getByRole("heading", { name: "Reserva" })).toBeVisible();
      await expect(page.getByRole("meter", { name: "Progresso de Reserva" })).toBeVisible();

      // telas novas sem rolagem lateral no celular
      await page.setViewportSize({ width: 360, height: 740 });
      for (const rota of [
        "/",
        "/cartoes",
        "/orcamento",
        "/metas",
        "/patrimonio",
        "/agenda",
        "/recorrencias",
        "/mais",
      ]) {
        await page.goto(rota);
        await page.waitForLoadState("networkidle");
        await semRolagemHorizontal(page);
      }
      expect(violacoes).toEqual([]);
    });
    test("fase 3: Open Finance conecta as próprias contas e a diferença de saldo vira alerta", async ({
      page,
    }) => {
      const violacoes: string[] = [];
      vigiarCSP(page, violacoes);
      page.on("dialog", (d) => d.accept()); // confirmações nativas

      await page.goto("/entrar");
      await page.getByLabel("E-mail").fill("ana@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Continuar" }).click();
      await page.getByRole("button", { name: "Usar código de recuperação" }).click();
      await page.getByLabel("Código de recuperação").fill(codigosAna[3] ?? "");
      await page.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Ana");

      // credenciais do Meu Pluggy (a API falsa do e2e), com a senha de novo
      await page.goto("/open-finance");
      await expect(page.getByText("Como conectar")).toBeVisible();
      await page.getByLabel("Client ID").fill("demo-client-id");
      await page.getByLabel("Client Secret").fill("errado");
      await page.getByRole("button", { name: "Conectar" }).click();
      const reauth = page.getByRole("dialog", { name: "Confirme sua senha" });
      await reauth.getByLabel("Senha").fill(senha);
      await reauth.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByText("recusou o Client ID")).toBeVisible();
      await page.getByLabel("Client Secret").fill("demo-client-secret");
      await page.getByRole("button", { name: "Conectar" }).click();
      await expect(page.getByText("Meu Pluggy conectado")).toBeVisible();
      await expect(page.getByText("demo-client-secret")).toHaveCount(0);

      // o banco e as contas dele
      await page.getByLabel("Id do item no Meu Pluggy").fill("item-demo");
      await page.getByRole("button", { name: "Adicionar banco" }).click();
      const contasDoBanco = page.getByRole("list", { name: "Contas de Nubank" }).getByRole("listitem");
      await expect(contasDoBanco).toHaveCount(2);

      // a conta corrente liga à "Nubank" que já existe; o cartão vira conta nova
      const corrente = contasDoBanco.filter({ hasText: "Conta Nubank" });
      await corrente.getByLabel("O que fazer com esta conta").selectOption({ label: "Ligar a “Nubank”" });
      await corrente.getByRole("button", { name: "Confirmar" }).click();
      await expect(corrente.getByText("Ligada a")).toBeVisible();
      const cartao = contasDoBanco.filter({ hasText: "Nubank Ultravioleta" });
      await cartao.getByLabel("O que fazer com esta conta").selectOption("criar");
      await cartao.getByRole("button", { name: "Confirmar" }).click();
      await expect(cartao.getByText("Ligada a")).toBeVisible();

      // sincroniza (o botão tem uma trava de alguns segundos entre uma e outra)
      await expect(async () => {
        await page.getByRole("button", { name: "Sincronizar agora" }).click();
        await expect(page.getByText(/Sincronizado: 4 transações novas/)).toBeVisible({ timeout: 2000 });
      }).toPass({ timeout: 30_000 });
      await expect(page.getByText(/1 conta ficou com saldo diferente do banco/)).toBeVisible();

      // a diferença vira alerta no início e aparece na conta, com o acerto
      await page.goto("/");
      await expect(page.getByText("Saldo de Nubank diferente do banco")).toBeVisible();
      await page.goto("/contas");
      const nubank = page
        .getByRole("listitem")
        .filter({ has: page.getByRole("link", { name: /^Nubank/ }) })
        .first();
      await expect(nubank.getByText(/· Open Finance/)).toBeVisible();
      await expect(nubank.getByText(/banco: R\$\s2\.345,67/)).toBeVisible();
      await nubank.getByRole("button", { name: "Acertar pelo banco" }).click();
      await expect(nubank.getByText("confere com o banco")).toBeVisible();

      // os investimentos do banco entram na carteira (o resgatado fica como encerrado)
      await page.goto("/investimentos");
      await expect(page.getByRole("heading", { name: /Carteira \(2 ativos\)/ })).toBeVisible();
      await expect(page.getByText("1 encerrados (resgatados)")).toBeVisible();
      await expect(page.getByText("isento de IR")).toBeVisible();

      // o que mais o Meu Pluggy entrega: a fatura fechada pelo banco e o empréstimo
      await page.goto("/cartoes");
      await page.getByRole("tab", { name: "Nubank Ultravioleta" }).click();
      const faturasBanco = page.getByRole("list", { name: "Faturas segundo o banco" });
      await expect(faturasBanco.getByText("R$ 1.234,56")).toBeVisible();
      await expect(faturasBanco.getByText(/mínimo R\$\s185,18/)).toBeVisible();
      await page.goto("/patrimonio");
      const emprestimos = page.getByRole("list", { name: "Empréstimos segundo o banco" });
      await expect(emprestimos.getByText("Crédito pessoal")).toBeVisible();
      await expect(emprestimos.getByText(/5 de 12 parcelas pagas/)).toBeVisible();
      await expect(emprestimos.getByText("R$ 6.543,21")).toBeVisible();
      await page.goto("/investimentos");

      // fase 4: extrato de negociação da B3 (prévia, importar, de novo não duplica)
      const negociacao = Buffer.from(
        "Data do Negócio;Tipo de Movimentação;Mercado;Prazo/Vencimento;Instituição;Código de Negociação;Quantidade;Preço;Valor\n" +
          "02/01/2026;Compra;Mercado à Vista;-;XP;PETR4;1000;30,00;30.000,00\n" +
          "05/01/2026;Compra;Mercado à Vista;-;XP;ITSA4;100;10,00;1.000,00\n" +
          "20/04/2026;Venda;Mercado à Vista;-;XP;PETR4;1000;35,00;35.000,00\n",
      );
      for (const vez of [1, 2]) {
        await page.getByRole("button", { name: "Importar da B3" }).click();
        const d = page.getByRole("dialog", { name: "Importar da B3" });
        await d.locator('input[type="file"]').setInputFiles({
          name: "negociacao.csv",
          mimeType: "text/csv",
          buffer: negociacao,
        });
        await d.getByRole("button", { name: "Ver prévia" }).click();
        if (vez === 1) {
          await expect(d.getByText(/3 linhas novas/)).toBeVisible();
          await expect(d.getByText("Ativos que serão criados: PETR4, ITSA4.")).toBeVisible();
          await d.getByRole("button", { name: "Importar 3 linhas" }).click();
          await expect(d.getByText(/Importado: extrato de negociação/)).toBeVisible();
        } else {
          await expect(d.getByText(/0 linhas novas, 3 já importadas/)).toBeVisible();
          await expect(d.getByRole("button", { name: /Importar 0/ })).toBeDisabled();
        }
        await d.getByRole("button", { name: "Fechar" }).click();
      }
      await expect(page.getByRole("heading", { name: /Carteira \(3 ativos\)/ })).toBeVisible();

      // provento lançado à mão no ativo
      await page.getByRole("button", { name: /ITSA4/ }).click();
      const ativo = page.getByRole("dialog", { name: "ITSA4" });
      await expect(ativo.getByText(/05\/01\/2026 · 100 × 10/)).toBeVisible();
      await ativo.getByLabel("Tipo").selectOption("provento");
      await ativo.getByLabel("Valor recebido (R$)").fill("12,34");
      await ativo.getByRole("button", { name: "Lançar" }).click();
      await expect(ativo.getByText("Proventos recebidos")).toBeVisible();
      await expect(ativo.getByText("R$ 12,34").first()).toBeVisible();
      await ativo.getByRole("button", { name: "Fechar" }).click();

      // venda de R$ 35 mil com R$ 5 mil de ganho: DARF de R$ 750 no fim de maio
      await page.getByRole("link", { name: "IR mensal" }).click();
      await expect(page.getByRole("heading", { name: "IR dos investimentos" })).toBeVisible();
      await expect(page.getByText("R$ 750,00").first()).toBeVisible();
      await expect(page.getByText("DARF 6015 até 29/05/2026")).toBeVisible();

      await page.setViewportSize({ width: 360, height: 740 });
      for (const rota of [
        "/open-finance",
        "/contas",
        "/cartoes",
        "/patrimonio",
        "/investimentos",
        "/investimentos/ir",
        "/gastos",
      ]) {
        await page.goto(rota);
        await page.waitForLoadState("networkidle");
        await semRolagemHorizontal(page);
      }
      expect(violacoes).toEqual([]);
    });
    test("fase 5: MEI com teto, receita, DAS e simulador de migração", async ({ page }) => {
      const violacoes: string[] = [];
      vigiarCSP(page, violacoes);

      await page.goto("/entrar");
      await page.getByLabel("E-mail").fill("ana@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Continuar" }).click();
      await page.getByRole("button", { name: "Usar código de recuperação" }).click();
      await page.getByLabel("Código de recuperação").fill(codigosAna[4] ?? "");
      await page.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Ana");

      // sem CNPJ, a tela manda cadastrar
      await page.goto("/cnpj");
      await expect(page.getByText("Nenhum CNPJ cadastrado")).toBeVisible();

      await page.goto("/entidades");
      await page.getByRole("button", { name: "Nova entidade" }).click();
      await page.getByLabel("Tipo").selectOption("PJ");
      await page.getByLabel("Nome", { exact: true }).fill("Ana Dev MEI");
      await page.getByLabel("Regime").selectOption("MEI");
      await page.getByLabel("Atividade").selectOption("servicos");
      await page.getByRole("button", { name: "Salvar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toHaveText("Ana Dev MEI");

      // R$ 60 mil no ano: 74 % do teto de R$ 81 mil → atenção
      await page.goto("/cnpj");
      await expect(page.getByText(/Teto do MEI em/)).toBeVisible();
      await page.getByRole("button", { name: "Receita", exact: true }).click();
      const d = page.getByRole("dialog", { name: "Nova receita" });
      await d.getByLabel("Valor (R$)").fill("60.000,00");
      await d.getByLabel("Cliente (opcional)").fill("Cliente Um");
      await d.getByLabel("Nº da nota (opcional)").fill("7");
      await d.getByRole("button", { name: "Salvar receita" }).click();
      await expect(page.getByText("Faturamento passou de 70 % do teto do MEI.")).toBeVisible();
      await expect(page.getByText(/74\s?% de R\$\s81\.000,00/)).toBeVisible();
      await expect(page.getByText("Cliente Um · nota 7")).toBeVisible();

      // DAS-MEI do mês marcado como pago
      const das = page.getByRole("button", { name: /paguei/i }).first();
      await das.click();
      await expect(page.getByText("pago", { exact: true }).first()).toBeVisible();

      // simulador: R$ 10 mil por mês não cabe no MEI; o Anexo III com Fator R sai mais barato
      await page.getByLabel("Receita por mês (R$)").fill("10.000,00");
      await expect(page.getByText("A receita projetada passa do teto do MEI.")).toBeVisible();
      const melhor = page.getByRole("listitem").filter({ hasText: "mais barato" });
      await expect(melhor).toContainText("ME no Anexo III");

      await page.setViewportSize({ width: 360, height: 740 });
      await page.goto("/cnpj");
      await page.waitForLoadState("networkidle");
      await semRolagemHorizontal(page);
      expect(violacoes).toEqual([]);
    });
    test("fase 6: ligar a IA com a senha e perguntar às finanças", async ({ page }) => {
      const violacoes: string[] = [];
      vigiarCSP(page, violacoes);

      await page.goto("/entrar");
      await page.getByLabel("E-mail").fill("ana@teste.com");
      await page.getByLabel("Senha").fill(senha);
      await page.getByRole("button", { name: "Continuar" }).click();
      await page.getByRole("button", { name: "Usar código de recuperação" }).click();
      await page.getByLabel("Código de recuperação").fill(codigosAna[5] ?? "");
      await page.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByRole("heading", { level: 1 })).toContainText("Ana");

      // desligada: o chat manda para a configuração
      await page.goto("/perguntar");
      await expect(page.getByText("A IA está desligada para você.")).toBeVisible();
      await page.getByRole("link", { name: "Ver configuração" }).click();

      // ligar pede a senha de novo
      await expect(page.getByRole("heading", { name: "Inteligência artificial" })).toBeVisible();
      await page.getByRole("button", { name: "Ligar IA" }).click();
      const reauth = page.getByRole("dialog", { name: "Confirme sua senha" });
      await reauth.getByLabel("Senha").fill(senha);
      await reauth.getByRole("button", { name: "Confirmar" }).click();
      await expect(page.getByRole("button", { name: "Desligar" })).toBeVisible();
      await page.getByRole("button", { name: "Categorizar agora" }).click();
      await expect(page.getByText(/transações ganharam categoria/)).toBeVisible();

      // pergunta pela sugestão: a resposta vem da ferramenta de gastos por categoria
      await page.getByRole("link", { name: "Perguntar às minhas finanças" }).click();
      await page.getByRole("button", { name: "Quanto gastei por categoria este mês?" }).click();
      await expect(page.getByText(/Resposta de teste com os números da ferramenta/)).toBeVisible();
      await expect(page.getByText("gastos por categoria", { exact: true })).toBeVisible();
      await expect(page.getByText(/não substitui/)).toBeVisible();

      await page.setViewportSize({ width: 360, height: 740 });
      for (const rota of ["/perguntar", "/ia"]) {
        await page.goto(rota);
        await page.waitForLoadState("networkidle");
        await semRolagemHorizontal(page);
      }
      expect(violacoes).toEqual([]);
    });
  });
