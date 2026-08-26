package application

import (
	"fmt"
	"time"

	"isolation-chamber-commissioning/internal/domain"
	"isolation-chamber-commissioning/internal/persistence"
	"isolation-chamber-commissioning/internal/verification"
)

type ReadinessProjection struct {
	CompletedCheckpoints int      `json:"completedCheckpoints"`
	TotalCheckpoints     int      `json:"totalCheckpoints"`
	PassedLatest         int      `json:"passedLatest"`
	FailedLatest         int      `json:"failedLatest"`
	OpenDeviations       int      `json:"openDeviations"`
	NextCheckpointID     string   `json:"nextCheckpointId,omitempty"`
	CanSubmitReview      bool     `json:"canSubmitReview"`
	CanReview            bool     `json:"canReview"`
	Immutable            bool     `json:"immutable"`
	BlockingReasons      []string `json:"blockingReasons"`
}

type EvidenceProjection struct {
	RunCount             int `json:"runCount"`
	OriginalRunCount     int `json:"originalRunCount"`
	RetestRunCount       int `json:"retestRunCount"`
	MeasurementCount     int `json:"measurementCount"`
	CriterionCount       int `json:"criterionCount"`
	FailedCriterionCount int `json:"failedCriterionCount"`
}

type CaseDetail struct {
	Case            *domain.CommissioningCase  `json:"case"`
	Audit           []persistence.AuditEvent   `json:"audit"`
	Readiness       ReadinessProjection        `json:"readiness"`
	Evidence        EvidenceProjection         `json:"evidence"`
	DeviationStates []DeviationStateProjection `json:"deviationStates"`
}

type DeviationStateProjection struct {
	DeviationID    string `json:"deviationId"`
	Status         string `json:"status"`
	Owner          string `json:"owner,omitempty"`
	RemainingHours int    `json:"remainingHours,omitempty"`
	OverdueDays    int    `json:"overdueDays,omitempty"`
}

func projectCase(c *domain.CommissioningCase, now time.Time) (ReadinessProjection, EvidenceProjection, []DeviationStateProjection) {
	readiness := ReadinessProjection{CanReview: c.Status == domain.StatusReview, Immutable: c.Status == domain.StatusApproved}
	evidence := EvidenceProjection{RunCount: len(c.Runs)}
	var deviationStates []DeviationStateProjection
	if c.Protocol == nil {
		readiness.BlockingReasons = append(readiness.BlockingReasons, "测试方案尚未冻结")
		return readiness, evidence, deviationStates
	}
	readiness.TotalCheckpoints = len(c.Protocol.Checkpoints)
	latest := map[string]domain.TestRun{}
	for _, run := range c.Runs {
		latest[run.CheckpointID] = run
		evidence.MeasurementCount += len(run.Measurements)
		evidence.CriterionCount += len(run.Criteria)
		if run.Attempt == 1 {
			evidence.OriginalRunCount++
		} else {
			evidence.RetestRunCount++
		}
		for _, item := range run.Criteria {
			if !item.Passed {
				evidence.FailedCriterionCount++
			}
		}
	}
	for _, checkpoint := range c.Protocol.Checkpoints {
		run, ok := latest[checkpoint.ID]
		if !ok {
			if readiness.NextCheckpointID == "" {
				readiness.NextCheckpointID = checkpoint.ID
			}
			continue
		}
		readiness.CompletedCheckpoints++
		if run.Verdict == domain.VerdictPass {
			readiness.PassedLatest++
		} else {
			readiness.FailedLatest++
		}
	}
	for _, deviation := range c.Deviations {
		if deviation.Status != domain.DeviationClosed {
			readiness.OpenDeviations++
		}
		projection := DeviationStateProjection{DeviationID: deviation.ID, Status: deviation.ProjectionStatus(now), Owner: deviation.Owner}
		if deviation.DueAt != nil {
			delta := deviation.DueAt.Sub(now)
			if delta < 0 {
				projection.OverdueDays = int((-delta + 24*time.Hour - 1) / (24 * time.Hour))
			} else {
				projection.RemainingHours = int(delta / time.Hour)
			}
		}
		deviationStates = append(deviationStates, projection)
	}
	if readiness.CompletedCheckpoints < readiness.TotalCheckpoints {
		readiness.BlockingReasons = append(readiness.BlockingReasons, fmt.Sprintf("仍有 %d 个初始检查点未完成", readiness.TotalCheckpoints-readiness.CompletedCheckpoints))
	}
	if readiness.FailedLatest > 0 {
		readiness.BlockingReasons = append(readiness.BlockingReasons, fmt.Sprintf("仍有 %d 个检查点的最新结果不合格", readiness.FailedLatest))
	}
	if readiness.OpenDeviations > 0 {
		readiness.BlockingReasons = append(readiness.BlockingReasons, fmt.Sprintf("仍有 %d 项偏差未闭环", readiness.OpenDeviations))
	}
	for _, projection := range deviationStates {
		if projection.Status == "OVERDUE" {
			readiness.BlockingReasons = append(readiness.BlockingReasons, fmt.Sprintf("偏差 %s 已逾期 %d 天，责任人：%s", projection.DeviationID, projection.OverdueDays, projection.Owner))
		}
	}
	readiness.CanSubmitReview = c.Status == domain.StatusReady && len(readiness.BlockingReasons) == 0
	return readiness, evidence, deviationStates
}

func (s *Service) GetCase(id string) (CaseDetail, error) {
	c, err := s.store.GetCase(id)
	if err != nil {
		return CaseDetail{}, err
	}
	events, err := s.store.AuditEvents(id)
	if err != nil {
		return CaseDetail{}, err
	}
	readiness, evidence, deviations := projectCase(c, s.now().UTC())
	return CaseDetail{Case: c, Audit: events, Readiness: readiness, Evidence: evidence, DeviationStates: deviations}, nil
}
func (s *Service) ListCases(query string) []persistence.CaseSummary { return s.store.ListCases(query) }

type SystemStatus struct {
	RuleSetVersion string                        `json:"ruleSetVersion"`
	Rules          []verification.RuleDefinition `json:"rules"`
	Persistence    persistence.Diagnostics       `json:"persistence"`
}

func (s *Service) Status() SystemStatus {
	return SystemStatus{RuleSetVersion: verification.RuleSetVersion, Rules: verification.RuleCatalog(), Persistence: s.store.Diagnostics()}
}

type CredentialVerification struct {
	Authentic        bool                        `json:"authentic"`
	Credential       domain.ActivationCredential `json:"credential"`
	RecomputedDigest string                      `json:"recomputedDigest"`
	Message          string                      `json:"message"`
	Timeline         []domain.TimelineEvent      `json:"timeline"`
}

func (s *Service) VerifyCredential(id string) (CredentialVerification, error) {
	credential, snapshot, err := s.store.Credential(id)
	if err != nil {
		return CredentialVerification{}, err
	}
	ok, recomputed, err := verification.VerifyCredential(credential, snapshot)
	if err != nil {
		return CredentialVerification{}, err
	}
	message := "凭据摘要与封存案卷一致"
	if !ok {
		message = "凭据摘要与封存案卷不一致"
	}
	return CredentialVerification{ok, credential, recomputed, message, snapshot.Timeline}, nil
}
