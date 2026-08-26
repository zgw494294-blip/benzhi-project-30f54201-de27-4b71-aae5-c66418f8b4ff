package application

import (
	"fmt"
	"strings"

	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

func (s *Service) FindBatchMatches(cableSection, circuitName string) (BatchMatches, error) {
	batches, err := s.repository.ListBatches()
	if err != nil {
		return BatchMatches{}, err
	}
	cableSection, circuitName = strings.TrimSpace(cableSection), strings.TrimSpace(circuitName)
	result := BatchMatches{}
	for _, batch := range batches {
		if !strings.EqualFold(batch.CableSection, cableSection) || !strings.EqualFold(batch.CircuitName, circuitName) {
			continue
		}
		summary := summarize(batch)
		if batch.Status != domain.StatusSealed {
			result.Active = append(result.Active, summary)
		} else if result.LatestSealed == nil {
			copySummary := summary
			result.LatestSealed = &copySummary
		}
	}
	return result, nil
}

func (s *Service) ListBatches() ([]BatchSummary, error) {
	batches, err := s.repository.ListBatches()
	if err != nil {
		return nil, err
	}
	result := make([]BatchSummary, 0, len(batches))
	for _, batch := range batches {
		result = append(result, summarize(batch))
	}
	return result, nil
}

func (s *Service) GetBatch(id string) (*BatchDetail, error) {
	batch, err := s.repository.GetBatch(id)
	if err != nil {
		return nil, err
	}
	timeline, err := s.repository.AuditEvents(id)
	if err != nil {
		return nil, err
	}
	allEvents, err := s.repository.AuditEvents("")
	if err != nil {
		return nil, err
	}
	sequence, hash := uint64(0), ""
	if len(allEvents) > 0 {
		sequence, hash = allEvents[len(allEvents)-1].Sequence, allEvents[len(allEvents)-1].Hash
	}
	reviewReadiness, err := batch.BuildReviewReadiness(sequence, hash, s.repository.Health() == nil)
	if err != nil {
		return nil, err
	}
	return &BatchDetail{Batch: batch, Summary: summarize(batch), Timeline: timeline,
		DiagnosisReadiness: batch.DiagnosisReadiness(), ReviewReadiness: reviewReadiness}, nil
}

func (s *Service) GetDiagnosisReadiness(batchID string) (domain.DiagnosisReadiness, error) {
	batch, err := s.repository.GetBatch(batchID)
	if err != nil {
		return domain.DiagnosisReadiness{}, err
	}
	return batch.DiagnosisReadiness(), nil
}

func (s *Service) GetReviewReadiness(batchID string) (domain.ReviewReadiness, error) {
	detail, err := s.GetBatch(batchID)
	if err != nil {
		return domain.ReviewReadiness{}, err
	}
	return detail.ReviewReadiness, nil
}

func (s *Service) GetCertificate(batchID string) (*domain.Certificate, error) {
	batch, err := s.repository.GetBatch(batchID)
	if err != nil {
		return nil, err
	}
	if batch.Certificate == nil {
		return nil, domain.ErrNotFound
	}
	certificate := *batch.Certificate
	return &certificate, nil
}

func (s *Service) VerifyCertificate(batchID string) (CertificateVerification, error) {
	batch, err := s.repository.GetBatch(batchID)
	if err != nil {
		return CertificateVerification{}, err
	}
	if batch.Certificate == nil || batch.VerificationPackage == nil {
		return CertificateVerification{}, domain.ErrNotFound
	}
	pkg := batch.VerificationPackage
	anomalies := pkg.ValidateDigests()
	anomalies = append(anomalies, domain.MissingEvidenceItems(batch, pkg.EvidenceList)...)
	events, auditErr := s.repository.AuditEvents("")
	bySequence := map[uint64]persistence.AuditEvent{}
	byHash := map[string]bool{}
	for _, event := range events {
		bySequence[event.Sequence] = event
		byHash[event.Hash] = true
	}
	if len(events) == 0 || events[0].Hash != pkg.AuditFirstHash {
		anomalies = append(anomalies, "审计链首摘要不一致")
	}
	if !byHash[pkg.AuditLastHash] {
		anomalies = append(anomalies, "签发证据对应的审计链末摘要缺失")
	}
	for _, reference := range pkg.EvidenceList.AuditReferences {
		event, ok := bySequence[reference.Sequence]
		if !ok {
			anomalies = append(anomalies, fmt.Sprintf("缺少审计序号 %d", reference.Sequence))
			continue
		}
		if event.Hash != reference.Hash || event.Operation != reference.Operation {
			anomalies = append(anomalies, fmt.Sprintf("审计序号 %d 的摘要或操作不一致", reference.Sequence))
		}
	}
	if auditErr != nil {
		anomalies = append(anomalies, "审计链校验失败："+auditErr.Error())
	}
	certificateValid := batch.Certificate.Verify()
	evidenceValid := pkg.EvidenceListDigest == batch.Certificate.SealedPayload.EvidenceListDigest && len(domain.MissingEvidenceItems(batch, pkg.EvidenceList)) == 0
	auditValid := auditErr == nil
	for _, anomaly := range anomalies {
		if strings.Contains(anomaly, "审计") {
			auditValid = false
		}
	}
	return CertificateVerification{BatchID: batchID, CertificateID: batch.Certificate.ID, CertificateVersion: batch.Certificate.CertificateVersion,
		Valid: len(anomalies) == 0 && certificateValid, CertificateValid: certificateValid, EvidenceListValid: evidenceValid,
		AuditValid: auditValid, EvidenceDigest: batch.Certificate.SealedPayload.EvidenceDigest,
		PayloadDigest: batch.Certificate.EvidenceDigest, ContentDigest: pkg.ContentDigest, Anomalies: anomalies, VerifiedAt: s.clock().UTC()}, nil
}

func (s *Service) GetVerificationPackage(batchID string) (*domain.VerificationPackage, error) {
	batch, err := s.repository.GetBatch(batchID)
	if err != nil {
		return nil, err
	}
	if batch.VerificationPackage == nil {
		return nil, domain.ErrNotFound
	}
	clone, err := domain.CloneBatch(batch)
	if err != nil {
		return nil, err
	}
	return clone.VerificationPackage, nil
}

func (s *Service) GetAuditTrail(batchID string) ([]persistence.AuditEvent, error) {
	if _, err := s.repository.GetBatch(batchID); err != nil {
		return nil, err
	}
	return s.repository.AuditEvents(batchID)
}

func (s *Service) Health() (HealthStatus, error) {
	if err := s.repository.Health(); err != nil {
		return HealthStatus{Status: "unhealthy", SchemaVersion: persistence.CurrentSchemaVersion}, err
	}
	return HealthStatus{Status: "ok", SchemaVersion: persistence.CurrentSchemaVersion}, nil
}
