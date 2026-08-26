package web

import (
	"net/http"

	"pdconsole/internal/application"
	"pdconsole/internal/domain"
)

// HandleGetBatch 返回聚合详情和已经过哈希链校验的操作时间线。
func (s *Server) HandleGetBatch(writer http.ResponseWriter, request *http.Request) {
	detail, err := s.service.GetBatch(request.PathValue("batchID"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, detail)
}

// HandleFreezeScope 校验量程和阈值，并永久冻结试验点边界。
func (s *Server) HandleFreezeScope(writer http.ResponseWriter, request *http.Request) {
	var command application.FreezeScopeCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.FreezeScope(request.PathValue("batchID"), command)
	writeBatchResult(writer, batch, err)
}

func (s *Server) HandleFreezePreflight(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Points []domain.TestPoint `json:"points"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	result, err := s.service.PreflightScope(body.Points)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

// HandleAddMeasurement 保存初始采样或指定为 retest 的定向复验读数。
func (s *Server) HandleAddMeasurement(writer http.ResponseWriter, request *http.Request) {
	var command application.AddMeasurementCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.AddMeasurement(request.PathValue("batchID"), command)
	writeBatchResult(writer, batch, err)
}

func (s *Server) HandleAddMeasurements(writer http.ResponseWriter, request *http.Request) {
	var command application.AddMeasurementsCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.AddMeasurements(request.PathValue("batchID"), command)
	writeBatchResult(writer, batch, err)
}

func (s *Server) HandleDiagnosisReadiness(writer http.ResponseWriter, request *http.Request) {
	result, err := s.service.GetDiagnosisReadiness(request.PathValue("batchID"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

// HandleDiagnose 运行冻结规则并保存定位到试验点的新偏差。
func (s *Server) HandleDiagnose(writer http.ResponseWriter, request *http.Request) {
	var command application.DiagnoseCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.Diagnose(request.PathValue("batchID"), command)
	writeBatchResult(writer, batch, err)
}

// HandleCorrectDeviation 登记整改措施、责任人和定向复验范围。
func (s *Server) HandleCorrectDeviation(writer http.ResponseWriter, request *http.Request) {
	var command application.CorrectDeviationCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.CorrectDeviation(request.PathValue("batchID"), request.PathValue("deviationID"), command)
	writeBatchResult(writer, batch, err)
}

// HandleEvaluateRetest 用冻结阈值重新评估某条偏差的定向复验读数。
func (s *Server) HandleEvaluateRetest(writer http.ResponseWriter, request *http.Request) {
	var command application.EvaluateRetestCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.EvaluateRetest(request.PathValue("batchID"), request.PathValue("deviationID"), command)
	writeBatchResult(writer, batch, err)
}

// HandleSubmitReview 锁定一名复核人的身份、意见和是否同意复归。
func (s *Server) HandleSubmitReview(writer http.ResponseWriter, request *http.Request) {
	var command application.SubmitReviewCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.SubmitReview(request.PathValue("batchID"), command)
	writeBatchResult(writer, batch, err)
}

func (s *Server) HandleReviewReadiness(writer http.ResponseWriter, request *http.Request) {
	result, err := s.service.GetReviewReadiness(request.PathValue("batchID"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

// HandleIssueCertificate 完成签发前完整性检查并封存证书。
func (s *Server) HandleIssueCertificate(writer http.ResponseWriter, request *http.Request) {
	var command application.IssueCertificateCommand
	if !decodeJSON(writer, request, &command) {
		return
	}
	batch, err := s.service.IssueCertificate(request.PathValue("batchID"), command)
	writeBatchResult(writer, batch, err)
}

// HandleDownloadCertificate 以附件形式返回不可变的证书 JSON。
func (s *Server) HandleDownloadCertificate(writer http.ResponseWriter, request *http.Request) {
	batchID := request.PathValue("batchID")
	certificate, err := s.service.GetCertificate(batchID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writer.Header().Set("Content-Disposition", `attachment; filename="release-certificate-`+batchID+`.json"`)
	writeJSON(writer, http.StatusOK, certificate)
}

// HandleVerifyCertificate 独立重算封存载荷摘要，供下载后核验。
func (s *Server) HandleVerifyCertificate(writer http.ResponseWriter, request *http.Request) {
	result, err := s.service.VerifyCertificate(request.PathValue("batchID"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) HandleDownloadVerificationPackage(writer http.ResponseWriter, request *http.Request) {
	batchID := request.PathValue("batchID")
	result, err := s.service.GetVerificationPackage(batchID)
	if err != nil {
		writeError(writer, err)
		return
	}
	writer.Header().Set("Content-Disposition", `attachment; filename="release-verification-`+batchID+`.json"`)
	writeJSON(writer, http.StatusOK, result)
}

// HandleBatchAudit 返回当前批次的审计序号、操作名和完整哈希字段。
func (s *Server) HandleBatchAudit(writer http.ResponseWriter, request *http.Request) {
	events, err := s.service.GetAuditTrail(request.PathValue("batchID"))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"events": events, "count": len(events)})
}

func writeBatchResult(writer http.ResponseWriter, batch any, err error) {
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, batch)
}
