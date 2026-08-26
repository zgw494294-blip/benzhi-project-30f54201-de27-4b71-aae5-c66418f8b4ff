package domain

import "time"

type CertificatePayload struct {
	CertificateID      string         `json:"certificateID"`
	BatchID            string         `json:"batchID"`
	CableSection       string         `json:"cableSection"`
	CircuitName        string         `json:"circuitName"`
	TestOwner          string         `json:"testOwner"`
	CertificateVersion string         `json:"certificateVersion"`
	ReviewerA          string         `json:"reviewerA"`
	ReviewerB          string         `json:"reviewerB"`
	ScopeDigest        string         `json:"scopeDigest"`
	MeasurementCount   int            `json:"measurementCount"`
	DeviationCount     int            `json:"deviationCount"`
	EvidenceDigest     string         `json:"evidenceDigest"`
	EvidenceListDigest string         `json:"evidenceListDigest"`
	IssuedAt           time.Time      `json:"issuedAt"`
	Reviews            []SafetyReview `json:"reviews"`
}

type Certificate struct {
	ID                 string             `json:"id"`
	BatchID            string             `json:"batchID"`
	CertificateVersion string             `json:"certificateVersion"`
	ReviewerA          string             `json:"reviewerA"`
	ReviewerB          string             `json:"reviewerB"`
	EvidenceDigest     string             `json:"evidenceDigest"`
	IssuedAt           time.Time          `json:"issuedAt"`
	SealedPayload      CertificatePayload `json:"sealedPayload"`
}

func (c Certificate) Verify() bool {
	digest, err := digestCertificatePayload(c.SealedPayload)
	return err == nil && digest == c.EvidenceDigest && c.SealedPayload.CertificateID == c.ID &&
		c.SealedPayload.BatchID == c.BatchID && c.SealedPayload.CertificateVersion == c.CertificateVersion
}
