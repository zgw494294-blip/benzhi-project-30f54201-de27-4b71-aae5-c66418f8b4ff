package application

import (
	"time"

	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

type BatchSummary struct {
	ID                 string             `json:"id"`
	CableSection       string             `json:"cableSection"`
	CircuitName        string             `json:"circuitName"`
	TestOwner          string             `json:"testOwner"`
	Status             domain.BatchStatus `json:"status"`
	StatusLabel        string             `json:"statusLabel"`
	Version            int64              `json:"version"`
	MeasurementCount   int                `json:"measurementCount"`
	OpenDeviationCount int                `json:"openDeviationCount"`
	UpdatedAt          time.Time          `json:"updatedAt"`
}

type BatchDetail struct {
	Batch              *domain.Batch             `json:"batch"`
	Summary            BatchSummary              `json:"summary"`
	Timeline           []persistence.AuditEvent  `json:"timeline"`
	DiagnosisReadiness domain.DiagnosisReadiness `json:"diagnosisReadiness"`
	ReviewReadiness    domain.ReviewReadiness    `json:"reviewReadiness"`
}

type BatchMatches struct {
	Active       []BatchSummary `json:"active"`
	LatestSealed *BatchSummary  `json:"latestSealed,omitempty"`
}

type DiagnosisResult struct {
	BatchID            string             `json:"batchID"`
	Version            int64              `json:"version"`
	Status             domain.BatchStatus `json:"status"`
	CreatedDeviations  []domain.Deviation `json:"createdDeviations"`
	OpenDeviationCount int                `json:"openDeviationCount"`
}

type RetestResult struct {
	BatchID     string              `json:"batchID"`
	DeviationID string              `json:"deviationID"`
	Result      domain.RetestResult `json:"result"`
	Version     int64               `json:"version"`
	Status      domain.BatchStatus  `json:"status"`
}

type HealthStatus struct {
	Status        string `json:"status"`
	SchemaVersion int    `json:"schemaVersion"`
}

type CertificateVerification struct {
	BatchID            string    `json:"batchID"`
	CertificateID      string    `json:"certificateID"`
	CertificateVersion string    `json:"certificateVersion"`
	Valid              bool      `json:"valid"`
	EvidenceDigest     string    `json:"evidenceDigest"`
	PayloadDigest      string    `json:"payloadDigest"`
	VerifiedAt         time.Time `json:"verifiedAt"`
	CertificateValid   bool      `json:"certificateValid"`
	EvidenceListValid  bool      `json:"evidenceListValid"`
	AuditValid         bool      `json:"auditValid"`
	ContentDigest      string    `json:"contentDigest"`
	Anomalies          []string  `json:"anomalies"`
}

func summarize(batch *domain.Batch) BatchSummary {
	return BatchSummary{
		ID: batch.ID, CableSection: batch.CableSection, CircuitName: batch.CircuitName,
		TestOwner: batch.TestOwner, Status: batch.Status, StatusLabel: batch.Status.Label(),
		Version: batch.Version, MeasurementCount: len(batch.Measurements),
		OpenDeviationCount: batch.OpenDeviationCount(), UpdatedAt: batch.UpdatedAt,
	}
}
