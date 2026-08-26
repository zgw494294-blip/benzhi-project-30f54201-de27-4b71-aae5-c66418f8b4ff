package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	RulePassed       = "passed"
	RuleTriggered    = "triggered"
	RuleInsufficient = "insufficient"
)

type RuleReadiness struct {
	RuleCode  string `json:"ruleCode"`
	Evaluable bool   `json:"evaluable"`
	Reason    string `json:"reason,omitempty"`
}

type PointReadiness struct {
	PointID          string          `json:"pointID"`
	InitialCount     int             `json:"initialCount"`
	ValidRounds      []int           `json:"validRounds"`
	TimeSpanSeconds  int64           `json:"timeSpanSeconds"`
	SensorConsistent bool            `json:"sensorConsistent"`
	SensorSerials    []string        `json:"sensorSerials"`
	Rules            []RuleReadiness `json:"rules"`
	Blockers         []string        `json:"blockers"`
}

type DiagnosisReadiness struct {
	Ready                 bool             `json:"ready"`
	EvaluableRuleCount    int              `json:"evaluableRuleCount"`
	InsufficientRuleCount int              `json:"insufficientRuleCount"`
	Blockers              []string         `json:"blockers"`
	Points                []PointReadiness `json:"points"`
}

type RuleResult struct {
	PointID     string   `json:"pointID"`
	RuleCode    string   `json:"ruleCode"`
	Outcome     string   `json:"outcome"`
	Severity    Severity `json:"severity,omitempty"`
	FrozenLimit string   `json:"frozenLimit"`
	ActualValue string   `json:"actualValue"`
	EvidenceIDs []string `json:"evidenceIDs"`
	Explanation string   `json:"explanation"`
	DeviationID string   `json:"deviationID,omitempty"`
}

type RiskSummary struct {
	Severe       int `json:"severe"`
	Major        int `json:"major"`
	Notice       int `json:"notice"`
	Insufficient int `json:"insufficient"`
}

type DiagnosisReport struct {
	RunID           string       `json:"runID"`
	EvidenceVersion int64        `json:"evidenceVersion"`
	EvidenceDigest  string       `json:"evidenceDigest"`
	ExecutedAt      time.Time    `json:"executedAt"`
	Results         []RuleResult `json:"results"`
	Risk            RiskSummary  `json:"risk"`
}

func initialMeasurements(measurements []Measurement, pointID string) []Measurement {
	items := make([]Measurement, 0)
	for _, item := range measurements {
		if item.Purpose == "initial" && (pointID == "" || item.PointID == pointID) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].MeasuredAt.Equal(items[j].MeasuredAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].MeasuredAt.Before(items[j].MeasuredAt)
	})
	return items
}

func (b *Batch) DiagnosisEvidenceDigest() (string, error) {
	items := initialMeasurements(b.Measurements, "")
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return digestValue(struct {
		ScopeDigest  string        `json:"scopeDigest"`
		Measurements []Measurement `json:"measurements"`
	}{b.FrozenScope.ScopeDigest, items})
}

func (b *Batch) DiagnosisReadiness() DiagnosisReadiness {
	result := DiagnosisReadiness{Ready: true, Points: make([]PointReadiness, 0, len(b.FrozenScope.Points))}
	for _, point := range b.FrozenScope.Points {
		items := initialMeasurements(b.Measurements, point.ID)
		entry := PointReadiness{PointID: point.ID, InitialCount: len(items), SensorConsistent: true}
		roundSeen := map[int]string{}
		sensors := map[string]bool{}
		for _, item := range items {
			entry.ValidRounds = append(entry.ValidRounds, item.Round)
			sensors[item.SensorSerial] = true
			if prior, exists := roundSeen[item.Round]; exists {
				entry.Blockers = append(entry.Blockers, fmt.Sprintf("轮次 %d 重复（%s、%s）", item.Round, prior, item.ID))
			} else {
				roundSeen[item.Round] = item.ID
			}
			if item.MeasuredAt.Before(b.FrozenScope.FrozenAt) {
				entry.Blockers = append(entry.Blockers, fmt.Sprintf("记录 %s 的时间早于冻结时间", item.ID))
			}
		}
		if len(items) == 0 {
			entry.Blockers = append(entry.Blockers, "缺少初始采样，请补录至少一轮读数")
		}
		if len(items) > 1 {
			entry.TimeSpanSeconds = int64(items[len(items)-1].MeasuredAt.Sub(items[0].MeasuredAt).Seconds())
		}
		for serial := range sensors {
			entry.SensorSerials = append(entry.SensorSerials, serial)
		}
		sort.Strings(entry.SensorSerials)
		entry.SensorConsistent = len(entry.SensorSerials) <= 1
		entry.Rules = []RuleReadiness{
			{RuleCode: "PD_AMPLITUDE", Evaluable: len(items) >= 1, Reason: evidenceReason(len(items), 1)},
			{RuleCode: "PD_TREND", Evaluable: len(items) >= 2, Reason: evidenceReason(len(items), 2)},
			{RuleCode: "PD_REPEAT", Evaluable: len(items) >= point.RepeatabilityCount, Reason: evidenceReason(len(items), point.RepeatabilityCount)},
			{RuleCode: "ENV_CONDITION", Evaluable: len(items) >= 1, Reason: evidenceReason(len(items), 1)},
			{RuleCode: "PHASE_CLUSTER", Evaluable: len(items) >= point.RepeatabilityCount, Reason: evidenceReason(len(items), point.RepeatabilityCount)},
		}
		for _, rule := range entry.Rules {
			if rule.Evaluable {
				result.EvaluableRuleCount++
			} else {
				result.InsufficientRuleCount++
			}
		}
		if len(entry.Blockers) > 0 {
			result.Ready = false
			for _, blocker := range entry.Blockers {
				result.Blockers = append(result.Blockers, point.ID+"："+blocker)
			}
		}
		result.Points = append(result.Points, entry)
	}
	if len(b.FrozenScope.Points) == 0 {
		result.Ready = false
		result.Blockers = append(result.Blockers, "试验边界尚未冻结")
	}
	return result
}

func evidenceReason(actual, required int) string {
	if actual >= required {
		return "证据数量满足规则要求"
	}
	return fmt.Sprintf("需要 %d 轮，当前 %d 轮", required, actual)
}

func (b *Batch) FindDiagnosisReportByEvidence(digest string) *DiagnosisReport {
	for index := range b.DiagnosisReports {
		if b.DiagnosisReports[index].EvidenceDigest == digest {
			return &b.DiagnosisReports[index]
		}
	}
	return nil
}

func (b *Batch) RunDiagnosis(idFactory func() string, now time.Time) ([]Deviation, error) {
	before := len(b.Deviations)
	_, err := b.RunDiagnosisReport(fmt.Sprintf("diagnosis-%d", b.DiagnosisRun+1), idFactory, now)
	if err != nil {
		return nil, err
	}
	return append([]Deviation(nil), b.Deviations[before:]...), nil
}

func (b *Batch) RunDiagnosisReport(runID string, idFactory func() string, now time.Time) (*DiagnosisReport, error) {
	if err := b.ensureMutable(); err != nil {
		return nil, err
	}
	if b.Status == StatusDraft {
		return nil, invalid("measurements", "冻结并录入测量记录后才能诊断")
	}
	if len(b.Reviews) > 0 {
		return nil, ErrReviewLocked
	}
	readiness := b.DiagnosisReadiness()
	if !readiness.Ready {
		failures := make(ValidationErrors, 0, len(readiness.Blockers))
		for index, blocker := range readiness.Blockers {
			failures = append(failures, FieldError{Field: fmt.Sprintf("readiness.blockers[%d]", index), Message: blocker})
		}
		return nil, failures
	}
	digest, err := b.DiagnosisEvidenceDigest()
	if err != nil {
		return nil, err
	}
	if existing := b.FindDiagnosisReportByEvidence(digest); existing != nil {
		return existing, nil
	}
	report := DiagnosisReport{RunID: strings.TrimSpace(runID), EvidenceVersion: b.Version, EvidenceDigest: digest, ExecutedAt: now.UTC()}
	if report.RunID == "" {
		return nil, invalid("runID", "诊断运行编号不能为空")
	}
	for _, point := range b.FrozenScope.Points {
		items := initialMeasurements(b.Measurements, point.ID)
		report.Results = append(report.Results, evaluatePointRules(point, items)...)
	}
	b.DiagnosisRun++
	for index := range report.Results {
		result := &report.Results[index]
		if result.Outcome == RuleInsufficient {
			report.Risk.Insufficient++
			continue
		}
		if result.Outcome != RuleTriggered {
			continue
		}
		switch result.Severity {
		case SeveritySevere:
			report.Risk.Severe++
		case SeverityMajor:
			report.Risk.Major++
		case SeverityNotice:
			report.Risk.Notice++
		}
		if existing := b.openFinding(result.RuleCode, result.PointID); existing != nil {
			result.DeviationID = existing.ID
			continue
		}
		point, _ := pointByID(b.FrozenScope, result.PointID)
		deviation := Deviation{ID: idFactory(), BatchID: b.ID, RuleCode: result.RuleCode, Severity: result.Severity,
			PointID: result.PointID, Location: point.Location, Finding: result.Explanation,
			EvidenceIDs: append([]string(nil), result.EvidenceIDs...), CreatedAt: now.UTC(), DiagnosisRun: b.DiagnosisRun}
		b.Deviations = append(b.Deviations, deviation)
		result.DeviationID = deviation.ID
	}
	b.DiagnosisReports = append(b.DiagnosisReports, report)
	if b.OpenDeviationCount() > 0 {
		b.Status = StatusDiagnosed
	} else {
		b.Status = StatusReviewing
	}
	b.touch(now)
	return &b.DiagnosisReports[len(b.DiagnosisReports)-1], nil
}

func (b *Batch) openFinding(ruleCode, pointID string) *Deviation {
	for index := range b.Deviations {
		deviation := &b.Deviations[index]
		if deviation.RuleCode == ruleCode && deviation.PointID == pointID && !deviation.IsClosed() {
			return deviation
		}
	}
	return nil
}

func evaluatePointRules(point TestPoint, items []Measurement) []RuleResult {
	return []RuleResult{evaluateAmplitude(point, items), evaluateTrend(point, items), evaluateRepeat(point, items), evaluateEnvironment(point, items), evaluatePhase(point, items)}
}

func evidenceIDs(items []Measurement) []string {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	return ids
}

func evaluateAmplitude(point TestPoint, items []Measurement) RuleResult {
	r := RuleResult{PointID: point.ID, RuleCode: "PD_AMPLITUDE", FrozenLimit: fmt.Sprintf("≤ %.2f pC", point.AmplitudeLimitPC), EvidenceIDs: evidenceIDs(items)}
	if len(items) == 0 {
		r.Outcome = RuleInsufficient
		r.ActualValue = "无读数"
		r.Explanation = "缺少幅值读数，证据不足"
		return r
	}
	peak := items[0].PeakAmplitudePC
	for _, item := range items[1:] {
		if item.PeakAmplitudePC > peak {
			peak = item.PeakAmplitudePC
		}
	}
	r.ActualValue = fmt.Sprintf("最大 %.2f pC", peak)
	if peak > point.AmplitudeLimitPC {
		r.Outcome = RuleTriggered
		r.Severity = SeverityMajor
		if peak >= point.AmplitudeLimitPC*1.5 {
			r.Severity = SeveritySevere
		}
		r.Explanation = fmt.Sprintf("峰值 %.2f pC 超过冻结阈值 %.2f pC", peak, point.AmplitudeLimitPC)
	} else {
		r.Outcome = RulePassed
		r.Explanation = "全部有效读数未超过冻结幅值阈值"
	}
	return r
}

func evaluateTrend(point TestPoint, items []Measurement) RuleResult {
	r := RuleResult{PointID: point.ID, RuleCode: "PD_TREND", FrozenLimit: fmt.Sprintf("涨幅 ≤ %.1f%%", point.TrendLimitPercent), EvidenceIDs: evidenceIDs(items)}
	if len(items) < 2 {
		r.Outcome = RuleInsufficient
		r.ActualValue = fmt.Sprintf("%d 轮", len(items))
		r.Explanation = "趋势规则至少需要两轮有效读数"
		return r
	}
	first, last := items[0].PeakAmplitudePC, items[len(items)-1].PeakAmplitudePC
	increase := float64(0)
	if first > 0 {
		increase = (last - first) / first * 100
	} else if last > 0 {
		increase = 100
	}
	r.ActualValue = fmt.Sprintf("涨幅 %.1f%%", increase)
	if increase > point.TrendLimitPercent {
		r.Outcome = RuleTriggered
		r.Severity = SeverityMajor
		r.Explanation = fmt.Sprintf("局放幅值较首轮上升 %.1f%%，超过趋势阈值 %.1f%%", increase, point.TrendLimitPercent)
	} else {
		r.Outcome = RulePassed
		r.Explanation = "首末轮幅值变化未超过冻结趋势阈值"
	}
	return r
}

func evaluateRepeat(point TestPoint, items []Measurement) RuleResult {
	r := RuleResult{PointID: point.ID, RuleCode: "PD_REPEAT", FrozenLimit: fmt.Sprintf("少于 %d 轮达到阈值 90%%", point.RepeatabilityCount), EvidenceIDs: evidenceIDs(items)}
	if len(items) < point.RepeatabilityCount {
		r.Outcome = RuleInsufficient
		r.ActualValue = fmt.Sprintf("%d 轮", len(items))
		r.Explanation = fmt.Sprintf("重复性规则需要 %d 轮，当前证据不足", point.RepeatabilityCount)
		return r
	}
	count := 0
	for _, item := range items {
		if item.PeakAmplitudePC >= point.AmplitudeLimitPC*0.9 {
			count++
		}
	}
	r.ActualValue = fmt.Sprintf("%d 轮达到 90%%", count)
	if count >= point.RepeatabilityCount {
		r.Outcome = RuleTriggered
		r.Severity = SeverityMajor
		r.Explanation = fmt.Sprintf("连续 %d 次读数达到阈值的 90%%，重复性风险成立", count)
	} else {
		r.Outcome = RulePassed
		r.Explanation = "达到阈值 90% 的轮次数未构成重复性风险"
	}
	return r
}

func evaluateEnvironment(point TestPoint, items []Measurement) RuleResult {
	r := RuleResult{PointID: point.ID, RuleCode: "ENV_CONDITION", FrozenLimit: "温度 -10~55 ℃且湿度 ≤ 80%", EvidenceIDs: evidenceIDs(items)}
	if len(items) == 0 {
		r.Outcome = RuleInsufficient
		r.ActualValue = "无读数"
		r.Explanation = "缺少环境读数，证据不足"
		return r
	}
	bad := 0
	maxHumidity := float64(0)
	for _, item := range items {
		if item.HumidityPercent > maxHumidity {
			maxHumidity = item.HumidityPercent
		}
		if item.HumidityPercent > 80 || item.TemperatureC < -10 || item.TemperatureC > 55 {
			bad++
		}
	}
	r.ActualValue = fmt.Sprintf("异常 %d 轮，最高湿度 %.1f%%", bad, maxHumidity)
	if bad > 0 {
		r.Outcome = RuleTriggered
		r.Severity = SeverityNotice
		r.Explanation = "环境条件超出诊断建议范围"
	} else {
		r.Outcome = RulePassed
		r.Explanation = "温湿度均在诊断建议范围内"
	}
	return r
}

func evaluatePhase(point TestPoint, items []Measurement) RuleResult {
	r := RuleResult{PointID: point.ID, RuleCode: "PHASE_CLUSTER", FrozenLimit: fmt.Sprintf("%d 轮内不得持续集中尖峰", point.RepeatabilityCount), EvidenceIDs: evidenceIDs(items)}
	if len(items) < point.RepeatabilityCount {
		r.Outcome = RuleInsufficient
		r.ActualValue = fmt.Sprintf("%d 轮", len(items))
		r.Explanation = fmt.Sprintf("相位规则需要 %d 轮，当前证据不足", point.RepeatabilityCount)
		return r
	}
	count := 0
	for _, item := range items {
		summary := strings.ToLower(item.PhaseSummary)
		if strings.Contains(summary, "集中") || strings.Contains(summary, "尖峰") || strings.Contains(summary, "cluster") {
			count++
		}
	}
	r.ActualValue = fmt.Sprintf("%d 轮出现集中尖峰", count)
	if count >= point.RepeatabilityCount {
		r.Outcome = RuleTriggered
		r.Severity = SeverityMajor
		r.Explanation = "相位分布在足量采样中重复出现集中放电特征"
	} else {
		r.Outcome = RulePassed
		r.Explanation = "相位集中尖峰未达到冻结重复次数"
	}
	return r
}
