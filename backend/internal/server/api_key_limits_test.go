package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAdminCanClearAllAPIKeyLimits(t *testing.T) {
	store := NewMemoryStore()
	if err := BootstrapBaseData(store); err != nil {
		t.Fatal(err)
	}
	_, _, err := store.CreateAPIKey(defaultProjectID, APIKey{
		ID:   "key_clear_limits",
		Name: "Clear Limits",
		Limits: QuotaLimits{
			DailyRequests:   1000,
			MonthlyRequests: 30000,
			DailyTokens:     1000000,
			MonthlyTokens:   20000000,
			DailyCostUSD:    100,
			MonthlyCostUSD:  2000,
			MaxConcurrency:  20,
		},
		Status: StatusActive,
	}, "thk_clear_limits")
	if err != nil {
		t.Fatal(err)
	}
	app := New(store).Handler()

	resp := doJSON(t, app, http.MethodPatch, "/api/admin/api-keys/key_clear_limits", map[string]any{
		"name": "Renamed Key",
	}, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	var updated APIKey
	if err := json.Unmarshal([]byte(resp.Body), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Limits.DailyRequests != 1000 {
		t.Fatalf("expected omitted limits to be preserved, got %+v", updated.Limits)
	}

	resp = doJSON(t, app, http.MethodPatch, "/api/admin/api-keys/key_clear_limits", map[string]any{
		"limits": QuotaLimits{},
	}, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body)
	}
	if err := json.Unmarshal([]byte(resp.Body), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Limits != (QuotaLimits{}) {
		t.Fatalf("expected all limits to be cleared, got %+v", updated.Limits)
	}
}

func TestAdminCanClearAllAPIKeyLimitsAfterApproval(t *testing.T) {
	store := NewMemoryStore()
	if err := BootstrapBaseData(store); err != nil {
		t.Fatal(err)
	}
	key, _, err := store.CreateAPIKey(defaultProjectID, APIKey{
		Name:   "Clear Limits After Approval",
		Limits: QuotaLimits{DailyRequests: 1000},
		Status: StatusActive,
	}, "thk_clear_limits_after_approval")
	if err != nil {
		t.Fatal(err)
	}
	store.CreateResource("approval-flows", AdminResource{
		Name:   "Quota Increase",
		Status: StatusActive,
		Fields: map[string]any{"trigger": "quota_increase", "approver_role": "admin"},
	})
	app := New(store).Handler()

	resp := doJSON(t, app, http.MethodPatch, "/api/admin/api-keys/"+key.ID, map[string]any{
		"limits": QuotaLimits{},
	}, "")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected approval response, got %d: %s", resp.Code, resp.Body)
	}
	approvals := store.ListApprovalRequests()
	if len(approvals) != 1 || approvals[0].Trigger != "quota_increase" {
		t.Fatalf("expected quota increase approval, got %+v", approvals)
	}
	keys := store.ListProjectKeys(defaultProjectID)
	if len(keys) != 1 || keys[0].Limits.DailyRequests != 1000 {
		t.Fatalf("expected limits to remain unchanged before approval, got %+v", keys)
	}

	resp = doJSON(t, app, http.MethodPost, "/api/admin/approvals/"+approvals[0].ID+"/approve", map[string]any{}, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected approval to apply, got %d: %s", resp.Code, resp.Body)
	}
	keys = store.ListProjectKeys(defaultProjectID)
	if len(keys) != 1 || keys[0].Limits != (QuotaLimits{}) {
		t.Fatalf("expected approved limits to be cleared, got %+v", keys)
	}
}
