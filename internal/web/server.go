package web

import (
	"net/http"
	"strings"

	"pdconsole/internal/application"
)

type Server struct {
	service *application.Service
	mux     *http.ServeMux
}

func NewServer(service *application.Service) *Server {
	server := &Server{service: service, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) Handler() http.Handler {
	return requestLogHeaders(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.HandleIndex)
	s.mux.HandleFunc("GET /assets/style.css", s.HandleCSS)
	s.mux.HandleFunc("GET /assets/app.js", s.HandleJS)
	s.mux.HandleFunc("GET /api/health", s.HandleHealth)
	s.mux.HandleFunc("GET /api/batches", s.HandleListBatches)
	s.mux.HandleFunc("GET /api/batches/matches", s.HandleBatchMatches)
	s.mux.HandleFunc("POST /api/batches", s.HandleCreateBatch)
	s.mux.HandleFunc("GET /api/batches/{batchID}", s.HandleGetBatch)
	s.mux.HandleFunc("POST /api/batches/{batchID}/freeze", s.HandleFreezeScope)
	s.mux.HandleFunc("POST /api/batches/{batchID}/freeze/preflight", s.HandleFreezePreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/measurements", s.HandleAddMeasurement)
	s.mux.HandleFunc("POST /api/batches/{batchID}/measurements/batch", s.HandleAddMeasurements)
	s.mux.HandleFunc("GET /api/batches/{batchID}/diagnosis-readiness", s.HandleDiagnosisReadiness)
	s.mux.HandleFunc("POST /api/batches/{batchID}/diagnose", s.HandleDiagnose)
	s.mux.HandleFunc("POST /api/batches/{batchID}/deviations/{deviationID}/correction", s.HandleCorrectDeviation)
	s.mux.HandleFunc("POST /api/batches/{batchID}/deviations/{deviationID}/retest", s.HandleEvaluateRetest)
	s.mux.HandleFunc("POST /api/batches/{batchID}/reviews", s.HandleSubmitReview)
	s.mux.HandleFunc("GET /api/batches/{batchID}/review-readiness", s.HandleReviewReadiness)
	s.mux.HandleFunc("POST /api/batches/{batchID}/issue", s.HandleIssueCertificate)
	s.mux.HandleFunc("GET /api/batches/{batchID}/certificate", s.HandleDownloadCertificate)
	s.mux.HandleFunc("GET /api/batches/{batchID}/certificate/verify", s.HandleVerifyCertificate)
	s.mux.HandleFunc("GET /api/batches/{batchID}/verification-package", s.HandleDownloadVerificationPackage)
	s.mux.HandleFunc("GET /api/batches/{batchID}/verification-package/verify", s.HandleVerifyCertificate)
	s.mux.HandleFunc("GET /api/batches/{batchID}/audit", s.HandleBatchAudit)
}

func requestLogHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "same-origin")
		writer.Header().Set("Cache-Control", "no-store")
		if strings.HasPrefix(request.URL.Path, "/api/") {
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		next.ServeHTTP(writer, request)
	})
}
