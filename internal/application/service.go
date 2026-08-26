package application

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

type Service struct {
	repository Repository
	writeMu    sync.Mutex
	clock      func() time.Time
	idFactory  func(string) string
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, clock: time.Now, idFactory: randomID}
}

func (s *Service) CreateBatch(command CreateBatchCommand) (*domain.Batch, error) {
	key, err := operationKey("create_batch", command.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var cached domain.Batch
	if found, err := s.repository.GetIdempotency(key, &cached); err != nil {
		return nil, err
	} else if found {
		return domain.CloneBatch(&cached)
	}
	now := s.clock().UTC()
	var batch *domain.Batch
	if strings.TrimSpace(command.SourceBatchID) != "" {
		source, sourceErr := s.repository.GetBatch(strings.TrimSpace(command.SourceBatchID))
		if sourceErr != nil {
			return nil, domain.FieldError{Field: "sourceBatchID", Message: "来源批次不存在"}
		}
		batch, err = domain.NewBatchFromSource(s.idFactory("batch"), source, now)
	} else {
		batch, err = domain.NewBatch(s.idFactory("batch"), command.CableSection, command.CircuitName, command.TestOwner, now)
	}
	if err != nil {
		return nil, err
	}
	matches, err := s.FindBatchMatches(batch.CableSection, batch.CircuitName)
	if err != nil {
		return nil, err
	}
	if len(matches.Active) > 0 && !command.ConfirmDuplicate {
		return nil, DuplicateBatchError{Matches: matches.Active}
	}
	if err := s.repository.Commit(persistence.Commit{
		Batch: batch, Operation: "batch.created", Actor: batch.TestOwner,
		Details:        map[string]any{"cableSection": batch.CableSection, "circuitName": batch.CircuitName, "sourceBatchID": batch.SourceBatchID, "duplicateConfirmed": command.ConfirmDuplicate},
		IdempotencyKey: key, Response: batch,
	}); err != nil {
		return nil, err
	}
	return domain.CloneBatch(batch)
}

func (s *Service) PreflightScope(points []domain.TestPoint) (domain.ScopePreflight, error) {
	return domain.PreflightScope(points)
}

func (s *Service) FreezeScope(batchID string, command FreezeScopeCommand) (*domain.Batch, error) {
	return s.mutateBatch("freeze_scope", "batch.scope_frozen", batchID, command.CommandMeta, func(batch *domain.Batch, now time.Time) (any, error) {
		preflight, err := domain.PreflightScope(command.Points)
		if err != nil {
			return nil, err
		}
		if !command.Confirmed || strings.TrimSpace(command.PreflightScopeDigest) == "" {
			return nil, domain.FieldError{Field: "confirmed", Message: "执行冻结预检并确认摘要后才能冻结"}
		}
		if command.PreflightScopeDigest != preflight.ScopeDigest {
			return nil, domain.FieldError{Field: "preflightScopeDigest", Message: "试验点内容已变化，请重新预检并确认"}
		}
		if err := batch.Freeze(command.Points, command.Actor, now); err != nil {
			return nil, err
		}
		return batch, nil
	})
}

func (s *Service) AddMeasurement(batchID string, command AddMeasurementCommand) (*domain.Batch, error) {
	return s.mutateBatch("add_measurement", "measurement.recorded", batchID, command.CommandMeta, func(batch *domain.Batch, now time.Time) (any, error) {
		measurement := domain.Measurement{
			ID: s.idFactory("measurement"), BatchID: batch.ID, PointID: command.PointID,
			Round: command.Round, MeasuredAt: command.MeasuredAt, PeakAmplitudePC: command.PeakAmplitudePC,
			PhaseSummary: command.PhaseSummary, TemperatureC: command.TemperatureC,
			HumidityPercent: command.HumidityPercent, SensorSerial: command.SensorSerial,
			Operator: command.Operator, Purpose: strings.TrimSpace(command.Purpose),
		}
		if measurement.Purpose == "" {
			measurement.Purpose = "initial"
		}
		if err := batch.AddMeasurement(measurement, now); err != nil {
			return nil, err
		}
		return batch, nil
	})
}

func (s *Service) AddMeasurements(batchID string, command AddMeasurementsCommand) (*domain.Batch, error) {
	return s.mutateBatch("add_measurements", "measurements.recorded", batchID, command.CommandMeta, func(batch *domain.Batch, now time.Time) (any, error) {
		measurements := make([]domain.Measurement, 0, len(command.Measurements))
		for _, input := range command.Measurements {
			id := strings.TrimSpace(input.ID)
			if id == "" {
				id = s.idFactory("measurement")
			}
			measurements = append(measurements, domain.Measurement{ID: id, BatchID: batch.ID, PointID: input.PointID,
				Round: input.Round, MeasuredAt: input.MeasuredAt, PeakAmplitudePC: input.PeakAmplitudePC, PhaseSummary: input.PhaseSummary,
				TemperatureC: input.TemperatureC, HumidityPercent: input.HumidityPercent, SensorSerial: input.SensorSerial,
				Operator: input.Operator, Purpose: input.Purpose})
		}
		return batch.AddMeasurements(measurements, now)
	})
}

func (s *Service) Diagnose(batchID string, command DiagnoseCommand) (*domain.Batch, error) {
	key, err := operationKey("diagnose", command.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(command.Actor) == "" {
		return nil, domain.FieldError{Field: "actor", Message: "操作人不能为空"}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var cached domain.Batch
	if found, cacheErr := s.repository.GetIdempotency(key, &cached); cacheErr != nil {
		return nil, cacheErr
	} else if found {
		return domain.CloneBatch(&cached)
	}
	batch, err := s.repository.GetBatch(batchID)
	if err != nil {
		return nil, err
	}
	if command.ExpectedVersion != batch.Version {
		return nil, VersionConflictError{BatchID: batchID, Want: command.ExpectedVersion, Actual: batch.Version}
	}
	digest, err := batch.DiagnosisEvidenceDigest()
	if err != nil {
		return nil, err
	}
	if batch.FindDiagnosisReportByEvidence(digest) != nil {
		return domain.CloneBatch(batch)
	}
	beforeVersion := batch.Version
	report, err := batch.RunDiagnosisReport(s.idFactory("diagnosis"), func() string { return s.idFactory("deviation") }, s.clock().UTC())
	if err != nil {
		return nil, err
	}
	if err := s.repository.Commit(persistence.Commit{Batch: batch, Operation: "diagnosis.completed", Actor: command.Actor,
		Details:        map[string]any{"fromVersion": beforeVersion, "toVersion": batch.Version, "runID": report.RunID, "evidenceDigest": report.EvidenceDigest, "risk": report.Risk},
		IdempotencyKey: key, Response: batch}); err != nil {
		return nil, err
	}
	return domain.CloneBatch(batch)
}

func (s *Service) CorrectDeviation(batchID, deviationID string, command CorrectDeviationCommand) (*domain.Batch, error) {
	operation := "correct_deviation:" + deviationID
	return s.mutateBatch(operation, "deviation.corrected", batchID, command.CommandMeta, func(batch *domain.Batch, now time.Time) (any, error) {
		correction := domain.Correction{
			Measure: command.Measure, Assignee: command.Assignee,
			RetestPoints: append([]string(nil), command.RetestPoints...), RecordedBy: command.Actor,
		}
		if err := batch.RecordCorrection(deviationID, correction, now); err != nil {
			return nil, err
		}
		return batch, nil
	})
}

func (s *Service) EvaluateRetest(batchID, deviationID string, command EvaluateRetestCommand) (*domain.Batch, error) {
	operation := "evaluate_retest:" + deviationID
	return s.mutateBatch(operation, "deviation.retested", batchID, command.CommandMeta, func(batch *domain.Batch, now time.Time) (any, error) {
		ids := append([]string(nil), command.MeasurementIDs...)
		if strings.TrimSpace(command.MeasurementID) != "" {
			ids = append(ids, command.MeasurementID)
		}
		_, err := batch.EvaluateRetestMeasurements(deviationID, ids, command.Conclusion, now)
		return batch, err
	})
}

func (s *Service) SubmitReview(batchID string, command SubmitReviewCommand) (*domain.Batch, error) {
	return s.mutateBatch("submit_review", "review.submitted", batchID, command.CommandMeta, func(batch *domain.Batch, now time.Time) (any, error) {
		review := domain.SafetyReview{
			Reviewer: command.Reviewer, Role: command.Role, Approved: command.Approved,
			Opinion: command.Opinion, SubmittedAt: now,
		}
		events, err := s.repository.AuditEvents("")
		if err != nil {
			return nil, err
		}
		sequence, hash := uint64(0), ""
		if len(events) > 0 {
			sequence, hash = events[len(events)-1].Sequence, events[len(events)-1].Hash
		}
		readiness, err := batch.BuildReviewReadiness(sequence, hash, s.repository.Health() == nil)
		if err != nil {
			return nil, err
		}
		if !readiness.Ready || readiness.Snapshot == nil {
			return nil, domain.FieldError{Field: "reviewReadiness", Message: "复核就绪清单未通过"}
		}
		if command.EvidenceDigest != readiness.Snapshot.Digest || command.EvidenceVersion != readiness.Snapshot.BatchVersion {
			return nil, domain.FieldError{Field: "evidenceDigest", Message: "证据摘要或版本已变化，请重新加载后提交"}
		}
		if err := batch.AddReviewWithSnapshot(review, *readiness.Snapshot, now); err != nil {
			return nil, err
		}
		return batch, nil
	})
}

func (s *Service) IssueCertificate(batchID string, command IssueCertificateCommand) (*domain.Batch, error) {
	return s.mutateBatch("issue_certificate", "certificate.issued", batchID, command.CommandMeta, func(batch *domain.Batch, now time.Time) (any, error) {
		events, err := s.repository.AuditEvents("")
		if err != nil {
			return nil, err
		}
		references := make([]domain.AuditReference, 0, len(events))
		firstHash, lastHash := "", ""
		for _, event := range events {
			if firstHash == "" {
				firstHash = event.Hash
			}
			lastHash = event.Hash
			if event.BatchID == batch.ID {
				references = append(references, domain.AuditReference{Sequence: event.Sequence, Hash: event.Hash, Operation: event.Operation})
			}
		}
		evidence, err := domain.BuildEvidenceList(batch, references)
		if err != nil {
			return nil, err
		}
		_, err = batch.IssueCertificateWithEvidence(s.idFactory("certificate"), evidence, firstHash, lastHash, now)
		return batch, err
	})
}

type batchMutation func(batch *domain.Batch, now time.Time) (any, error)

func (s *Service) mutateBatch(operation, auditOperation, batchID string, meta CommandMeta, mutation batchMutation) (*domain.Batch, error) {
	key, err := operationKey(operation, meta.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(meta.Actor) == "" {
		return nil, domain.FieldError{Field: "actor", Message: "操作人不能为空"}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var cached domain.Batch
	if found, err := s.repository.GetIdempotency(key, &cached); err != nil {
		return nil, err
	} else if found {
		return domain.CloneBatch(&cached)
	}
	batch, err := s.repository.GetBatch(batchID)
	if err != nil {
		return nil, err
	}
	if meta.ExpectedVersion != batch.Version {
		return nil, VersionConflictError{BatchID: batchID, Want: meta.ExpectedVersion, Actual: batch.Version}
	}
	beforeVersion := batch.Version
	result, err := mutation(batch, s.clock().UTC())
	if err != nil {
		return nil, err
	}
	if batch.Version <= beforeVersion {
		return nil, fmt.Errorf("操作 %s 未推进批次版本", operation)
	}
	if err := s.repository.Commit(persistence.Commit{
		Batch: batch, Operation: auditOperation, Actor: meta.Actor,
		Details:        map[string]any{"fromVersion": beforeVersion, "toVersion": batch.Version, "result": result},
		IdempotencyKey: key, Response: batch,
	}); err != nil {
		return nil, err
	}
	return domain.CloneBatch(batch)
}
