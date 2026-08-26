package domain

import (
	"strings"
	"time"
)

func (b *Batch) AddReview(review SafetyReview, now time.Time) error {
	return b.addReview(review, nil, now)
}

func (b *Batch) AddReviewWithSnapshot(review SafetyReview, snapshot ReviewEvidenceSnapshot, now time.Time) error {
	return b.addReview(review, &snapshot, now)
}

func (b *Batch) addReview(review SafetyReview, snapshot *ReviewEvidenceSnapshot, now time.Time) error {
	if err := b.ensureMutable(); err != nil {
		return err
	}
	if b.Status != StatusReviewing {
		if b.OpenDeviationCount() > 0 {
			return ErrOpenDeviation
		}
		return ErrInvalidTransition
	}
	if len(b.Reviews) >= 2 {
		return ErrReviewLocked
	}
	review.SubmittedAt = now.UTC()
	if err := review.NormalizeAndValidate(); err != nil {
		return err
	}
	for _, existing := range b.Reviews {
		if strings.EqualFold(existing.Reviewer, review.Reviewer) {
			return invalid("reviewer", "两次复核必须由不同人员提交")
		}
	}
	if snapshot != nil {
		if strings.TrimSpace(snapshot.Digest) == "" {
			return invalid("evidenceDigest", "复核证据摘要不能为空")
		}
		if b.ReviewSnapshot == nil {
			copySnapshot := *snapshot
			b.ReviewSnapshot = &copySnapshot
		} else if b.ReviewSnapshot.Digest != snapshot.Digest || b.ReviewSnapshot.BatchVersion != snapshot.BatchVersion {
			return invalid("evidenceDigest", "复核证据摘要或证据版本已经变化，请重新加载")
		}
		review.EvidenceDigest = snapshot.Digest
		review.EvidenceVersion = snapshot.BatchVersion
		review.DeviationCount = snapshot.DeviationCount
	}
	b.Reviews = append(b.Reviews, review)
	b.touch(now)
	return nil
}

func (b *Batch) IssueCertificate(id string, now time.Time) (*Certificate, error) {
	evidence := EvidenceList{BatchID: b.ID, ScopeDigest: b.FrozenScope.ScopeDigest}
	return b.IssueCertificateWithEvidence(id, evidence, "", "", now)
}

func (b *Batch) IssueCertificateWithEvidence(id string, evidence EvidenceList, firstAuditHash, lastAuditHash string, now time.Time) (*Certificate, error) {
	if err := b.ensureMutable(); err != nil {
		return nil, err
	}
	if b.Status != StatusReviewing || b.OpenDeviationCount() != 0 {
		return nil, ErrOpenDeviation
	}
	if !reviewsApproved(b.Reviews) {
		return nil, invalid("reviews", "需要两名不同人员给出通过意见")
	}
	evidenceListDigest, err := digestValue(evidence)
	if err != nil {
		return nil, err
	}
	payload := CertificatePayload{
		CertificateID: id, BatchID: b.ID, CableSection: b.CableSection,
		CircuitName: b.CircuitName, TestOwner: b.TestOwner, CertificateVersion: "1.0",
		ReviewerA: b.Reviews[0].Reviewer, ReviewerB: b.Reviews[1].Reviewer,
		ScopeDigest: b.FrozenScope.ScopeDigest, MeasurementCount: len(b.Measurements),
		DeviationCount: len(b.Deviations), IssuedAt: now.UTC(),
		Reviews:            append([]SafetyReview(nil), b.Reviews...),
		EvidenceListDigest: evidenceListDigest,
	}
	evidenceDigest, err := EvidenceDigest(b)
	if err != nil {
		return nil, err
	}
	payload.EvidenceDigest = evidenceDigest
	digest, err := digestCertificatePayload(payload)
	if err != nil {
		return nil, err
	}
	certificate := &Certificate{
		ID: id, BatchID: b.ID, CertificateVersion: "1.0",
		ReviewerA: payload.ReviewerA, ReviewerB: payload.ReviewerB,
		EvidenceDigest: digest, IssuedAt: now.UTC(), SealedPayload: payload,
	}
	b.Certificate = certificate
	verificationPackage, err := NewVerificationPackage(*certificate, evidence, firstAuditHash, lastAuditHash)
	if err != nil {
		return nil, err
	}
	b.VerificationPackage = verificationPackage
	b.Status = StatusSealed
	b.touch(now)
	return certificate, nil
}
