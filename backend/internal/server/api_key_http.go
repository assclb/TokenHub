package server

import "time"

type adminAPIKeyUpdateRequest struct {
	Name          string       `json:"name"`
	Group         string       `json:"group"`
	OwnerUserID   *string      `json:"owner_user_id"`
	AllowedModels []string     `json:"allowed_models"`
	IPAllowlist   []string     `json:"ip_allowlist"`
	Limits        *QuotaLimits `json:"limits"`
	Status        string       `json:"status"`
	ExpiresAt     *time.Time   `json:"expires_at"`
}
