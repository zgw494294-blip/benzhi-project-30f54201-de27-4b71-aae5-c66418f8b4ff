package domain

import (
	"fmt"
	"sort"
)

type ChecklistItem struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

type ReviewEvidenceSnapshot struct {
	Digest           string `json:"digest"`
	BatchVersion     int64  `json:"batchVersion"`
	ScopeDigest      string `json:"scopeDigest"`
	DiagnosisRunID   string `json:"diagnosisRunID"`
	MeasurementCount int    `json:"measurementCount"`
	DeviationCount   int    `json:"deviationCount"`
	AuditSequence    uint64 `json:"auditSequence"`
	AuditHash        string `json:"auditHash"`
}

type ReviewReadiness struct {
	Ready     bool                    `json:"ready"`
	Checklist []ChecklistItem         `json:"checklist"`
	Snapshot  *ReviewEvidenceSnapshot `json:"snapshot,omitempty"`
}

func (b *Batch) BuildReviewReadiness(auditSequence uint64, auditHash string, auditHealthy bool) (ReviewReadiness, error) {
	if b.ReviewSnapshot != nil {
		copySnapshot := *b.ReviewSnapshot
		return ReviewReadiness{Ready: true, Snapshot: &copySnapshot, Checklist: []ChecklistItem{{Code: "snapshot", Passed: true, Message: "复核证据快照已经锁定"}}}, nil
	}
	coverage := b.DiagnosisReadiness()
	latestRun := ""
	if len(b.DiagnosisReports) > 0 {
		latestRun = b.DiagnosisReports[len(b.DiagnosisReports)-1].RunID
	}
	items := []ChecklistItem{
		{Code: "scope", Passed: b.FrozenScope.ScopeDigest != "", Message: "冻结范围摘要完整"},
		{Code: "coverage", Passed: coverage.Ready, Message: "所有冻结点初始采样覆盖完整"},
		{Code: "diagnosis", Passed: latestRun != "", Message: "诊断运行报告已保存"},
		{Code: "deviations", Passed: b.OpenDeviationCount() == 0, Message: "全部偏差已经关闭"},
		{Code: "audit", Passed: auditHealthy && auditSequence > 0 && auditHash != "", Message: "审计链连续且摘要可用"},
	}
	ready := b.Status == StatusReviewing
	for index := range items {
		if !items[index].Passed {
			ready = false
			items[index].Message += "（未通过）"
		}
	}
	result := ReviewReadiness{Ready: ready, Checklist: items}
	if !ready {
		return result, nil
	}
	snapshot := ReviewEvidenceSnapshot{BatchVersion: b.Version, ScopeDigest: b.FrozenScope.ScopeDigest, DiagnosisRunID: latestRun,
		MeasurementCount: len(b.Measurements), DeviationCount: len(b.Deviations), AuditSequence: auditSequence, AuditHash: auditHash}
	digest, err := digestValue(snapshot)
	if err != nil {
		return ReviewReadiness{}, err
	}
	snapshot.Digest = digest
	result.Snapshot = &snapshot
	return result, nil
}

type PointEvidence struct {
	PointID            string   `json:"pointID"`
	MeasurementIDs     []string `json:"measurementIDs"`
	DiagnosisRunIDs    []string `json:"diagnosisRunIDs"`
	ClosedDeviationIDs []string `json:"closedDeviationIDs"`
}

type AuditReference struct {
	Sequence  uint64 `json:"sequence"`
	Hash      string `json:"hash"`
	Operation string `json:"operation"`
}

type EvidenceList struct {
	BatchID              string           `json:"batchID"`
	ScopeDigest          string           `json:"scopeDigest"`
	Points               []PointEvidence  `json:"points"`
	Reviewers            []string         `json:"reviewers"`
	Reviews              []SafetyReview   `json:"reviews"`
	ReviewEvidenceDigest string           `json:"reviewEvidenceDigest"`
	AuditReferences      []AuditReference `json:"auditReferences"`
}

func BuildEvidenceList(batch *Batch, references []AuditReference) (EvidenceList, error) {
	if batch == nil || batch.ReviewSnapshot == nil {
		return EvidenceList{}, invalid("reviewSnapshot", "缺少已锁定的复核证据快照")
	}
	list := EvidenceList{BatchID: batch.ID, ScopeDigest: batch.FrozenScope.ScopeDigest, ReviewEvidenceDigest: batch.ReviewSnapshot.Digest,
		AuditReferences: append([]AuditReference(nil), references...)}
	for _, review := range batch.Reviews {
		list.Reviewers = append(list.Reviewers, review.Reviewer)
		list.Reviews = append(list.Reviews, review)
	}
	for _, point := range batch.FrozenScope.Points {
		entry := PointEvidence{PointID: point.ID}
		for _, measurement := range batch.Measurements {
			if measurement.PointID == point.ID {
				entry.MeasurementIDs = append(entry.MeasurementIDs, measurement.ID)
			}
		}
		for _, report := range batch.DiagnosisReports {
			for _, result := range report.Results {
				if result.PointID == point.ID {
					entry.DiagnosisRunIDs = appendUnique(entry.DiagnosisRunIDs, report.RunID)
					break
				}
			}
		}
		for _, deviation := range batch.Deviations {
			if deviation.PointID == point.ID && deviation.IsClosed() {
				entry.ClosedDeviationIDs = append(entry.ClosedDeviationIDs, deviation.ID)
			}
		}
		sort.Strings(entry.MeasurementIDs)
		sort.Strings(entry.DiagnosisRunIDs)
		sort.Strings(entry.ClosedDeviationIDs)
		list.Points = append(list.Points, entry)
	}
	return list, nil
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

type VerificationPackage struct {
	Certificate        Certificate  `json:"certificate"`
	EvidenceList       EvidenceList `json:"evidenceList"`
	AuditFirstHash     string       `json:"auditFirstHash"`
	AuditLastHash      string       `json:"auditLastHash"`
	CertificateDigest  string       `json:"certificateDigest"`
	EvidenceListDigest string       `json:"evidenceListDigest"`
	AuditDigest        string       `json:"auditDigest"`
	ContentDigest      string       `json:"contentDigest"`
}

func NewVerificationPackage(certificate Certificate, evidence EvidenceList, firstHash, lastHash string) (*VerificationPackage, error) {
	certificateDigest, err := digestValue(certificate)
	if err != nil {
		return nil, err
	}
	evidenceDigest, err := digestValue(evidence)
	if err != nil {
		return nil, err
	}
	auditDigest, err := digestValue(struct {
		First      string           `json:"first"`
		Last       string           `json:"last"`
		References []AuditReference `json:"references"`
	}{firstHash, lastHash, evidence.AuditReferences})
	if err != nil {
		return nil, err
	}
	result := &VerificationPackage{Certificate: certificate, EvidenceList: evidence, AuditFirstHash: firstHash, AuditLastHash: lastHash,
		CertificateDigest: certificateDigest, EvidenceListDigest: evidenceDigest, AuditDigest: auditDigest}
	result.ContentDigest, err = result.recalculateContentDigest()
	return result, err
}

func (p VerificationPackage) recalculateContentDigest() (string, error) {
	return digestValue(struct {
		CertificateDigest  string `json:"certificateDigest"`
		EvidenceListDigest string `json:"evidenceListDigest"`
		AuditDigest        string `json:"auditDigest"`
	}{p.CertificateDigest, p.EvidenceListDigest, p.AuditDigest})
}

func (p VerificationPackage) ValidateDigests() []string {
	var failures []string
	certificateDigest, _ := digestValue(p.Certificate)
	if certificateDigest != p.CertificateDigest {
		failures = append(failures, "证书组成部分摘要不一致")
	}
	evidenceDigest, _ := digestValue(p.EvidenceList)
	if evidenceDigest != p.EvidenceListDigest {
		failures = append(failures, "证据清单摘要不一致")
	}
	auditDigest, _ := digestValue(struct {
		First      string           `json:"first"`
		Last       string           `json:"last"`
		References []AuditReference `json:"references"`
	}{p.AuditFirstHash, p.AuditLastHash, p.EvidenceList.AuditReferences})
	if auditDigest != p.AuditDigest {
		failures = append(failures, "审计关联摘要不一致")
	}
	contentDigest, _ := p.recalculateContentDigest()
	if contentDigest != p.ContentDigest {
		failures = append(failures, "核验包内容摘要不一致")
	}
	if !p.Certificate.Verify() {
		failures = append(failures, "证书载荷摘要不一致")
	}
	if p.Certificate.SealedPayload.EvidenceListDigest != p.EvidenceListDigest {
		failures = append(failures, "证书未关联当前证据清单")
	}
	return failures
}

func MissingEvidenceItems(batch *Batch, list EvidenceList) []string {
	measurementIDs, runIDs, deviationIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, item := range batch.Measurements {
		measurementIDs[item.ID] = true
	}
	for _, item := range batch.DiagnosisReports {
		runIDs[item.RunID] = true
	}
	for _, item := range batch.Deviations {
		if item.IsClosed() {
			deviationIDs[item.ID] = true
		}
	}
	var failures []string
	for _, point := range list.Points {
		for _, id := range point.MeasurementIDs {
			if !measurementIDs[id] {
				failures = append(failures, fmt.Sprintf("试验点 %s 缺少测量记录 %s", point.PointID, id))
			}
		}
		for _, id := range point.DiagnosisRunIDs {
			if !runIDs[id] {
				failures = append(failures, fmt.Sprintf("试验点 %s 缺少诊断运行 %s", point.PointID, id))
			}
		}
		for _, id := range point.ClosedDeviationIDs {
			if !deviationIDs[id] {
				failures = append(failures, fmt.Sprintf("试验点 %s 缺少关闭偏差 %s", point.PointID, id))
			}
		}
	}
	return failures
}
