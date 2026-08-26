package persistence

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"isolation-chamber-commissioning/internal/domain"
)

func validateAudit(path string) (uint64, string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("打开审计日志: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	var seq uint64
	prev := ""
	line := 0
	for scanner.Scan() {
		line++
		var ev AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			return 0, "", fmt.Errorf("审计日志第 %d 行无效: %w", line, err)
		}
		if ev.Sequence != seq+1 {
			return 0, "", fmt.Errorf("审计序号中断：%d", ev.Sequence)
		}
		if ev.PreviousHash != prev {
			return 0, "", fmt.Errorf("审计前序摘要不匹配：%d", ev.Sequence)
		}
		expected, err := auditHash(ev)
		if err != nil {
			return 0, "", err
		}
		if expected != ev.Hash {
			return 0, "", fmt.Errorf("审计摘要不匹配：%d", ev.Sequence)
		}
		seq = ev.Sequence
		prev = ev.Hash
	}
	if err := scanner.Err(); err != nil {
		return 0, "", fmt.Errorf("读取审计日志: %w", err)
	}
	return seq, prev, nil
}

func auditHash(ev AuditEvent) (string, error) {
	ev.Hash = ""
	b, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func cloneCase(c *domain.CommissioningCase) (*domain.CommissioningCase, error) {
	if c == nil {
		return nil, nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var out domain.CommissioningCase
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
