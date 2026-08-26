package application

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"isolation-chamber-commissioning/internal/domain"
	"isolation-chamber-commissioning/internal/persistence"
	"isolation-chamber-commissioning/internal/verification"
)

func (s *Service) RecordRun(caseID string, cmd RecordRunCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	key := scoped("run", caseID, cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	c, err := s.store.GetCase(caseID)
	if err != nil {
		return WriteResult{}, err
	}
	before, now := c.Version, s.now().UTC()
	items := cmd.Runs
	if len(items) == 0 {
		items = cmd.Items
	}
	if len(items) == 0 {
		items = []RunItemCommand{{CheckpointID: cmd.CheckpointID, Measurements: cmd.Measurements, InstrumentID: cmd.InstrumentID, CertificateNumber: cmd.CertificateNumber, CalibrationCertificateNumber: cmd.CalibrationCertificateNumber, CalibrationValidFrom: cmd.CalibrationValidFrom, CalibrationValidUntil: cmd.CalibrationValidUntil, ApplicableKinds: cmd.ApplicableKinds, ApplicableCheckpointKinds: cmd.ApplicableCheckpointKinds, Instrument: cmd.Instrument, Witness: cmd.Witness, StartedAt: cmd.StartedAt, CompletedAt: cmd.CompletedAt, Actor: cmd.Actor}}
	}
	if len(items) == 0 {
		return WriteResult{}, domain.Validation("runs", "批次至少包含一个检查点")
	}
	seen := map[string]bool{}
	runs := make([]domain.TestRun, 0, len(items))
	for i, item := range items {
		if seen[item.CheckpointID] {
			return WriteResult{}, indexedError(i, domain.Validation("checkpointId", "批次内不得重复检查点"))
		}
		seen[item.CheckpointID] = true
		attempt, e := c.ValidateRunTarget(c.Version, cmd.ProtocolRevision, item.CheckpointID)
		if e != nil {
			return WriteResult{}, indexedError(i, e)
		}
		var checkpoint domain.Checkpoint
		found := false
		for _, cp := range c.Protocol.Checkpoints {
			if cp.ID == item.CheckpointID {
				checkpoint, found = cp, true
				break
			}
		}
		if !found {
			return WriteResult{}, indexedError(i, domain.Validation("checkpointId", "检查点不存在"))
		}
		started, completed := item.StartedAt, item.CompletedAt
		if completed.IsZero() {
			completed = now
		}
		if started.IsZero() {
			started = completed.Add(-time.Minute)
		}
		instrumentInput := item.Instrument
		if instrumentInput.ID == "" {
			instrumentInput.ID = item.InstrumentID
		}
		if instrumentInput.CertificateNumber == "" {
			instrumentInput.CertificateNumber = item.CertificateNumber
		}
		if instrumentInput.CertificateNumber == "" {
			instrumentInput.CertificateNumber = item.CalibrationCertificateNumber
		}
		if instrumentInput.CalibrationValidFrom.IsZero() {
			instrumentInput.CalibrationValidFrom = item.CalibrationValidFrom
		}
		if instrumentInput.CalibrationValidUntil.IsZero() {
			instrumentInput.CalibrationValidUntil = item.CalibrationValidUntil
		}
		if len(instrumentInput.ApplicableKinds) == 0 {
			instrumentInput.ApplicableKinds = item.ApplicableKinds
		}
		if len(instrumentInput.ApplicableKinds) == 0 {
			instrumentInput.ApplicableKinds = item.ApplicableCheckpointKinds
		}
		instrument, e := c.ValidateInstrument(instrumentInput, checkpoint.Kind, completed.UTC())
		if e != nil {
			return WriteResult{}, indexedError(i, e)
		}
		runID, e := randomID("run-")
		if e != nil {
			return WriteResult{}, e
		}
		run, e := s.engine.Evaluate(verification.Input{CaseID: c.ID, ProtocolRevision: c.Protocol.Revision, Checkpoint: checkpoint, Limits: c.AcceptanceLimits, Measurements: item.Measurements, InstrumentID: instrument.ID, Instrument: instrument, Witness: item.Witness, StartedAt: started, CompletedAt: completed, Attempt: attempt, RunID: runID})
		if e != nil {
			return WriteResult{}, indexedError(i, e)
		}
		assessment := domain.DeviationAssessment{}
		if run.Verdict == domain.VerdictFail {
			assessment, e = verification.AssessDeviation(run, *c.Protocol)
			if e != nil {
				return WriteResult{}, indexedError(i, e)
			}
		}
		if e = c.AddRunWithAssessment(c.Version, run, assessment, actor(item.Actor, item.Witness), now); e != nil {
			return WriteResult{}, indexedError(i, e)
		}
		runs = append(runs, run)
	}
	if len(items) > 1 {
		c.Version = before + 1
		c.UpdatedAt = now
	}
	response := struct {
		Case *domain.CommissioningCase `json:"case"`
		Runs []domain.TestRun          `json:"runs"`
		Run  *domain.TestRun           `json:"run,omitempty"`
	}{Case: c, Runs: runs}
	if len(runs) == 1 {
		response.Run = &runs[0]
	}
	r, err := result(201, response)
	if err != nil {
		return WriteResult{}, err
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: before, Case: c, Action: "TEST_BATCH_COMPLETED", Payload: map[string]any{"runs": runs, "count": len(runs), "deviations": c.Deviations}, Result: r, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}

func indexedError(index int, err error) error {
	var de *domain.DomainError
	if errors.As(err, &de) {
		copy := *de
		if copy.Field == "" {
			copy.Field = fmt.Sprintf("runs[%d]", index)
		} else {
			copy.Field = fmt.Sprintf("runs[%d].%s", index, copy.Field)
		}
		copy.Message = fmt.Sprintf("批次第 %d 项：%s", index+1, copy.Message)
		return &copy
	}
	return err
}

func (s *Service) Remediate(caseID, deviationID string, cmd RemediateCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	key := scoped("remediate-"+deviationID, caseID, cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	c, err := s.store.GetCase(caseID)
	if err != nil {
		return WriteResult{}, err
	}
	before := c.Version
	now := s.now().UTC()
	if err := c.RemediateDeviationWithDisposition(cmd.ExpectedVersion, deviationID, cmd.RootCause, cmd.CorrectiveAction, cmd.Owner, cmd.DueAt, cmd.RiskLevel, cmd.RiskOverrideReason, actor(cmd.Actor, "验证工程师"), now); err != nil {
		return WriteResult{}, err
	}
	r, err := result(200, c)
	if err != nil {
		return WriteResult{}, err
	}
	var disposition domain.Deviation
	for _, deviation := range c.Deviations {
		if deviation.ID == deviationID {
			disposition = deviation
			break
		}
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: before, Case: c, Action: "DEVIATION_REMEDIATED", Payload: disposition, Result: r, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}

func (s *Service) SubmitReview(caseID string, cmd SubmitReviewCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	key := scoped("submit-review", caseID, cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	c, err := s.store.GetCase(caseID)
	if err != nil {
		return WriteResult{}, err
	}
	before := c.Version
	now := s.now().UTC()
	items, err := verification.ReviewChecklist(c)
	if err != nil {
		return WriteResult{}, err
	}
	if err := c.SubmitReviewChecklist(cmd.ExpectedVersion, actor(cmd.Actor, "验证工程师"), items, cmd.IssueResolutions, verification.RuleSetVersion, now); err != nil {
		return WriteResult{}, err
	}
	r, err := result(200, c)
	if err != nil {
		return WriteResult{}, err
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: before, Case: c, Action: "REVIEW_SUBMITTED", Payload: c.ReviewRounds[len(c.ReviewRounds)-1], Result: r, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}

func (s *Service) Review(caseID string, cmd ReviewCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	decision := strings.ToUpper(strings.TrimSpace(cmd.Decision))
	if decision != "APPROVE" && decision != "RETURN" {
		return WriteResult{}, domain.Validation("decision", "复核决定必须为 APPROVE 或 RETURN")
	}
	key := scoped("review-"+decision, caseID, cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	c, err := s.store.GetCase(caseID)
	if err != nil {
		return WriteResult{}, err
	}
	before := c.Version
	now := s.now().UTC()
	reviewer := actor(cmd.Reviewer, "生物安全复核员")
	var snapshot *domain.Snapshot
	answers := cmd.Answers
	if len(answers) == 0 {
		answers = cmd.ChecklistAnswers
	}
	if err := c.ReviewChecklist(cmd.ExpectedVersion, decision, reviewer, answers, cmd.Issues, now); err != nil {
		return WriteResult{}, err
	}
	if decision == "RETURN" {
		reasons := make([]string, 0, len(cmd.Issues))
		for _, issue := range cmd.Issues {
			reasons = append(reasons, issue.Reason)
		}
		if err := c.ReturnReview(cmd.ExpectedVersion, reviewer, strings.Join(reasons, "；"), now); err != nil {
			return WriteResult{}, err
		}
	} else {
		snap, err := c.ImmutableSnapshot()
		if err != nil {
			return WriteResult{}, err
		}
		snapshot = &snap
		sd, err := verification.SnapshotDigest(snap)
		if err != nil {
			return WriteResult{}, err
		}
		credentialID := fmt.Sprintf("AC-%s-%s", now.Format("20060102"), strings.ToUpper(c.ID[len(c.ID)-8:]))
		cd, err := verification.CredentialDigest(credentialID, c.ID, c.CaseNumber, reviewer, now, sd, persistence.SchemaVersion)
		if err != nil {
			return WriteResult{}, err
		}
		checklistVersion := c.ReviewRounds[len(c.ReviewRounds)-1].Version
		credential := domain.ActivationCredential{ID: credentialID, CaseID: c.ID, CaseNumber: c.CaseNumber, ApprovedBy: reviewer, ApprovedAt: now, SnapshotDigest: sd, CredentialDigest: cd, SchemaVersion: persistence.SchemaVersion, ReviewChecklistVersion: checklistVersion}
		if err := c.Approve(cmd.ExpectedVersion, credential, reviewer, now); err != nil {
			return WriteResult{}, err
		}
	}
	r, err := result(200, c)
	if err != nil {
		return WriteResult{}, err
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: before, Case: c, Action: "REVIEW_" + decision, Payload: c.ReviewRounds[len(c.ReviewRounds)-1], Result: r, CredentialSnapshot: snapshot, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}
