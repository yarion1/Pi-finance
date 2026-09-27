// Service worker do Finanças: deixa o app abrir sem rede (a casca: HTML, JS, CSS,
// fontes e ícones). Nada da API é guardado: dado financeiro nunca fica no cache do
// aparelho. Também recebe as notificações push (fase 7).
const CACHE = "financas-casca-v2";
const CASCA = ["/", "/manifest.webmanifest", "/icone.svg", "/icone-192.png"];

self.addEventListener("install", (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(CASCA)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (e) => {
  e.waitUntil(
    caches
      .keys()
      .then((nomes) => Promise.all(nomes.filter((n) => n !== CACHE).map((n) => caches.delete(n))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (e) => {
  const req = e.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== self.location.origin || url.pathname.startsWith("/api/")) return;

  // arquivos com hash no nome não mudam: cache primeiro
  if (url.pathname.startsWith("/assets/")) {
    e.respondWith(
      caches.match(req).then(
        (achado) =>
          achado ||
          fetch(req).then((resp) => {
            if (resp.ok) {
              const copia = resp.clone();
              caches.open(CACHE).then((c) => c.put(req, copia));
            }
            return resp;
          }),
      ),
    );
    return;
  }

  // navegação: rede primeiro (a versão nova), sem rede a casca guardada
  if (req.mode === "navigate") {
    e.respondWith(
      fetch(req)
        .then((resp) => {
          if (resp.ok) {
            const copia = resp.clone();
            caches.open(CACHE).then((c) => c.put("/", copia));
          }
          return resp;
        })
        .catch(() => caches.match("/")),
    );
  }
});

// Push: {titulo, texto, url}. O servidor já tira os valores se a pessoa não quer vê-los
// na tela bloqueada.
self.addEventListener("push", (e) => {
  let d = {};
  try {
    d = e.data ? e.data.json() : {};
  } catch {
    d = {};
  }
  const url = typeof d.url === "string" && d.url.startsWith("/") && !d.url.startsWith("//") ? d.url : "/";
  e.waitUntil(
    self.registration.showNotification(d.titulo || "Finanças", {
      body: d.texto || "",
      icon: "/icone-192.png",
      badge: "/icone-192.png",
      lang: "pt-BR",
      tag: url,
      data: { url },
    }),
  );
});

// Toque na notificação: foca o painel aberto (e navega) ou abre um novo.
self.addEventListener("notificationclick", (e) => {
  e.notification.close();
  const url = new URL(e.notification.data?.url || "/", self.location.origin);
  if (url.origin !== self.location.origin) return;
  e.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((janelas) => {
      for (const j of janelas) {
        if (new URL(j.url).origin === self.location.origin && "focus" in j) {
          return j.focus().then((f) => (f && "navigate" in f ? f.navigate(url.href) : f));
        }
      }
      return self.clients.openWindow(url.href);
    }),
  );
});
