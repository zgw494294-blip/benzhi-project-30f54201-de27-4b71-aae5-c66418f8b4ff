package application

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"isolation-chamber-commissioning/internal/domain"
	"isolation-chamber-commissioning/internal/persistence"
	"isolation-chamber-commissioning/internal/verification"
)

type Clock func() time.Time

type Service struct {
	store  *persistence.Store
	engine *verification.Engine
	now    Clock
}

func New(store *persistence.Store, engine *verification.Engine) *Service {
	return &Service{store: store, engine: engine, now: time.Now}
}
func NewWithClock(store *persistence.Store, engine *verification.Engine, clock Clock) *Service {
	return &Service{store: store, engine: engine, now: clock}
}

type CreateCaseCommand struct {
	ChamberName      string                  `json:"chamberName"`
	Zones            []domain.ZoneBoundary   `json:"zones"`
	AirflowDirection string                  `json:"airflowDirection"`
	AcceptanceLimits domain.AcceptanceLimits `json:"acceptanceLimits"`
	Actor            string                  `json:"actor"`
	IdempotencyKey   string                  `json:"-"`
}

type FreezeProtocolCommand struct {
	ExpectedVersion   int    `json:"expectedVersion"`
	FrozenBy          string `json:"frozenBy"`
	ConfirmationToken string `json:"confirmationToken"`
	ConfirmationID    string `json:"confirmationId,omitempty"`
	IdempotencyKey    string `json:"-"`
}

type ReviseCaseCommand struct {
	ExpectedVersion  int                     `json:"expectedVersion"`
	ChamberName      string                  `json:"chamberName"`
	Zones            []domain.ZoneBoundary   `json:"zones"`
	AirflowDirection string                  `json:"airflowDirection"`
	AcceptanceLimits domain.AcceptanceLimits `json:"acceptanceLimits"`
	Actor            string                  `json:"actor"`
	IdempotencyKey   string                  `json:"-"`
}

type FreezePreflightCommand struct {
	ExpectedVersion int    `json:"expectedVersion"`
	Actor           string `json:"actor"`
	IdempotencyKey  string `json:"-"`
}

type RunItemCommand struct {
	CheckpointID                 string                    `json:"checkpointId"`
	Measurements                 []domain.Measurement      `json:"measurements"`
	InstrumentID                 string                    `json:"instrumentId"`
	CertificateNumber            string                    `json:"certificateNumber"`
	CalibrationCertificateNumber string                    `json:"calibrationCertificateNumber,omitempty"`
	CalibrationValidFrom         time.Time                 `json:"calibrationValidFrom"`
	CalibrationValidUntil        time.Time                 `json:"calibrationValidUntil"`
	ApplicableKinds              []domain.CheckpointKind   `json:"applicableKinds"`
	ApplicableCheckpointKinds    []domain.CheckpointKind   `json:"applicableCheckpointKinds,omitempty"`
	Instrument                   domain.InstrumentEvidence `json:"instrument,omitempty"`
	Witness                      string                    `json:"witness"`
	StartedAt                    time.Time                 `json:"startedAt"`
	CompletedAt                  time.Time                 `json:"completedAt"`
	Actor                        string                    `json:"actor"`
}

type RecordRunCommand struct {
	ExpectedVersion              int                       `json:"expectedVersion"`
	ProtocolRevision             int                       `json:"protocolRevision"`
	CheckpointID                 string                    `json:"checkpointId"`
	Measurements                 []domain.Measurement      `json:"measurements"`
	InstrumentID                 string                    `json:"instrumentId"`
	Witness                      string                    `json:"witness"`
	StartedAt                    time.Time                 `json:"startedAt"`
	CompletedAt                  time.Time                 `json:"completedAt"`
	CertificateNumber            string                    `json:"certificateNumber"`
	CalibrationCertificateNumber string                    `json:"calibrationCertificateNumber,omitempty"`
	CalibrationValidFrom         time.Time                 `json:"calibrationValidFrom"`
	CalibrationValidUntil        time.Time                 `json:"calibrationValidUntil"`
	ApplicableKinds              []domain.CheckpointKind   `json:"applicableKinds"`
	ApplicableCheckpointKinds    []domain.CheckpointKind   `json:"applicableCheckpointKinds,omitempty"`
	Instrument                   domain.InstrumentEvidence `json:"instrument,omitempty"`
	Runs                         []RunItemCommand          `json:"runs,omitempty"`
	Items                        []RunItemCommand          `json:"items,omitempty"`
	Actor                        string                    `json:"actor"`
	IdempotencyKey               string                    `json:"-"`
}

type RemediateCommand struct {
	ExpectedVersion    int              `json:"expectedVersion"`
	RootCause          string           `json:"rootCause"`
	CorrectiveAction   string           `json:"correctiveAction"`
	Owner              string           `json:"owner"`
	DueAt              time.Time        `json:"dueAt"`
	RiskLevel          domain.RiskLevel `json:"riskLevel"`
	RiskOverrideReason string           `json:"riskOverrideReason"`
	Actor              string           `json:"actor"`
	IdempotencyKey     string           `json:"-"`
}
type SubmitReviewCommand struct {
	ExpectedVersion  int               `json:"expectedVersion"`
	Actor            string            `json:"actor"`
	IssueResolutions map[string]string `json:"issueResolutions,omitempty"`
	IdempotencyKey   string            `json:"-"`
}
type ReviewCommand struct {
	ExpectedVersion  int                      `json:"expectedVersion"`
	Decision         string                   `json:"decision"`
	Reviewer         string                   `json:"reviewer"`
	Reason           string                   `json:"reason"`
	Answers          []domain.ChecklistAnswer `json:"answers,omitempty"`
	ChecklistAnswers []domain.ChecklistAnswer `json:"checklistAnswers,omitempty"`
	Issues           []domain.ReviewIssue     `json:"issues,omitempty"`
	IdempotencyKey   string                   `json:"-"`
}

type WriteResult struct {
	Status   int
	Body     json.RawMessage
	Replayed bool
}

func randomID(prefix string) (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}
func requiredKey(k string) error {
	if strings.TrimSpace(k) == "" {
		return domain.Validation("Idempotency-Key", "写请求必须提供 Idempotency-Key")
	}
	if len(k) > 160 {
		return domain.Validation("Idempotency-Key", "幂等键过长")
	}
	return nil
}
func actor(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
func scoped(action, caseID, key string) string {
	return action + ":" + caseID + ":" + strings.TrimSpace(key)
}

func result(status int, value any) (persistence.CommandResult, error) {
	b, err := json.Marshal(value)
	return persistence.CommandResult{Status: status, Body: b}, err
}
