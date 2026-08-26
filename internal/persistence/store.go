package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"isolation-chamber-commissioning/internal/domain"
)

const SchemaVersion = 1

type CommandResult struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

type diskState struct {
	SchemaVersion int                                  `json:"schemaVersion"`
	CaseSequence  int                                  `json:"caseSequence"`
	Cases         map[string]*domain.CommissioningCase `json:"cases"`
	Credentials   map[string]string                    `json:"credentials"`
	Idempotency   map[string]CommandResult             `json:"idempotency"`
	Snapshots     map[string]domain.Snapshot           `json:"snapshots"`
	LastAuditSeq  uint64                               `json:"lastAuditSeq"`
	LastAuditHash string                               `json:"lastAuditHash"`
}

type AuditEvent struct {
	Sequence     uint64          `json:"sequence"`
	PreviousHash string          `json:"previousHash"`
	Timestamp    time.Time       `json:"timestamp"`
	CaseID       string          `json:"caseId"`
	CaseVersion  int             `json:"caseVersion"`
	Action       string          `json:"action"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Hash         string          `json:"hash"`
}

type Store struct {
	mu           sync.RWMutex
	dir          string
	snapshotPath string
	auditPath    string
	state        diskState
	closed       bool
}

type Diagnostics struct {
	SchemaVersion     int    `json:"schemaVersion"`
	CaseCount         int    `json:"caseCount"`
	CredentialCount   int    `json:"credentialCount"`
	IdempotencyCount  int    `json:"idempotencyCount"`
	LastAuditSequence uint64 `json:"lastAuditSequence"`
	LastAuditHash     string `json:"lastAuditHash"`
	Writable          bool   `json:"writable"`
}

func (s *Store) Diagnostics() Diagnostics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Diagnostics{SchemaVersion: s.state.SchemaVersion, CaseCount: len(s.state.Cases), CredentialCount: len(s.state.Credentials), IdempotencyCount: len(s.state.Idempotency), LastAuditSequence: s.state.LastAuditSeq, LastAuditHash: s.state.LastAuditHash, Writable: !s.closed}
}

func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("持久化目录不能为空")
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("创建持久化目录: %w", err)
	}
	s := &Store{dir: dir, snapshotPath: filepath.Join(dir, "snapshot.json"), auditPath: filepath.Join(dir, "audit.jsonl")}
	s.state = diskState{SchemaVersion: SchemaVersion, Cases: map[string]*domain.CommissioningCase{}, Credentials: map[string]string{}, Idempotency: map[string]CommandResult{}, Snapshots: map[string]domain.Snapshot{}}
	if err := s.loadSnapshot(); err != nil {
		return nil, err
	}
	seq, hash, err := validateAudit(s.auditPath)
	if err != nil {
		return nil, err
	}
	if seq != s.state.LastAuditSeq || hash != s.state.LastAuditHash {
		return nil, fmt.Errorf("审计链与快照不一致：日志序号 %d，快照序号 %d", seq, s.state.LastAuditSeq)
	}
	if err := s.rebuildIndexes(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) loadSnapshot() error {
	b, err := os.ReadFile(s.snapshotPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取快照: %w", err)
	}
	var state diskState
	if err := json.Unmarshal(b, &state); err != nil {
		return fmt.Errorf("解析快照: %w", err)
	}
	if state.SchemaVersion != SchemaVersion {
		return fmt.Errorf("不支持的快照 schemaVersion：%d", state.SchemaVersion)
	}
	if state.Cases == nil {
		state.Cases = map[string]*domain.CommissioningCase{}
	}
	if state.Credentials == nil {
		state.Credentials = map[string]string{}
	}
	if state.Idempotency == nil {
		state.Idempotency = map[string]CommandResult{}
	}
	if state.Snapshots == nil {
		state.Snapshots = map[string]domain.Snapshot{}
	}
	s.state = state
	return nil
}

func (s *Store) rebuildIndexes() error {
	rebuilt := map[string]string{}
	for id, c := range s.state.Cases {
		if c.ID != id {
			return fmt.Errorf("快照案卷索引损坏：%s", id)
		}
		if c.Credential != nil {
			if other, ok := rebuilt[c.Credential.ID]; ok && other != id {
				return fmt.Errorf("凭据编号重复：%s", c.Credential.ID)
			}
			rebuilt[c.Credential.ID] = id
		}
	}
	s.state.Credentials = rebuilt
	return nil
}
