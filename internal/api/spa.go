package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spa serve os arquivos do front; rotas desconhecidas caem no index.html
// (o roteamento é do lado do navegador).
func (s *Servidor) spa() http.Handler {
	if s.Front == nil {
		return http.HandlerFunc(semFront)
	}
	if _, err := fs.Stat(s.Front, "index.html"); err != nil {
		return http.HandlerFunc(semFront)
	}
	arquivos := http.FileServerFS(s.Front)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		caminho := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if caminho != "" && caminho != "index.html" {
			if info, err := fs.Stat(s.Front, caminho); err == nil && !info.IsDir() {
				if strings.HasPrefix(caminho, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				arquivos.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(caminho, "assets/") || path.Ext(caminho) != "" {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, s.Front, "index.html")
	})
}

func semFront(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("front não compilado: rode `npm run build` em web/"))
}
