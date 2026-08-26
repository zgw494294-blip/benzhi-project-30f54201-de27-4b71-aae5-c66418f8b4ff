package application

import (
	"pdconsole/internal/domain"
	"pdconsole/internal/persistence"
)

type Repository interface {
	GetBatch(id string) (*domain.Batch, error)
	ListBatches() ([]*domain.Batch, error)
	GetIdempotency(key string, target any) (bool, error)
	Commit(change persistence.Commit) error
	AuditEvents(batchID string) ([]persistence.AuditEvent, error)
	Health() error
}
