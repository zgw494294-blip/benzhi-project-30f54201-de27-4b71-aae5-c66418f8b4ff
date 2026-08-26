package persistence

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type unsignedAuditEvent struct {
	Sequence     uint64          `json:"sequence"`
	OccurredAt   any             `json:"occurredAt"`
	Operation    string          `json:"operation"`
	BatchID      string          `json:"batchID"`
	Actor        string          `json:"actor"`
	Version      int64           `json:"version"`
	Details      json.RawMessage `json:"details,omitempty"`
	PreviousHash string          `json:"previousHash"`
}

func signAudit(event AuditEvent) (string, error) {
	unsigned := unsignedAuditEvent{
		Sequence: event.Sequence, OccurredAt: event.OccurredAt.UTC(), Operation: event.Operation,
		BatchID: event.BatchID, Actor: event.Actor, Version: event.Version,
		Details: event.Details, PreviousHash: event.PreviousHash,
	}
	encoded, err := json.Marshal(unsigned)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func appendAudit(path string, event AuditEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("编码审计事件: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("打开审计日志: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("追加审计日志: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步审计日志: %w", err)
	}
	return nil
}

func readAndValidateAudit(path string) ([]AuditEvent, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("打开审计日志: %w", err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	events := make([]AuditEvent, 0)
	var previousHash string
	var expectedSequence uint64 = 1
	for {
		line, readErr := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var event AuditEvent
			if err := json.Unmarshal(line, &event); err != nil {
				return nil, fmt.Errorf("%w: 第 %d 行不是合法 JSON", ErrAuditCorrupt, expectedSequence)
			}
			expectedHash, err := signAudit(event)
			if err != nil {
				return nil, err
			}
			if event.Sequence != expectedSequence || event.PreviousHash != previousHash || event.Hash != expectedHash {
				return nil, fmt.Errorf("%w: 序号 %d 的链路不连续", ErrAuditCorrupt, event.Sequence)
			}
			events = append(events, event)
			previousHash = event.Hash
			expectedSequence++
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取审计日志: %w", readErr)
		}
	}
	return events, nil
}
