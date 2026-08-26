package verification

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"isolation-chamber-commissioning/internal/domain"
)

const RuleSetVersion = "greenhouse-commissioning-rules/1.0"

type Input struct {
	CaseID           string
	ProtocolRevision int
	Checkpoint       domain.Checkpoint
	Limits           domain.AcceptanceLimits
	Measurements     []domain.Measurement
	InstrumentID     string
	Instrument       domain.InstrumentEvidence
	Witness          string
	StartedAt        time.Time
	CompletedAt      time.Time
	Attempt          int
	RunID            string
}

type Engine struct{}

func New() *Engine { return &Engine{} }

type RuleDefinition struct {
	Kind        domain.CheckpointKind `json:"kind"`
	Code        string                `json:"code"`
	Name        string                `json:"name"`
	Measurement string                `json:"measurement"`
	Comparator  string                `json:"comparator"`
	Unit        string                `json:"unit"`
	Description string                `json:"description"`
}

func RuleCatalog() []RuleDefinition {
	return []RuleDefinition{
		{domain.KindPressure, "PRESSURE_MIN", "持续窗口最低压差", "pressurePa", ">=", "Pa", "采样窗口内任一点压差不得低于冻结下限"},
		{domain.KindPressure, "PRESSURE_WINDOW", "压差采样持续窗口", "offsetSec", ">=", "s", "首末压差样本间隔必须覆盖冻结持续时间"},
		{domain.KindAirtightness, "LEAKAGE_MAX", "最大泄漏率", "leakagePercent", "<=", "%", "实测泄漏率不得超过冻结上限"},
		{domain.KindInterlock, "INTERLOCK_RESPONSE", "互锁响应时间", "responseSec", "<=", "s", "门禁互锁必须在冻结时限内动作"},
		{domain.KindInterlock, "INTERLOCK_EXCLUSION", "双门互斥", "bothDoorsOpen", "=", "boolean", "测试期间不得出现双门同时开启"},
		{domain.KindRecovery, "RECOVERY_TIME", "净化恢复时间", "recoveryMinutes", "<=", "min", "恢复时间不得超过冻结时限"},
		{domain.KindRecovery, "RECOVERY_PARTICLES", "恢复终点粒子浓度", "particleCount", "<=", "particles/m3", "终点粒子浓度不得超过冻结目标"},
	}
}

func (e *Engine) Evaluate(in Input) (domain.TestRun, error) {
	if in.Instrument.ID != "" {
		in.InstrumentID = in.Instrument.ID
	}
	if strings.TrimSpace(in.InstrumentID) == "" {
		return domain.TestRun{}, domain.Validation("instrumentId", "仪器编号不能为空")
	}
	if strings.TrimSpace(in.Witness) == "" {
		return domain.TestRun{}, domain.Validation("witness", "见证人不能为空")
	}
	if in.StartedAt.IsZero() || in.CompletedAt.IsZero() || in.CompletedAt.Before(in.StartedAt) {
		return domain.TestRun{}, domain.Validation("time", "测试起止时间无效")
	}
	if len(in.Measurements) == 0 {
		return domain.TestRun{}, domain.Validation("measurements", "至少提交一条原始测量")
	}
	measurements, err := normalizeMeasurements(in.Measurements)
	if err != nil {
		return domain.TestRun{}, err
	}
	reasons := evaluate(in.Checkpoint.Kind, in.Limits, measurements)
	criteria := buildCriteria(in.Checkpoint.Kind, in.Limits, measurements)
	verdict := domain.VerdictPass
	if len(reasons) > 0 {
		verdict = domain.VerdictFail
	}
	run := domain.TestRun{ID: in.RunID, CaseID: in.CaseID, ProtocolRevision: in.ProtocolRevision, CheckpointID: in.Checkpoint.ID, Attempt: in.Attempt, Measurements: measurements, InstrumentID: strings.TrimSpace(in.InstrumentID), Instrument: in.Instrument, Witness: strings.TrimSpace(in.Witness), StartedAt: in.StartedAt.UTC(), CompletedAt: in.CompletedAt.UTC(), Verdict: verdict, FailureReasons: reasons, Criteria: criteria, RuleSetVersion: RuleSetVersion}
	digest, err := EvidenceDigest(run)
	if err != nil {
		return domain.TestRun{}, err
	}
	run.EvidenceDigest = digest
	return run, nil
}

func normalizeMeasurements(items []domain.Measurement) ([]domain.Measurement, error) {
	out := append([]domain.Measurement(nil), items...)
	names := map[string]bool{}
	for i := range out {
		out[i].Name = strings.TrimSpace(out[i].Name)
		out[i].Unit = strings.TrimSpace(out[i].Unit)
		if out[i].Name == "" {
			return nil, domain.Validation("measurements.name", "测量名称不能为空")
		}
		if math.IsNaN(out[i].Value) || math.IsInf(out[i].Value, 0) {
			return nil, domain.Validation("measurements.value", "测量值必须为有限数字")
		}
		key := fmt.Sprintf("%s/%d", out[i].Name, out[i].OffsetSec)
		if names[key] {
			return nil, domain.Validation("measurements", "同一采样时点的测量名称不能重复")
		}
		names[key] = true
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OffsetSec == out[j].OffsetSec {
			return out[i].Name < out[j].Name
		}
		return out[i].OffsetSec < out[j].OffsetSec
	})
	return out, nil
}

func measurementsByName(items []domain.Measurement, name string) []domain.Measurement {
	var out []domain.Measurement
	for _, m := range items {
		if m.Name == name {
			out = append(out, m)
		}
	}
	return out
}

func require(items []domain.Measurement, name string) (domain.Measurement, bool) {
	for _, m := range items {
		if m.Name == name {
			return m, true
		}
	}
	return domain.Measurement{}, false
}
