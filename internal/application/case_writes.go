package application

import (
	"strings"

	"isolation-chamber-commissioning/internal/domain"
	"isolation-chamber-commissioning/internal/persistence"
	"isolation-chamber-commissioning/internal/verification"
)

func (s *Service) CreateCase(cmd CreateCaseCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	key := scoped("create", "", cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	now := s.now().UTC()
	number, err := s.store.NextCaseNumber(now)
	if err != nil {
		return WriteResult{}, err
	}
	id, err := randomID("case-")
	if err != nil {
		return WriteResult{}, err
	}
	c, err := domain.CreateCase(domain.NewCase{ID: id, CaseNumber: number, ChamberName: cmd.ChamberName, Zones: cmd.Zones, AirflowDirection: cmd.AirflowDirection, Limits: cmd.AcceptanceLimits, Actor: actor(cmd.Actor, "验证工程师"), Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	r, err := result(201, c)
	if err != nil {
		return WriteResult{}, err
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: 0, Case: c, Action: "CASE_CREATED", Payload: map[string]any{"caseNumber": number}, Result: r, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}

func (s *Service) ReviseCase(caseID string, cmd ReviseCaseCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	key := scoped("revise", caseID, cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	c, err := s.store.GetCase(caseID)
	if err != nil {
		return WriteResult{}, err
	}
	revision, err := domain.NormalizeBaseData(cmd.ChamberName, cmd.Zones, cmd.AirflowDirection, cmd.AcceptanceLimits)
	if err != nil {
		return WriteResult{}, err
	}
	before, now := c.Version, s.now().UTC()
	fields, err := c.ReviseBase(cmd.ExpectedVersion, revision, actor(cmd.Actor, "验证工程师"), now)
	if err != nil {
		return WriteResult{}, err
	}
	r, err := result(200, c)
	if err != nil {
		return WriteResult{}, err
	}
	auditPayload := map[string]any{"fields": fields}
	if len(c.Timeline) > 0 && c.Timeline[len(c.Timeline)-1].Type == "CASE_REVISED" {
		auditPayload = c.Timeline[len(c.Timeline)-1].Data
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: before, Case: c, Action: "CASE_REVISED", Payload: auditPayload, Result: r, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}

func (s *Service) FreezePreflight(caseID string, cmd FreezePreflightCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	key := scoped("freeze-preflight", caseID, cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	c, err := s.store.GetCase(caseID)
	if err != nil {
		return WriteResult{}, err
	}
	if cmd.ExpectedVersion != c.Version {
		return WriteResult{}, domain.Conflict(cmd.ExpectedVersion, c.Version)
	}
	if c.Status != domain.StatusDraft {
		return WriteResult{}, domain.State("只有草拟状态可执行冻结预检")
	}
	checks := verification.FreezeChecks(c.AcceptanceLimits)
	if _, err := domain.NormalizeBaseData(c.ChamberName, c.Zones, c.AirflowDirection, c.AcceptanceLimits); err != nil {
		checks[0].BlockingReasons = append(checks[0].BlockingReasons, err.Error())
	}
	if err := c.AcceptanceLimits.Validate(); err != nil {
		checks[0].BlockingReasons = append(checks[0].BlockingReasons, err.Error())
	}
	preview, err := verification.ProtocolPreviewDigest(c.ID, c.Version, verification.RuleSetVersion, c.AcceptanceLimits, domain.DefaultCheckpoints())
	if err != nil {
		return WriteResult{}, err
	}
	token, err := randomID("confirm-")
	if err != nil {
		return WriteResult{}, err
	}
	order := make([]string, 0, len(checks))
	blocked := false
	for _, check := range checks {
		order = append(order, check.CheckpointID)
		blocked = blocked || len(check.BlockingReasons) > 0
	}
	if blocked {
		token = ""
	}
	now := s.now().UTC()
	confirmation := domain.FreezeConfirmation{Token: token, CaseVersion: c.Version, RuleSetVersion: verification.RuleSetVersion, CheckpointOrder: order, Checks: checks, PreviewSummary: preview, CreatedAt: now, CreatedBy: actor(cmd.Actor, "验证工程师")}
	if !blocked {
		if err := c.AddFreezeConfirmation(confirmation, confirmation.CreatedBy, now); err != nil {
			return WriteResult{}, err
		}
	}
	response := struct {
		Report   domain.FreezeConfirmation `json:"report"`
		Blocking bool                      `json:"blocking"`
	}{confirmation, blocked}
	r, err := result(200, response)
	if err != nil {
		return WriteResult{}, err
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: c.Version, Case: c, Action: "FREEZE_PREFLIGHTED", Payload: confirmation, Result: r, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}

func (s *Service) FreezeProtocol(caseID string, cmd FreezeProtocolCommand) (WriteResult, error) {
	if err := requiredKey(cmd.IdempotencyKey); err != nil {
		return WriteResult{}, err
	}
	key := scoped("freeze", caseID, cmd.IdempotencyKey)
	if r, ok := s.store.FindIdempotency(key); ok {
		return WriteResult{r.Status, r.Body, true}, nil
	}
	c, err := s.store.GetCase(caseID)
	if err != nil {
		return WriteResult{}, err
	}
	before := c.Version
	now := s.now().UTC()
	by := actor(cmd.FrozenBy, "验证工程师")
	preview, err := verification.ProtocolPreviewDigest(c.ID, c.Version, verification.RuleSetVersion, c.AcceptanceLimits, domain.DefaultCheckpoints())
	if err != nil {
		return WriteResult{}, err
	}
	confirmationToken := cmd.ConfirmationToken
	if strings.TrimSpace(confirmationToken) == "" {
		confirmationToken = cmd.ConfirmationID
	}
	if err := c.UseFreezeConfirmation(cmd.ExpectedVersion, confirmationToken, preview, by, now); err != nil {
		return WriteResult{}, err
	}
	r, err := result(200, c)
	if err != nil {
		return WriteResult{}, err
	}
	stored, replayed, err := s.store.Commit(persistence.Commit{IdempotencyKey: key, ExpectedVersion: before, Case: c, Action: "PROTOCOL_FROZEN", Payload: map[string]any{"protocol": c.Protocol, "confirmationToken": confirmationToken, "previewSummary": preview}, Result: r, Now: now})
	if err != nil {
		return WriteResult{}, err
	}
	return WriteResult{stored.Status, stored.Body, replayed}, nil
}
