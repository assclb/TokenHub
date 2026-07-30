package server

import "time"

// newRouteAttemptLog is the persisted form of one routing attempt.
//
// It lives next to nothing else on purpose: the mapping is long, it belongs to the
// RouteAttempt type rather than to the store, and keeping it here means adding a
// field to an attempt touches one function instead of the store's largest file.
func newRouteAttemptLog(requestID string, index int, attempt RouteAttempt, now time.Time) RouteAttemptLog {
	return RouteAttemptLog{
		ID:                 NewID("rat"),
		RequestID:          requestID,
		AttemptIndex:       index + 1,
		RouteID:            attempt.Selection.Route.ID,
		ProviderID:         attempt.Selection.Provider.ID,
		ProviderResourceID: routeResourceID(attempt.Selection),
		ProviderModel:      attempt.Selection.ProviderModel,
		StatusCode:         attempt.Status,
		ErrorCode:          attempt.ErrorCode,
		ErrorMessage:       attempt.Error,
		Invoked:            attempt.Invoked,
		LatencyMS:          attempt.LatencyMS,
		ServedModel:        attempt.Usage.ServedModel,
		UpstreamRequestID:  attempt.Usage.UpstreamRequestID,
		Transport:          attempt.Usage.Transport,
		InputTokens:        attempt.Usage.PromptTokens,
		CachedInputTokens:  attempt.Usage.CachedInputTokens,
		CacheWriteTokens:   attempt.Usage.CacheWriteInputTokens,
		OutputTokens:       attempt.Usage.CompletionTokens,
		ReasoningTokens:    attempt.Usage.ReasoningOutputTokens,
		TotalTokens:        attempt.Usage.TotalTokens,
		CostUSD:            attempt.Usage.CostUSD,
		StartedAt:          attempt.StartedAt,
		EndedAt:            attempt.EndedAt,
		CreatedAt:          now,
	}
}
