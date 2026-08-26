package persistence

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"isolation-chamber-commissioning/internal/domain"
)

type CaseSummary struct {
	ID          string            `json:"id"`
	CaseNumber  string            `json:"caseNumber"`
	ChamberName string            `json:"chamberName"`
	Status      domain.CaseStatus `json:"status"`
	Version     int               `json:"version"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Progress    int               `json:"progress"`
	Total       int               `json:"total"`
}

func (s *Store) ListCases(query string) []CaseSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]CaseSummary, 0, len(s.state.Cases))
	for _, c := range s.state.Cases {
		if q != "" && !strings.Contains(strings.ToLower(c.CaseNumber+" "+c.ChamberName), q) {
			continue
		}
		progress := 0
		seen := map[string]bool{}
		for _, r := range c.Runs {
			if r.Verdict == domain.VerdictPass {
				seen[r.CheckpointID] = true
			}
		}
		progress = len(seen)
		total := 0
		if c.Protocol != nil {
			total = len(c.Protocol.Checkpoints)
		}
		out = append(out, CaseSummary{c.ID, c.CaseNumber, c.ChamberName, c.Status, c.Version, c.UpdatedAt, progress, total})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *Store) Credential(id string) (domain.ActivationCredential, domain.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	caseID := s.state.Credentials[id]
	if caseID == "" {
		return domain.ActivationCredential{}, domain.Snapshot{}, domain.NotFound("启用凭据", id)
	}
	c := s.state.Cases[caseID]
	if c == nil || c.Credential == nil {
		return domain.ActivationCredential{}, domain.Snapshot{}, errors.New("凭据索引损坏")
	}
	snapshot, ok := s.state.Snapshots[id]
	if !ok {
		return domain.ActivationCredential{}, domain.Snapshot{}, errors.New("凭据封存快照缺失")
	}
	return *c.Credential, snapshot, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return nil
}

func (s *Store) AuditEvents(caseID string) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, err := os.Open(s.auditPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []AuditEvent
	dec := json.NewDecoder(f)
	for {
		var ev AuditEvent
		err := dec.Decode(&ev)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if caseID == "" || ev.CaseID == caseID {
			out = append(out, ev)
		}
	}
	return out, nil
}
