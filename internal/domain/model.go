package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type CaseStatus string

const (
	StatusDraft     CaseStatus = "DRAFT"
	StatusFrozen    CaseStatus = "PROTOCOL_FROZEN"
	StatusTesting   CaseStatus = "TESTING"
	StatusDeviation CaseStatus = "DEVIATION_OPEN"
	StatusReady     CaseStatus = "READY_FOR_REVIEW"
	StatusReview    CaseStatus = "IN_REVIEW"
	StatusReturned  CaseStatus = "RETURNED"
	StatusApproved  CaseStatus = "APPROVED"
)

type CheckpointKind string

const (
	KindPressure     CheckpointKind = "PRESSURE"
	KindAirtightness CheckpointKind = "AIRTIGHTNESS"
	KindInterlock    CheckpointKind = "INTERLOCK"
	KindRecovery     CheckpointKind = "RECOVERY"
)

var RequiredKinds = []CheckpointKind{KindPressure, KindAirtightness, KindInterlock, KindRecovery}

type AcceptanceLimits struct {
	PressureMinPa           float64 `json:"pressureMinPa"`
	PressureDurationSec     int     `json:"pressureDurationSec"`
	MaxLeakagePercent       float64 `json:"maxLeakagePercent"`
	InterlockResponseSec    float64 `json:"interlockResponseSec"`
	RecoveryMaxMinutes      float64 `json:"recoveryMaxMinutes"`
	RecoveryTargetParticles float64 `json:"recoveryTargetParticles"`
}

func (l AcceptanceLimits) Validate() error {
	if l.PressureMinPa <= 0 {
		return Validation("acceptanceLimits.pressureMinPa", "压差下限必须大于 0")
	}
	if l.PressureDurationSec <= 0 {
		return Validation("acceptanceLimits.pressureDurationSec", "压差持续窗口必须大于 0")
	}
	if l.MaxLeakagePercent <= 0 || l.MaxLeakagePercent >= 100 {
		return Validation("acceptanceLimits.maxLeakagePercent", "泄漏率上限必须在 0 到 100 之间")
	}
	if l.InterlockResponseSec <= 0 {
		return Validation("acceptanceLimits.interlockResponseSec", "互锁响应上限必须大于 0")
	}
	if l.RecoveryMaxMinutes <= 0 {
		return Validation("acceptanceLimits.recoveryMaxMinutes", "净化恢复时限必须大于 0")
	}
	if l.RecoveryTargetParticles <= 0 {
		return Validation("acceptanceLimits.recoveryTargetParticles", "目标粒子浓度必须大于 0")
	}
	return nil
}

type ZoneBoundary struct {
	Chamber  string `json:"chamber"`
	Adjacent string `json:"adjacent"`
}

type Checkpoint struct {
	ID       string         `json:"id"`
	Kind     CheckpointKind `json:"kind"`
	Name     string         `json:"name"`
	Sequence int            `json:"sequence"`
	Required []string       `json:"required"`
}

type TestProtocol struct {
	ID             string       `json:"id"`
	CaseID         string       `json:"caseId"`
	Revision       int          `json:"revision"`
	Checkpoints    []Checkpoint `json:"checkpoints"`
	RuleSetVersion string       `json:"ruleSetVersion"`
	FrozenAt       time.Time    `json:"frozenAt"`
	FrozenBy       string       `json:"frozenBy"`
	Digest         string       `json:"digest"`
}

type Measurement struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	OffsetSec int     `json:"offsetSec"`
	Flag      *bool   `json:"flag,omitempty"`
}

type Verdict string

const (
	VerdictPass Verdict = "PASS"
	VerdictFail Verdict = "FAIL"
)

type TestRun struct {
	ID               string              `json:"id"`
	CaseID           string              `json:"caseId"`
	ProtocolRevision int                 `json:"protocolRevision"`
	CheckpointID     string              `json:"checkpointId"`
	Attempt          int                 `json:"attempt"`
	Measurements     []Measurement       `json:"measurements"`
	InstrumentID     string              `json:"instrumentId"`
	Instrument       InstrumentEvidence  `json:"instrument"`
	Witness          string              `json:"witness"`
	StartedAt        time.Time           `json:"startedAt"`
	CompletedAt      time.Time           `json:"completedAt"`
	Verdict          Verdict             `json:"verdict"`
	FailureReasons   []string            `json:"failureReasons,omitempty"`
	Criteria         []CriterionEvidence `json:"criteria"`
	RuleSetVersion   string              `json:"ruleSetVersion"`
	EvidenceDigest   string              `json:"evidenceDigest"`
}

type CriterionEvidence struct {
	Code        string  `json:"code"`
	Label       string  `json:"label"`
	Comparator  string  `json:"comparator"`
	Limit       float64 `json:"limit"`
	Actual      float64 `json:"actual"`
	Unit        string  `json:"unit"`
	Passed      bool    `json:"passed"`
	Explanation string  `json:"explanation"`
}

type DeviationStatus string

const (
	DeviationOpen       DeviationStatus = "OPEN"
	DeviationRemediated DeviationStatus = "REMEDIATED"
	DeviationClosed     DeviationStatus = "CLOSED"
)

type Deviation struct {
	ID                  string          `json:"id"`
	CaseID              string          `json:"caseId"`
	FailedRunID         string          `json:"failedRunId"`
	Reason              string          `json:"reason"`
	RootCause           string          `json:"rootCause,omitempty"`
	CorrectiveAction    string          `json:"correctiveAction,omitempty"`
	SuggestedRisk       RiskLevel       `json:"suggestedRisk"`
	RiskLevel           RiskLevel       `json:"riskLevel"`
	RiskBasis           []string        `json:"riskBasis,omitempty"`
	RiskOverrideReason  string          `json:"riskOverrideReason,omitempty"`
	Owner               string          `json:"owner,omitempty"`
	DueAt               *time.Time      `json:"dueAt,omitempty"`
	RetestCheckpointIDs []string        `json:"retestCheckpointIds"`
	RetestScopeSource   string          `json:"retestScopeSource,omitempty"`
	Status              DeviationStatus `json:"status"`
	OpenedAt            time.Time       `json:"openedAt"`
	ClosedAt            *time.Time      `json:"closedAt,omitempty"`
}

type ActivationCredential struct {
	ID                     string    `json:"id"`
	CaseID                 string    `json:"caseId"`
	CaseNumber             string    `json:"caseNumber"`
	ApprovedBy             string    `json:"approvedBy"`
	ApprovedAt             time.Time `json:"approvedAt"`
	SnapshotDigest         string    `json:"snapshotDigest"`
	CredentialDigest       string    `json:"credentialDigest"`
	SchemaVersion          int       `json:"schemaVersion"`
	ReviewChecklistVersion int       `json:"reviewChecklistVersion"`
}

type TimelineEvent struct {
	Type    string         `json:"type"`
	At      time.Time      `json:"at"`
	Actor   string         `json:"actor"`
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data,omitempty"`
}

type CommissioningCase struct {
	ID                  string                `json:"id"`
	CaseNumber          string                `json:"caseNumber"`
	ChamberName         string                `json:"chamberName"`
	Zones               []ZoneBoundary        `json:"zones"`
	AirflowDirection    string                `json:"airflowDirection"`
	AcceptanceLimits    AcceptanceLimits      `json:"acceptanceLimits"`
	Status              CaseStatus            `json:"status"`
	Version             int                   `json:"version"`
	CreatedAt           time.Time             `json:"createdAt"`
	UpdatedAt           time.Time             `json:"updatedAt"`
	Protocol            *TestProtocol         `json:"protocol,omitempty"`
	Runs                []TestRun             `json:"runs"`
	Deviations          []Deviation           `json:"deviations"`
	Credential          *ActivationCredential `json:"credential,omitempty"`
	ReviewReturnReason  string                `json:"reviewReturnReason,omitempty"`
	FreezeConfirmations []FreezeConfirmation  `json:"freezeConfirmations,omitempty"`
	ReviewRounds        []ReviewRound         `json:"reviewRounds,omitempty"`
	Timeline            []TimelineEvent       `json:"timeline"`
}

type NewCase struct {
	ID               string
	CaseNumber       string
	ChamberName      string
	Zones            []ZoneBoundary
	AirflowDirection string
	Limits           AcceptanceLimits
	Actor            string
	Now              time.Time
}

func CreateCase(c NewCase) (*CommissioningCase, error) {
	if strings.TrimSpace(c.ID) == "" {
		return nil, Validation("id", "验证案 ID 不能为空")
	}
	if strings.TrimSpace(c.CaseNumber) == "" {
		return nil, Validation("caseNumber", "验证案编号不能为空")
	}
	if strings.TrimSpace(c.ChamberName) == "" {
		return nil, Validation("chamberName", "隔离舱名称不能为空")
	}
	if len(c.Zones) == 0 {
		return nil, Validation("zones", "至少登记一个相邻区域")
	}
	seen := map[string]bool{}
	for _, z := range c.Zones {
		if strings.TrimSpace(z.Chamber) == "" || strings.TrimSpace(z.Adjacent) == "" {
			return nil, Validation("zones", "边界两侧名称均不能为空")
		}
		key := strings.ToLower(strings.TrimSpace(z.Chamber) + "|" + strings.TrimSpace(z.Adjacent))
		if seen[key] {
			return nil, Validation("zones", "不能登记重复边界")
		}
		seen[key] = true
	}
	if strings.TrimSpace(c.AirflowDirection) == "" {
		return nil, Validation("airflowDirection", "气流方向不能为空")
	}
	if err := c.Limits.Validate(); err != nil {
		return nil, err
	}
	actor := strings.TrimSpace(c.Actor)
	if actor == "" {
		actor = "系统操作员"
	}
	x := &CommissioningCase{ID: c.ID, CaseNumber: c.CaseNumber, ChamberName: strings.TrimSpace(c.ChamberName), Zones: append([]ZoneBoundary(nil), c.Zones...), AirflowDirection: strings.TrimSpace(c.AirflowDirection), AcceptanceLimits: c.Limits, Status: StatusDraft, Version: 1, CreatedAt: c.Now.UTC(), UpdatedAt: c.Now.UTC()}
	x.record("CASE_CREATED", actor, "创建启用验证案", c.Now, map[string]any{"caseNumber": c.CaseNumber})
	return x, nil
}

func (c *CommissioningCase) checkVersion(expected int) error {
	if expected != c.Version {
		return Conflict(expected, c.Version)
	}
	return nil
}

func (c *CommissioningCase) mutate(now time.Time) { c.Version++; c.UpdatedAt = now.UTC() }

func (c *CommissioningCase) record(kind, actor, summary string, now time.Time, data map[string]any) {
	c.Timeline = append(c.Timeline, TimelineEvent{Type: kind, At: now.UTC(), Actor: actor, Summary: summary, Data: data})
}

func DefaultCheckpoints() []Checkpoint {
	return []Checkpoint{
		{ID: "pressure", Kind: KindPressure, Name: "舱内外压差持续性", Sequence: 1, Required: []string{"pressurePa", "durationSec"}},
		{ID: "airtightness", Kind: KindAirtightness, Name: "围护结构气密性", Sequence: 2, Required: []string{"leakagePercent"}},
		{ID: "interlock", Kind: KindInterlock, Name: "门禁互锁响应", Sequence: 3, Required: []string{"responseSec", "bothDoorsOpen"}},
		{ID: "recovery", Kind: KindRecovery, Name: "净化恢复能力", Sequence: 4, Required: []string{"recoveryMinutes", "particleCount"}},
	}
}

func (c *CommissioningCase) FreezeProtocol(expected int, actor string, now time.Time, checkpoints []Checkpoint, ruleVersion string) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if c.Status != StatusDraft && c.Status != StatusReturned {
		return State("只有草拟或退回状态可冻结方案")
	}
	if c.Credential != nil {
		return State("已批准案卷不可修改")
	}
	if len(checkpoints) != len(RequiredKinds) {
		return Validation("checkpoints", "方案必须包含四类检查点")
	}
	byKind := map[CheckpointKind]bool{}
	for i, cp := range checkpoints {
		if cp.ID == "" || cp.Name == "" || cp.Sequence != i+1 {
			return Validation("checkpoints", "检查点必须具有唯一标识、名称和连续顺序")
		}
		if byKind[cp.Kind] {
			return Validation("checkpoints", "检查点类型不能重复")
		}
		byKind[cp.Kind] = true
	}
	for _, kind := range RequiredKinds {
		if !byKind[kind] {
			return Validation("checkpoints", "方案缺少检查点 "+string(kind))
		}
	}
	revision := 1
	if c.Protocol != nil {
		revision = c.Protocol.Revision + 1
	}
	proto := TestProtocol{ID: fmt.Sprintf("%s-P%02d", c.ID, revision), CaseID: c.ID, Revision: revision, Checkpoints: append([]Checkpoint(nil), checkpoints...), RuleSetVersion: ruleVersion, FrozenAt: now.UTC(), FrozenBy: actor}
	b, _ := json.Marshal(proto)
	sum := sha256.Sum256(b)
	proto.Digest = hex.EncodeToString(sum[:])
	c.Protocol = &proto
	c.Status = StatusFrozen
	c.Runs = nil
	c.Deviations = nil
	c.ReviewReturnReason = ""
	c.mutate(now)
	c.record("PROTOCOL_FROZEN", actor, fmt.Sprintf("冻结测试方案第 %d 版", revision), now, map[string]any{"digest": proto.Digest})
	return nil
}

func (c *CommissioningCase) nextInitialCheckpoint() *Checkpoint {
	if c.Protocol == nil {
		return nil
	}
	completed := map[string]bool{}
	for _, r := range c.Runs {
		if r.Attempt == 1 {
			completed[r.CheckpointID] = true
		}
	}
	for i := range c.Protocol.Checkpoints {
		if !completed[c.Protocol.Checkpoints[i].ID] {
			return &c.Protocol.Checkpoints[i]
		}
	}
	return nil
}

func (c *CommissioningCase) ValidateRunTarget(expected, protocolRevision int, checkpointID string) (int, error) {
	if err := c.checkVersion(expected); err != nil {
		return 0, err
	}
	if c.Protocol == nil || c.Status == StatusDraft || c.Status == StatusReturned {
		return 0, State("测试方案尚未冻结")
	}
	if c.Status == StatusReview || c.Status == StatusApproved {
		return 0, State("复核中或已批准案卷不可录入测试")
	}
	if protocolRevision != c.Protocol.Revision {
		return 0, Conflict(protocolRevision, c.Protocol.Revision)
	}
	var cp *Checkpoint
	for i := range c.Protocol.Checkpoints {
		if c.Protocol.Checkpoints[i].ID == checkpointID {
			cp = &c.Protocol.Checkpoints[i]
			break
		}
	}
	if cp == nil {
		return 0, Validation("checkpointId", "检查点不属于冻结方案")
	}
	attempt := 1
	for _, r := range c.Runs {
		if r.CheckpointID == checkpointID && r.Attempt >= attempt {
			attempt = r.Attempt + 1
		}
	}
	if attempt == 1 {
		next := c.nextInitialCheckpoint()
		if next == nil {
			return 0, State("初始检查点均已完成")
		}
		if next.ID != checkpointID {
			return 0, State("必须按冻结方案顺序执行，下一项为 " + next.ID)
		}
	} else {
		allowed := false
		for _, d := range c.Deviations {
			if d.Status == DeviationRemediated {
				for _, id := range d.RetestCheckpointIDs {
					if id == checkpointID {
						allowed = true
					}
				}
			}
		}
		if !allowed {
			return 0, State("该检查点没有已批准的定向复测任务")
		}
	}
	return attempt, nil
}

func (c *CommissioningCase) AddRun(expected int, run TestRun, actor string, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	for _, existing := range c.Runs {
		if existing.CheckpointID == run.CheckpointID && existing.Attempt == run.Attempt {
			return Duplicate("同一检查点的该次测试已完成")
		}
	}
	c.Runs = append(c.Runs, run)
	if run.Verdict == VerdictFail {
		d := Deviation{ID: fmt.Sprintf("DEV-%s-%02d", c.ID, len(c.Deviations)+1), CaseID: c.ID, FailedRunID: run.ID, Reason: strings.Join(run.FailureReasons, "；"), RetestCheckpointIDs: []string{run.CheckpointID}, Status: DeviationOpen, OpenedAt: now.UTC()}
		c.Deviations = append(c.Deviations, d)
		c.Status = StatusDeviation
		c.record("DEVIATION_OPENED", actor, "测试失败并自动建立偏差", now, map[string]any{"deviationId": d.ID, "checkpointId": run.CheckpointID})
	} else if run.Attempt > 1 {
		for i := range c.Deviations {
			d := &c.Deviations[i]
			if d.Status == DeviationRemediated && contains(d.RetestCheckpointIDs, run.CheckpointID) {
				closed := now.UTC()
				d.Status = DeviationClosed
				d.ClosedAt = &closed
				c.record("DEVIATION_CLOSED", actor, "定向复测合格，关闭偏差", now, map[string]any{"deviationId": d.ID})
			}
		}
	}
	if c.allRequirementsPassed() {
		c.Status = StatusReady
	} else if c.Status != StatusDeviation {
		c.Status = StatusTesting
	}
	c.mutate(now)
	c.record("TEST_COMPLETED", actor, fmt.Sprintf("完成检查点 %s，第 %d 次结果 %s", run.CheckpointID, run.Attempt, run.Verdict), now, map[string]any{"runId": run.ID, "evidenceDigest": run.EvidenceDigest})
	return nil
}

func contains(items []string, value string) bool {
	for _, x := range items {
		if x == value {
			return true
		}
	}
	return false
}

func (c *CommissioningCase) allRequirementsPassed() bool {
	if c.Protocol == nil {
		return false
	}
	for _, d := range c.Deviations {
		if d.Status != DeviationClosed {
			return false
		}
	}
	for _, cp := range c.Protocol.Checkpoints {
		found := false
		for i := len(c.Runs) - 1; i >= 0; i-- {
			if c.Runs[i].CheckpointID == cp.ID {
				found = c.Runs[i].Verdict == VerdictPass
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (c *CommissioningCase) RemediateDeviation(expected int, deviationID, rootCause, action, actor string, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if strings.TrimSpace(rootCause) == "" || strings.TrimSpace(action) == "" {
		return Validation("remediation", "根因和纠正措施均不能为空")
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
	target.RootCause = strings.TrimSpace(rootCause)
	target.CorrectiveAction = strings.TrimSpace(action)
	target.Status = DeviationRemediated
	c.Status = StatusTesting
	c.mutate(now)
	c.record("DEVIATION_REMEDIATED", actor, "记录根因和纠正措施，生成定向复测", now, map[string]any{"deviationId": deviationID, "scope": target.RetestCheckpointIDs})
	return nil
}

func (c *CommissioningCase) SubmitReview(expected int, actor string, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if c.Status != StatusReady || !c.allRequirementsPassed() {
		return State("全部检查点及定向复测合格后方可送审")
	}
	c.Status = StatusReview
	c.mutate(now)
	c.record("REVIEW_SUBMITTED", actor, "提交生物安全复核", now, nil)
	return nil
}

func (c *CommissioningCase) ReturnReview(expected int, reviewer, reason string, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if c.Status != StatusReview {
		return State("只有复核中案卷可退回")
	}
	if strings.TrimSpace(reason) == "" {
		return Validation("reason", "退回理由不能为空")
	}
	c.Status = StatusReturned
	c.ReviewReturnReason = strings.TrimSpace(reason)
	c.mutate(now)
	c.record("REVIEW_RETURNED", reviewer, "安全复核退回："+reason, now, nil)
	return nil
}

func (c *CommissioningCase) Approve(expected int, credential ActivationCredential, reviewer string, now time.Time) error {
	if err := c.checkVersion(expected); err != nil {
		return err
	}
	if c.Status != StatusReview {
		return State("只有复核中案卷可批准")
	}
	if c.Credential != nil {
		return Duplicate("该验证案已签发凭据")
	}
	c.Credential = &credential
	c.Status = StatusApproved
	c.mutate(now)
	c.record("CASE_APPROVED", reviewer, "安全复核批准并签发启用凭据", now, map[string]any{"credentialId": credential.ID, "digest": credential.CredentialDigest})
	return nil
}

type Snapshot struct {
	CaseID           string           `json:"caseId"`
	CaseNumber       string           `json:"caseNumber"`
	ChamberName      string           `json:"chamberName"`
	Zones            []ZoneBoundary   `json:"zones"`
	AirflowDirection string           `json:"airflowDirection"`
	Limits           AcceptanceLimits `json:"acceptanceLimits"`
	Protocol         TestProtocol     `json:"protocol"`
	Runs             []TestRun        `json:"runs"`
	Deviations       []Deviation      `json:"deviations"`
	ReviewRounds     []ReviewRound    `json:"reviewRounds"`
	Timeline         []TimelineEvent  `json:"timeline"`
}

func (c *CommissioningCase) ImmutableSnapshot() (Snapshot, error) {
	if c.Protocol == nil {
		return Snapshot{}, State("案卷没有冻结方案")
	}
	runs := append([]TestRun(nil), c.Runs...)
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CheckpointID == runs[j].CheckpointID {
			return runs[i].Attempt < runs[j].Attempt
		}
		return runs[i].CheckpointID < runs[j].CheckpointID
	})
	return Snapshot{CaseID: c.ID, CaseNumber: c.CaseNumber, ChamberName: c.ChamberName, Zones: append([]ZoneBoundary(nil), c.Zones...), AirflowDirection: c.AirflowDirection, Limits: c.AcceptanceLimits, Protocol: *c.Protocol, Runs: runs, Deviations: append([]Deviation(nil), c.Deviations...), ReviewRounds: append([]ReviewRound(nil), c.ReviewRounds...), Timeline: append([]TimelineEvent(nil), c.Timeline...)}, nil
}
