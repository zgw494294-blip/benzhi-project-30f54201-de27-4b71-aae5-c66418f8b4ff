package persistence

import "errors"

var (
	ErrNotFound      = errors.New("持久化对象不存在")
	ErrAlreadyExists = errors.New("持久化对象已经存在")
	ErrAuditCorrupt  = errors.New("审计日志完整性校验失败")
	ErrSchemaVersion = errors.New("快照 schemaVersion 不受支持")
	ErrConflict      = errors.New("本地证据库并发冲突")
)
