package web

import (
	"net/http"
)

func (s *Server) HandleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	serveEmbedded(writer, "static/index.html", "text/html; charset=utf-8")
}

func (s *Server) HandleCSS(writer http.ResponseWriter, _ *http.Request) {
	serveEmbedded(writer, "static/style.css", "text/css; charset=utf-8")
}

func (s *Server) HandleJS(writer http.ResponseWriter, _ *http.Request) {
	serveEmbedded(writer, "static/app.js", "text/javascript; charset=utf-8")
}

func serveEmbedded(writer http.ResponseWriter, name, contentType string) {
	content, err := staticFiles.ReadFile(name)
	if err != nil {
		http.Error(writer, "静态资源不存在", http.StatusNotFound)
		return
	}
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(content)
}

func (s *Server) HandleHealth(writer http.ResponseWriter, _ *http.Request) {
	status, err := s.service.Health()
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}
