package web

import (
	"net/http"

	"pdconsole/internal/application"
)

func (s *Server) HandleListBatches(writer http.ResponseWriter, _ *http.Request) {
	batches, err := s.service.ListBatches()
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batches": batches})
}

func (s *Server) HandleBatchMatches(writer http.ResponseWriter, request *http.Request) {
	result, err := s.service.FindBatchMatches(request.URL.Query().Get("cableSection"), request.URL.Query().Get("circuitName"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) HandleCreateBatch(writer http.ResponseWriter, request *http.Request) {
	var command application.CreateBatchCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.CreateBatch(command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writer.Header().Set("Location", "/api/batches/"+batch.ID)
	writeJSON(writer, http.StatusCreated, batch)
}
