package domain

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

var whitespace = regexp.MustCompile(`\s+`)

type InstrumentEvidence struct {
	ID                    string           `json:"id"`
	CertificateNumber     string           `json:"certificateNumber"`
	CalibrationValidFrom  time.Time        `json:"calibrationValidFrom"`
	CalibrationValidUntil time.Time        `json:"calibrationValidUntil"`
	ApplicableKinds       []CheckpointKind `json:"applicableKinds"`
	CalibrationStatus     string           `json:"calibrationStatus"`
}

type RiskLevel string

const (
	RiskLow    RiskLevel = "LOW"
	RiskMedium RiskLevel = "MEDIUM"
	RiskHigh   RiskLevel = "HIGH"
)

type DeviationAssessment struct {
	SuggestedRisk RiskLevel
	Basis         []string
	Scope         []string
	ScopeSource   string
}

type FreezeCheck struct {
	CheckpointID    string         `json:"checkpointId"`
	Kind            CheckpointKind `json:"kind"`
	Name            string         `json:"name"`
	Sequence        int            `json:"sequence"`
	Measurements    []string       `json:"measurements"`
	Units           []string       `json:"units"`
	SamplingWindow  string         `json:"samplingWindow"`
	Limits          []string       `json:"limits"`
	BlockingReasons []string       `json:"blockingReasons,omitempty"`
}

type FreezeConfirmation struct {
	Token           string        `json:"confirmationToken"`
	CaseVersion     int           `json:"caseVersion"`
	RuleSetVersion  string        `json:"ruleSetVersion"`
	CheckpointOrder []string      `json:"checkpointOrder"`
	Checks          []FreezeCheck `json:"checks"`
	PreviewSummary  string        `json:"previewSummary"`
	CreatedAt       time.Time     `json:"createdAt"`
	CreatedBy       string        `json:"createdBy"`
	UsedAt          *time.Time    `json:"usedAt,omitempty"`
}

type ChecklistItem struct {
	ID           string   `json:"id"`
	Group        string   `json:"group"`
	Label        string   `json:"label"`
	CheckpointID string   `json:"checkpointId,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}

type ChecklistAnswer struct {
	ItemID    string `json:"itemId"`
	Confirmed bool   `json:"confirmed"`
}

type ReviewIssue struct {
	ItemID                string   `json:"itemId"`
	Category              string   `json:"category"`
	Reason                string   `json:"reason"`
	AffectedCheckpointIDs []string `json:"affectedCheckpointIds"`
}

type ReviewRound struct {
	Version          int               `json:"version"`
	BoundCaseVersion int               `json:"boundCaseVersion"`
	RuleSetVersion   string            `json:"ruleSetVersion"`
	Items            []ChecklistItem   `json:"items"`
	Answers          []ChecklistAnswer `json:"answers,omitempty"`
	Issues           []ReviewIssue     `json:"issues,omitempty"`
	IssueResolutions map[string]string `json:"issueResolutions,omitempty"`
	SubmittedAt      time.Time         `json:"submittedAt"`
	SubmittedBy      string            `json:"submittedBy"`
	DecidedAt        *time.Time        `json:"decidedAt,omitempty"`
	Reviewer         string            `json:"reviewer,omitempty"`
	Decision         string            `json:"decision,omitempty"`
}

type BaseRevision struct {
	ChamberName      string
	Zones            []ZoneBoundary
	AirflowDirection string
	Limits           AcceptanceLimits
}

func normalizeName(value string) string {
	return whitespace.ReplaceAllString(strings.TrimSpace(value), " ")
}

func NormalizeBaseData(chamber string, zones []ZoneBoundary, airflow string, limits AcceptanceLimits) (BaseRevision, error) {
	chamber = normalizeName(chamber)
	if chamber == "" {
		return BaseRevision{}, Validation("chamberName", "隔离舱名称不能为空")
	}
	if len(zones) == 0 {
		return BaseRevision{}, Validation("zones", "至少登记一个相邻区域")
	}
	out := make([]ZoneBoundary, 0, len(zones))
	seen := map[string]bool{}
	areas := map[string]string{strings.ToLower(chamber): chamber}
	for i, zone := range zones {
		left, right := normalizeName(zone.Chamber), normalizeName(zone.Adjacent)
		if left == "" || right == "" {
			return BaseRevision{}, Validation(fmt.Sprintf("zones[%d]", i), "边界两侧名称均不能为空")
		}
		if !strings.EqualFold(left, chamber) {
			return BaseRevision{}, Validation(fmt.Sprintf("zones[%d].chamber", i), "边界的舱室名称必须与案卷隔离舱一致")
		}
		if strings.EqualFold(left, right) {
			return BaseRevision{}, Validation(fmt.Sprintf("zones[%d].adjacent", i), "隔离舱与相邻区域不能相同")
		}
		key := strings.ToLower(left + "|" + right)
		if seen[key] {
			return BaseRevision{}, Validation(fmt.Sprintf("zones[%d]", i), "不能登记重复边界")
		}
		seen[key] = true
		areas[strings.ToLower(right)] = right
		out = append(out, ZoneBoundary{Chamber: chamber, Adjacent: right})
	}
	airflow = normalizeName(strings.ReplaceAll(strings.ReplaceAll(airflow, "->", "→"), "➜", "→"))
	parts := strings.Split(airflow, "→")
	if len(parts) != 2 {
		return BaseRevision{}, Validation("airflowDirection", "气流方向必须以“已登记区域 → 已登记区域”表示")
	}
	from, to := normalizeName(parts[0]), normalizeName(parts[1])
	if strings.EqualFold(from, to) {
		return BaseRevision{}, Validation("airflowDirection", "气流起点与终点不能相同")
	}
	if _, ok := areas[strings.ToLower(from)]; !ok {
		return BaseRevision{}, Validation("airflowDirection", "气流起点未引用已登记区域")
	}
	if _, ok := areas[strings.ToLower(to)]; !ok {
		return BaseRevision{}, Validation("airflowDirection", "气流终点未引用已登记区域")
	}
	if err := limits.Validate(); err != nil {
		return BaseRevision{}, err
	}
	return BaseRevision{ChamberName: chamber, Zones: out, AirflowDirection: from + " → " + to, Limits: limits}, nil
}

func revisionFields(c *CommissioningCase, r BaseRevision) []string {
	var fields []string
	if c.ChamberName != r.ChamberName {
		fields = append(fields, "chamberName")
	}
	if !reflect.DeepEqual(c.Zones, r.Zones) {
		fields = append(fields, "zones")
	}
	if c.AirflowDirection != r.AirflowDirection {
		fields = append(fields, "airflowDirection")
	}
	if c.AcceptanceLimits != r.Limits {
		fields = append(fields, "acceptanceLimits")
	}
	return fields
}

func (c *CommissioningCase) ReviseBase(expected int, revision BaseRevision, actor string, now time.Time) ([]string, error) {
	if c.Status != StatusDraft {
		return nil, State("只有草拟状态可修订基础资料，方案冻结后受保护字段不可直接修改")
	}
	if expected != c.Version {
		return nil, RevisionConflict(expected, c.Version, revisionFields(c, revision))
	}
	fields := revisionFields(c, revision)
	if len(fields) == 0 {
		return nil, Validation("revision", "基础资料没有变化")
	}
	diffs := map[string]any{}
	for _, field := range fields {
		switch field {
		case "chamberName":
			diffs[field] = map[string]any{"before": c.ChamberName, "after": revision.ChamberName}
		case "zones":
			diffs[field] = map[string]any{"before": c.Zones, "after": revision.Zones}
		case "airflowDirection":
			diffs[field] = map[string]any{"before": c.AirflowDirection, "after": revision.AirflowDirection}
		case "acceptanceLimits":
			diffs[field] = map[string]any{"before": c.AcceptanceLimits, "after": revision.Limits}
		}
	}
	c.ChamberName, c.Zones, c.AirflowDirection, c.AcceptanceLimits = revision.ChamberName, revision.Zones, revision.AirflowDirection, revision.Limits
	c.mutate(now)
	c.record("CASE_REVISED", actor, "修订草拟案卷基础资料", now, map[string]any{"fields": fields, "diffs": diffs})
	return fields, nil
}

func (c *CommissioningCase) AddFreezeConfirmation(confirmation FreezeConfirmation, actor string, now time.Time) error {
	if c.Status != StatusDraft {
		return State("只有草拟状态可生成冻结预检")
	}
	if confirmation.CaseVersion != c.Version {
		return Conflict(confirmation.CaseVersion, c.Version)
	}
	c.FreezeConfirmations = append(c.FreezeConfirmations, confirmation)
	c.record("FREEZE_PREFLIGHTED", actor, "完成方案冻结预检", now, map[string]any{"confirmationToken": confirmation.Token, "previewSummary": confirmation.PreviewSummary})
	return nil
}

func (c *CommissioningCase) UseFreezeConfirmation(expected int, token, preview string, actor string, now time.Time) error {
	var confirmation *FreezeConfirmation
	for i := range c.FreezeConfirmations {
		if c.FreezeConfirmations[i].Token == strings.TrimSpace(token) {
			confirmation = &c.FreezeConfirmations[i]
			break
		}
	}
	if confirmation == nil {
		return Validation("confirmationToken", "冻结确认标识不存在")
	}
	if confirmation.UsedAt != nil {
		return State("冻结确认标识已使用")
	}
	if confirmation.CaseVersion != c.Version {
		return RevisionConflict(confirmation.CaseVersion, c.Version, []string{"baseData"})
	}
	if expected != c.Version {
		return Conflict(expected, c.Version)
	}
	if confirmation.PreviewSummary != preview {
		return State("预检摘要与当前方案不一致")
	}
	if err := c.FreezeProtocol(expected, actor, now, DefaultCheckpoints(), confirmation.RuleSetVersion); err != nil {
		return err
	}
	used := now.UTC()
	confirmation.UsedAt = &used
	c.Protocol.Digest = preview
	c.record("FREEZE_CONFIRMED", actor, "使用预检确认标识冻结方案", now, map[string]any{"confirmationToken": token, "previewSummary": preview})
	return nil
}

func (c *CommissioningCase) ValidateInstrument(in InstrumentEvidence, kind CheckpointKind, completedAt time.Time) (InstrumentEvidence, error) {
	in.ID = strings.TrimSpace(in.ID)
	in.CertificateNumber = strings.TrimSpace(in.CertificateNumber)
	if in.ID == "" {
		return in, Validation("instrumentId", "仪器编号不能为空")
	}
	if in.CertificateNumber == "" {
		return in, Validation("instrument.certificateNumber", "校准证书编号不能为空")
	}
	if in.CalibrationValidFrom.IsZero() {
		return in, Validation("instrument.calibrationValidFrom", "校准生效时间无效")
	}
	if in.CalibrationValidUntil.IsZero() || in.CalibrationValidUntil.Before(in.CalibrationValidFrom) {
		return in, Validation("instrument.calibrationValidUntil", "校准有效期无效")
	}
	in.CalibrationValidFrom, in.CalibrationValidUntil = in.CalibrationValidFrom.UTC(), in.CalibrationValidUntil.UTC()
	if completedAt.Before(in.CalibrationValidFrom) {
		return in, Validation("instrument.calibrationValidFrom", "测试完成时校准尚未生效")
	}
	if completedAt.After(in.CalibrationValidUntil) {
		return in, Validation("instrument.calibrationValidUntil", "测试完成时校准已经过期")
	}
	found := false
	unique := map[CheckpointKind]bool{}
	var kinds []CheckpointKind
	for _, applicable := range in.ApplicableKinds {
		if !containsKind(RequiredKinds, applicable) {
			return in, Validation("instrument.applicableKinds", "仪器适用类型无效")
		}
		if !unique[applicable] {
			unique[applicable] = true
			kinds = append(kinds, applicable)
		}
		if applicable == kind {
			found = true
		}
	}
	if !found {
		return in, Validation("instrument.applicableKinds", "仪器不适用于当前检查点类型")
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	in.ApplicableKinds, in.CalibrationStatus = kinds, "VALID"
	for _, run := range c.Runs {
		if !strings.EqualFold(run.InstrumentID, in.ID) {
			continue
		}
		existing := run.Instrument
		if existing.CertificateNumber != in.CertificateNumber || !existing.CalibrationValidFrom.Equal(in.CalibrationValidFrom) || !existing.CalibrationValidUntil.Equal(in.CalibrationValidUntil) {
			return in, Validation("instrument", "同一仪器编号的校准证书或有效期与既有证据冲突")
		}
	}
	return in, nil
}

func containsKind(items []CheckpointKind, value CheckpointKind) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func (c *CommissioningCase) AddRunWithAssessment(expected int, run TestRun, assessment DeviationAssessment, actor string, now time.Time) error {
	before := len(c.Deviations)
	if err := c.AddRun(expected, run, actor, now); err != nil {
		return err
	}
	if run.Verdict == VerdictFail && len(c.Deviations) == before+1 {
		d := &c.Deviations[len(c.Deviations)-1]
		d.SuggestedRisk, d.RiskLevel = assessment.SuggestedRisk, assessment.SuggestedRisk
		d.RiskBasis = append([]string(nil), assessment.Basis...)
		d.RetestCheckpointIDs = append([]string(nil), assessment.Scope...)
		d.RetestScopeSource = assessment.ScopeSource
		c.record("DEVIATION_CLASSIFIED", actor, "计算建议风险等级并锁定定向复测范围", now, map[string]any{"deviationId": d.ID, "suggestedRisk": d.SuggestedRisk, "basis": d.RiskBasis, "scope": d.RetestCheckpointIDs, "scopeSource": d.RetestScopeSource})
	}
	return nil
}

func (c *CommissioningCase) RemediateDeviationWithDisposition(expected int, deviationID, rootCause, action, owner string, dueAt time.Time, risk RiskLevel, overrideReason, actor string, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if strings.TrimSpace(owner) == "" {
		return Validation("owner", "偏差责任人不能为空")
	}
	if dueAt.IsZero() || dueAt.Before(now) {
		return Validation("dueAt", "完成期限不得早于当前时间")
	}
	var target *Deviation
	for i := range c.Deviations {
		if c.Deviations[i].ID == deviationID {
			target = &c.Deviations[i]
			break
		}
	}
	if target == nil {
		return NotFound("偏差", deviationID)
	}
	if target.Status != DeviationOpen {
		return State("只有未处置偏差可提交整改")
	}
	if strings.TrimSpace(rootCause) == "" {
		return Validation("rootCause", "根因不能为空")
	}
	if strings.TrimSpace(action) == "" {
		return Validation("correctiveAction", "纠正措施不能为空")
	}
	if risk == "" {
		risk = target.SuggestedRisk
	}
	if risk != RiskLow && risk != RiskMedium && risk != RiskHigh {
		return Validation("riskLevel", "风险等级必须为 LOW、MEDIUM 或 HIGH")
	}
	if risk != target.SuggestedRisk && strings.TrimSpace(overrideReason) == "" {
		return Validation("riskOverrideReason", "调整建议风险等级时必须填写理由")
	}
	target.RootCause, target.CorrectiveAction = strings.TrimSpace(rootCause), strings.TrimSpace(action)
	target.Owner, target.RiskLevel, target.RiskOverrideReason = strings.TrimSpace(owner), risk, strings.TrimSpace(overrideReason)
	due := dueAt.UTC()
	target.DueAt = &due
	target.Status = DeviationRemediated
	c.Status = StatusTesting
	c.mutate(now)
	c.record("DEVIATION_REMEDIATED", actor, "记录偏差分级、责任人与整改期限，生成定向复测", now, map[string]any{"deviationId": deviationID, "riskLevel": risk, "owner": target.Owner, "dueAt": due, "scope": target.RetestCheckpointIDs, "scopeSource": target.RetestScopeSource})
	return nil
}

func (d Deviation) ProjectionStatus(now time.Time) string {
	if d.Status == DeviationClosed {
		return "CLOSED"
	}
	if d.DueAt != nil && now.After(*d.DueAt) {
		return "OVERDUE"
	}
	if d.Status == DeviationRemediated {
		return "AWAITING_RETEST"
	}
	if d.DueAt != nil && d.DueAt.Sub(now) <= 72*time.Hour {
		return "DUE_SOON"
	}
	return "PENDING_ACTION"
}

func (c *CommissioningCase) SubmitReviewChecklist(expected int, actor string, items []ChecklistItem, resolutions map[string]string, ruleVersion string, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if (c.Status != StatusReady && c.Status != StatusReturned) || !c.allRequirementsPassed() {
		return State("全部检查点及定向复测合格后方可送审")
	}
	if len(items) == 0 {
		return Validation("checklist", "复核清单不能为空")
	}
	if len(c.ReviewRounds) > 0 {
		previous := c.ReviewRounds[len(c.ReviewRounds)-1]
		for _, issue := range previous.Issues {
			if strings.TrimSpace(resolutions[issue.ItemID]) == "" {
				return Validation("issueResolutions."+issue.ItemID, "再次送审必须填写原问题处理说明")
			}
		}
	}
	c.Status = StatusReview
	c.mutate(now)
	round := ReviewRound{Version: len(c.ReviewRounds) + 1, BoundCaseVersion: c.Version, RuleSetVersion: ruleVersion, Items: append([]ChecklistItem(nil), items...), IssueResolutions: resolutions, SubmittedAt: now.UTC(), SubmittedBy: actor}
	c.ReviewRounds = append(c.ReviewRounds, round)
	c.record("REVIEW_SUBMITTED", actor, fmt.Sprintf("提交第 %d 轮结构化安全复核", round.Version), now, map[string]any{"checklistVersion": round.Version, "itemCount": len(items)})
	return nil
}

func (c *CommissioningCase) ReviewChecklist(expected int, decision, reviewer string, answers []ChecklistAnswer, issues []ReviewIssue, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if c.Status != StatusReview || len(c.ReviewRounds) == 0 {
		return State("只有复核中案卷可作出清单决定")
	}
	if strings.TrimSpace(reviewer) == "" {
		return Validation("reviewer", "复核人不能为空")
	}
	round := &c.ReviewRounds[len(c.ReviewRounds)-1]
	if round.BoundCaseVersion != c.Version {
		return RevisionConflict(round.BoundCaseVersion, c.Version, []string{"reviewChecklist"})
	}
	known := map[string]ChecklistItem{}
	for _, item := range round.Items {
		known[item.ID] = item
	}
	seen := map[string]bool{}
	for _, answer := range answers {
		if _, ok := known[answer.ItemID]; !ok {
			return Validation("answers", "清单回答包含未知项目")
		}
		if seen[answer.ItemID] {
			return Validation("answers", "清单项目不能重复回答")
		}
		seen[answer.ItemID] = answer.Confirmed
	}
	decision = strings.ToUpper(strings.TrimSpace(decision))
	if decision == "APPROVE" {
		var missing []string
		for _, item := range round.Items {
			if !seen[item.ID] {
				missing = append(missing, item.ID)
			}
		}
		if len(missing) > 0 {
			return Validation("answers", "仍有未确认清单项目："+strings.Join(missing, ", "))
		}
	} else if decision == "RETURN" {
		if len(issues) == 0 {
			return Validation("issues", "退回时至少登记一个清单问题")
		}
		for i, issue := range issues {
			item, ok := known[issue.ItemID]
			if !ok {
				return Validation(fmt.Sprintf("issues[%d].itemId", i), "问题未关联有效清单项")
			}
			if strings.TrimSpace(issue.Category) == "" {
				return Validation(fmt.Sprintf("issues[%d].category", i), "问题类别不能为空")
			}
			if strings.TrimSpace(issue.Reason) == "" {
				return Validation(fmt.Sprintf("issues[%d].reason", i), "退回理由不能为空")
			}
			if len(issue.AffectedCheckpointIDs) == 0 {
				return Validation(fmt.Sprintf("issues[%d].affectedCheckpointIds", i), "必须填写受影响检查点")
			}
			for _, id := range issue.AffectedCheckpointIDs {
				if !c.hasCheckpoint(id) {
					return Validation(fmt.Sprintf("issues[%d].affectedCheckpointIds", i), "受影响检查点不属于冻结方案")
				}
			}
			if item.CheckpointID != "" && !contains(issue.AffectedCheckpointIDs, item.CheckpointID) {
				return Validation(fmt.Sprintf("issues[%d].affectedCheckpointIds", i), "问题范围必须包含清单项目对应检查点")
			}
		}
	} else {
		return Validation("decision", "复核决定必须为 APPROVE 或 RETURN")
	}
	round.Answers, round.Issues, round.Reviewer, round.Decision = append([]ChecklistAnswer(nil), answers...), append([]ReviewIssue(nil), issues...), strings.TrimSpace(reviewer), decision
	decided := now.UTC()
	round.DecidedAt = &decided
	return nil
}

func (c *CommissioningCase) hasCheckpoint(id string) bool {
	if c.Protocol == nil {
		return false
	}
	for _, cp := range c.Protocol.Checkpoints {
		if cp.ID == id {
			return true
		}
	}
	return false
}
