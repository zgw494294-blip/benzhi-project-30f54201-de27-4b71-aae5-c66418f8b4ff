package persistence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"isolation-chamber-commissioning/internal/domain"
)

func (s *Store) GetCase(id string) (*domain.CommissioningCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.state.Cases[id]
	if c == nil {
		return nil, domain.NotFound("验证案", id)
	}
	return cloneCase(c)
}

func (s *Store) FindIdempotency(key string) (CommandResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.state.Idempotency[key]
	if ok {
		v.Body = append(json.RawMessage(nil), v.Body...)
	}
	return v, ok
}

func (s *Store) NextCaseNumber(now time.Time) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", errors.New("持久化已关闭")
	}
	s.state.CaseSequence++
	return fmt.Sprintf("ICC-%s-%04d", now.UTC().Format("20060102"), s.state.CaseSequence), nil
}

type Commit struct {
	IdempotencyKey     string
	ExpectedVersion    int
	Case               *domain.CommissioningCase
	Action             string
	Payload            any
	Result             CommandResult
	CredentialSnapshot *domain.Snapshot
	Now                time.Time
}

func (s *Store) Commit(c Commit) (CommandResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return CommandResult{}, false, errors.New("持久化已关闭")
	}
	if strings.TrimSpace(c.IdempotencyKey) == "" {
		return CommandResult{}, false, domain.Validation("Idempotency-Key", "幂等键不能为空")
	}
	if previous, ok := s.state.Idempotency[c.IdempotencyKey]; ok {
		return previous, true, nil
	}
	if c.Case == nil {
		return CommandResult{}, false, errors.New("提交案卷不能为空")
	}
	existing := s.state.Cases[c.Case.ID]
	if existing == nil {
		if c.ExpectedVersion != 0 {
			return CommandResult{}, false, domain.Conflict(c.ExpectedVersion, 0)
		}
	} else if existing.Version != c.ExpectedVersion {
		return CommandResult{}, false, domain.Conflict(c.ExpectedVersion, existing.Version)
	}
	copyCase, err := cloneCase(c.Case)
	if err != nil {
		return CommandResult{}, false, err
	}
	payload, err := json.Marshal(c.Payload)
	if err != nil {
		return CommandResult{}, false, err
	}
	ev := AuditEvent{Sequence: s.state.LastAuditSeq + 1, PreviousHash: s.state.LastAuditHash, Timestamp: c.Now.UTC(), CaseID: c.Case.ID, CaseVersion: c.Case.Version, Action: c.Action, Payload: payload}
	ev.Hash, err = auditHash(ev)
	if err != nil {
		return CommandResult{}, false, err
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return CommandResult{}, false, err
	}
	line = append(line, '\n')
	if err := appendSynced(s.auditPath, line); err != nil {
		return CommandResult{}, false, err
	}
	oldCases := s.state.Cases
	oldIdem := s.state.Idempotency
	oldCred := s.state.Credentials
	oldSnap := s.state.Snapshots
	oldSeq := s.state.LastAuditSeq
	oldHash := s.state.LastAuditHash
	s.state.Cases = copyCaseMap(oldCases)
	s.state.Idempotency = copyIdem(oldIdem)
	s.state.Credentials = copyStringMap(oldCred)
	s.state.Snapshots = copySnapshots(oldSnap)
	s.state.Cases[copyCase.ID] = copyCase
	s.state.Idempotency[c.IdempotencyKey] = CommandResult{Status: c.Result.Status, Body: append(json.RawMessage(nil), c.Result.Body...)}
	if copyCase.Credential != nil {
		s.state.Credentials[copyCase.Credential.ID] = copyCase.ID
		if c.CredentialSnapshot != nil {
			s.state.Snapshots[copyCase.Credential.ID] = *c.CredentialSnapshot
		}
	}
	s.state.LastAuditSeq = ev.Sequence
	s.state.LastAuditHash = ev.Hash
	if err := s.writeSnapshot(); err != nil {
		s.state.Cases = oldCases
		s.state.Idempotency = oldIdem
		s.state.Credentials = oldCred
		s.state.Snapshots = oldSnap
		s.state.LastAuditSeq = oldSeq
		s.state.LastAuditHash = oldHash
		return CommandResult{}, false, fmt.Errorf("保存快照失败（审计日志已追加，需恢复）: %w", err)
	}
	return c.Result, false, nil
}

func copyCaseMap(in map[string]*domain.CommissioningCase) map[string]*domain.CommissioningCase {
	out := make(map[string]*domain.CommissioningCase, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func copyIdem(in map[string]CommandResult) map[string]CommandResult {
	out := make(map[string]CommandResult, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func copySnapshots(in map[string]domain.Snapshot) map[string]domain.Snapshot {
	out := make(map[string]domain.Snapshot, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func appendSynced(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return fmt.Errorf("打开审计日志: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("追加审计日志: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("同步审计日志: %w", err)
	}
	return f.Close()
}

func (s *Store) writeSnapshot() error {
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "snapshot-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0640); err != nil {
		return err
	}
	if _, err := io.Copy(tmp, bytes.NewReader(b)); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.snapshotPath); err != nil {
		return err
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return err
	}
	ok = true
	return nil
}
