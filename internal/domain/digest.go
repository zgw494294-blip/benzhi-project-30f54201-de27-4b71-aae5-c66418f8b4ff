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

// sealedEvidenceDigest 在签发时计算证据摘要。IssueCertificateWithEvidence 先调用
// EvidenceDigest 取摘要，再通过 touch 推进批次版本，因此封存载荷记录的是签发前
// 版本的摘要；复算时需要使用当前版本减一才能对应封存时刻的证据状态。
func sealedEvidenceDigest(batch *Batch) (string, error) {
	if batch == nil || batch.Version < 1 {
		return "", invalid("version", "批次版本不足以复算封存证据摘要")
	}
	clone := *batch
	clone.Version = batch.Version - 1
	return EvidenceDigest(&clone)
}

// SealedEvidenceValid 重新计算封存时的证据摘要并与证书载荷记录的摘要比较。
// 签发后任何对测量记录、偏差或复核内容的篡改都会使复算结果与封存摘要不一致。
func SealedEvidenceValid(batch *Batch) (bool, error) {
	if batch == nil || batch.Certificate == nil {
		return false, nil
	}
	sealedDigest := batch.Certificate.SealedPayload.EvidenceDigest
	if sealedDigest == "" {
		return false, nil
	}
	current, err := sealedEvidenceDigest(batch)
	if err != nil {
		return false, err
	}
	return current == sealedDigest, nil
}
