package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func digestValue(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func digestCertificatePayload(payload CertificatePayload) (string, error) {
	return digestValue(payload)
}

func EvidenceDigest(batch *Batch) (string, error) {
	evidence := struct {
		BatchID      string         `json:"batchID"`
		Version      int64          `json:"version"`
		Scope        FrozenScope    `json:"scope"`
		Measurements []Measurement  `json:"measurements"`
		Deviations   []Deviation    `json:"deviations"`
		Reviews      []SafetyReview `json:"reviews"`
	}{batch.ID, batch.Version, batch.FrozenScope, batch.Measurements, batch.Deviations, batch.Reviews}
	return digestValue(evidence)
}
