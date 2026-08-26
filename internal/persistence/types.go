package persistence

import (
	"encoding/json"
	"time"

	"pdconsole/internal/domain"
)

const CurrentSchemaVersion = 1

type IdempotencyRecord struct {
	Key       string          `json:"key"`
	Operation string          `json:"operation"`
	BatchID   string          `json:"batchID"`
	Response  json.RawMessage `json:"response"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Snapshot struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Sequence      uint64                       `json:"sequence"`
	LastAuditHash string                       `json:"lastAuditHash"`
	Batches       map[string]*domain.Batch     `json:"batches"`
	Idempotency   map[string]IdempotencyRecord `json:"idempotency"`
	SavedAt       time.Time                    `json:"savedAt"`
}

type AuditEvent struct {
	Sequence     uint64          `json:"sequence"`
	OccurredAt   time.Time       `json:"occurredAt"`
	Operation    string          `json:"operation"`
	BatchID      string          `json:"batchID"`
	Actor        string          `json:"actor"`
	Version      int64           `json:"version"`
	Details      json.RawMessage `json:"details,omitempty"`
	PreviousHash string          `json:"previousHash"`
	Hash         string          `json:"hash"`
}

type Commit struct {
	Batch          *domain.Batch
	Operation      string
	Actor          string
	Details        any
	IdempotencyKey string
	Response       any
}
