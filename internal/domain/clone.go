package domain

import "encoding/json"

func CloneBatch(batch *Batch) (*Batch, error) {
	encoded, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}
	var clone Batch
	if err := json.Unmarshal(encoded, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func ValidateRestoredBatch(batch *Batch) error {
	if batch == nil || batch.ID == "" || batch.Version < 1 || batch.CreatedAt.IsZero() {
		return invalid("snapshot", "批次快照缺少必要字段")
	}
	if batch.Status != StatusDraft {
		if err := ValidatePoints(batch.FrozenScope.Points); err != nil {
			return err
		}
		digest, err := digestValue(canonicalPoints(batch.FrozenScope.Points))
		if err != nil || digest != batch.FrozenScope.ScopeDigest {
			return invalid("scopeDigest", "冻结边界校验摘要不匹配")
		}
	}
	if batch.Status == StatusSealed {
		if batch.Certificate == nil || !batch.Certificate.Verify() {
			return invalid("certificate", "封存证书校验失败")
		}
		if batch.VerificationPackage == nil || len(batch.VerificationPackage.ValidateDigests()) > 0 {
			return invalid("verificationPackage", "复归核验包校验失败")
		}
		if valid, err := SealedEvidenceValid(batch); err != nil {
			return invalid("evidenceDigest", "封存证据摘要复算失败："+err.Error())
		} else if !valid {
			return invalid("evidenceDigest", "封存证据摘要与当前证据内容不一致")
		}
	}
	return nil
}
