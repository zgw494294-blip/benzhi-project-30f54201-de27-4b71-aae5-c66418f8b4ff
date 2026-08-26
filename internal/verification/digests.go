package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"isolation-chamber-commissioning/internal/domain"
)

func EvidenceDigest(run domain.TestRun) (string, error) {
	copy := run
	copy.EvidenceDigest = ""
	b, err := canonical(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func SnapshotDigest(snapshot domain.Snapshot) (string, error) {
	b, err := canonical(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func CredentialDigest(id, caseID, caseNumber, approvedBy string, approvedAt time.Time, snapshotDigest string, schema int) (string, error) {
	payload := struct {
		ID             string `json:"id"`
		CaseID         string `json:"caseId"`
		CaseNumber     string `json:"caseNumber"`
		ApprovedBy     string `json:"approvedBy"`
		ApprovedAt     string `json:"approvedAt"`
		SnapshotDigest string `json:"snapshotDigest"`
		SchemaVersion  int    `json:"schemaVersion"`
	}{id, caseID, caseNumber, approvedBy, approvedAt.UTC().Format(time.RFC3339Nano), snapshotDigest, schema}
	b, err := canonical(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func VerifyCredential(c domain.ActivationCredential, snapshot domain.Snapshot) (bool, string, error) {
	sd, err := SnapshotDigest(snapshot)
	if err != nil {
		return false, "", err
	}
	cd, err := CredentialDigest(c.ID, c.CaseID, c.CaseNumber, c.ApprovedBy, c.ApprovedAt, sd, c.SchemaVersion)
	if err != nil {
		return false, "", err
	}
	return sd == c.SnapshotDigest && cd == c.CredentialDigest, cd, nil
}

func canonical(v any) ([]byte, error) { return json.Marshal(v) }

func RetestScope(run domain.TestRun, protocol domain.TestProtocol) ([]string, error) {
	if run.Verdict != domain.VerdictFail {
		return nil, domain.State("合格测试不需要复测范围")
	}
	for _, cp := range protocol.Checkpoints {
		if cp.ID == run.CheckpointID {
			return []string{cp.ID}, nil
		}
	}
	return nil, domain.Validation("checkpointId", "失败证据不属于当前方案")
}

func FreezeChecks(l domain.AcceptanceLimits) []domain.FreezeCheck {
	return []domain.FreezeCheck{
		{CheckpointID: "pressure", Kind: domain.KindPressure, Name: "舱内外压差持续性", Sequence: 1, Measurements: []string{"pressurePa"}, Units: []string{"Pa"}, SamplingWindow: fmt.Sprintf("至少 %d 秒", l.PressureDurationSec), Limits: []string{fmt.Sprintf("最低压差 ≥ %.2f Pa", l.PressureMinPa)}},
		{CheckpointID: "airtightness", Kind: domain.KindAirtightness, Name: "围护结构气密性", Sequence: 2, Measurements: []string{"leakagePercent"}, Units: []string{"%"}, SamplingWindow: "单次稳定读数", Limits: []string{fmt.Sprintf("泄漏率 ≤ %.2f%%", l.MaxLeakagePercent)}},
		{CheckpointID: "interlock", Kind: domain.KindInterlock, Name: "门禁互锁响应", Sequence: 3, Measurements: []string{"responseSec", "bothDoorsOpen"}, Units: []string{"s", "boolean"}, SamplingWindow: "完整开门动作周期", Limits: []string{fmt.Sprintf("响应时间 ≤ %.2f 秒", l.InterlockResponseSec), "双门不得同时开启"}},
		{CheckpointID: "recovery", Kind: domain.KindRecovery, Name: "净化恢复能力", Sequence: 4, Measurements: []string{"recoveryMinutes", "particleCount"}, Units: []string{"min", "particles/m3"}, SamplingWindow: "扰动结束至目标浓度", Limits: []string{fmt.Sprintf("恢复时间 ≤ %.2f 分钟", l.RecoveryMaxMinutes), fmt.Sprintf("粒子浓度 ≤ %.2f particles/m3", l.RecoveryTargetParticles)}},
	}
}

func ProtocolPreviewDigest(caseID string, caseVersion int, ruleVersion string, limits domain.AcceptanceLimits, checkpoints []domain.Checkpoint) (string, error) {
	payload := struct {
		CaseID         string                  `json:"caseId"`
		CaseVersion    int                     `json:"caseVersion"`
		RuleSetVersion string                  `json:"ruleSetVersion"`
		Limits         domain.AcceptanceLimits `json:"limits"`
		Checkpoints    []domain.Checkpoint     `json:"checkpoints"`
	}{caseID, caseVersion, ruleVersion, limits, checkpoints}
	b, err := canonical(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
