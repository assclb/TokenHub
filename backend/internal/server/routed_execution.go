package server

// Routed execution: the failover loop that turns a list of route candidates into one
// upstream call. Kept apart from the HTTP handlers because it is protocol agnostic —
// chat, responses, embeddings and the Anthropic Messages gateway all run through it.

import (
	"context"
	"errors"
	"time"
)

func executeRoutedWithStore[T any](
	ctx context.Context,
	store Store,
	routed RoutedCall,
	allowReasoningEffortFallback bool,
	// call receives the 1-based attempt number, counted across every candidate
	// including ones that never ran because capacity acquisition failed. Callbacks
	// must not derive it locally: those failures are appended to attempts here
	// without invoking the callback, so a local counter would undercount them.
	call func(context.Context, RouteSelection, bool, int) (T, Usage, error),
) (T, RouteSelection, Usage, []RouteAttempt, error) {
	var zero T
	var lastErr error = ErrProviderMissing
	var affinityBindings map[string]AdapterSessionBinding
	var err error
	routed, affinityBindings, err = applyAdapterSessionAffinity(ctx, store, routed)
	if err != nil {
		return zero, RouteSelection{}, Usage{}, nil, err
	}
	attempts := make([]RouteAttempt, 0, len(routed.Routes)+1)
	for _, route := range routed.Routes {
		if leaseErr := coordinationLeaseError(ctx); leaseErr != nil {
			return zero, route, Usage{}, attempts, leaseErr
		}
		resourceID := routeResourceID(route)
		binding, hasBinding := affinityBindings[route.Provider.ID]
		routeIsBound := hasBinding && binding.ResourceID == resourceID
		leaseID, leaseCtx, err := store.CheckProviderResourceCapacity(ctx, resourceID)
		if err != nil {
			status, code := statusAndCode(err)
			attempts = append(attempts, RouteAttempt{
				Selection: route,
				Status:    status,
				ErrorCode: code,
				Error:     errorMessage(err),
			})
			lastErr = err
			if !shouldFailoverRoutedError(err, routeIsBound) {
				return zero, route, Usage{}, attempts, err
			}
			continue
		}
		omitReasoningEffort := false
		for {
			attemptStartedAt := time.Now()
			resp, usage, err := call(leaseCtx, route, omitReasoningEffort, len(attempts)+1)
			attemptEndedAt := time.Now()
			latencyMS := maxInt64(1, attemptEndedAt.Sub(attemptStartedAt).Milliseconds())
			if leaseErr := coordinationLeaseError(leaseCtx); leaseErr != nil {
				err = leaseErr
			}
			// Neither a committed stream nor a client disconnect may be retried,
			// not even via the effort fallback on the same route. These checks are
			// load-bearing: ProviderInvocationError implements Unwrap, so
			// isReasoningEffortRejection sees through the wrapper and would
			// otherwise return true for an error that must not be retried.
			disposition := providerErrorDisposition(err)
			retryWithoutEffort := allowReasoningEffortFallback &&
				!omitReasoningEffort &&
				disposition != ProviderErrorStreamCommitted &&
				disposition != ProviderErrorClient &&
				isReasoningEffortRejection(err)
			if !retryWithoutEffort {
				finishProviderResourceAttempt(leaseCtx, store, resourceID, leaseID, err, usage)
			}
			status, code := routeAttemptStatusAndCode(err, retryWithoutEffort)
			attempts = append(attempts, RouteAttempt{
				Selection: route,
				Status:    status,
				ErrorCode: code,
				Error:     errorMessage(err),
				Invoked:   true,
				LatencyMS: latencyMS,
				// Priced per attempt so a failover reports what each candidate cost
				// rather than attributing the whole request to the winner. Tokens
				// burned by an attempt that later failed were still billed.
				Usage:     priceUsage(routed.Call.Model, usage),
				StartedAt: attemptStartedAt.UTC(),
				EndedAt:   attemptEndedAt.UTC(),
			})
			if err == nil {
				rebindReason := ""
				if binding, ok := affinityBindings[route.Provider.ID]; ok && binding.ResourceID != resourceID {
					rebindReason = "resource_failover"
				}
				if bindErr := commitAdapterSessionAffinity(ctx, store, routed, affinityBindings, route, rebindReason); bindErr != nil {
					return zero, route, usage, attempts, bindErr
				}
				return resp, route, usage, attempts, nil
			}
			lastErr = err
			if retryWithoutEffort {
				if retryErr := store.CheckProviderResourceRetryCapacity(leaseCtx, resourceID, leaseID); retryErr != nil {
					store.ReleaseProviderResourceCapacity(resourceID, leaseID)
					status, code = statusAndCode(retryErr)
					attempts = append(attempts, RouteAttempt{
						Selection: route,
						Status:    status,
						ErrorCode: code,
						Error:     errorMessage(retryErr),
					})
					lastErr = retryErr
					if !shouldFailoverRoutedError(retryErr, routeIsBound) {
						return zero, route, Usage{}, attempts, retryErr
					}
					break
				}
				omitReasoningEffort = true
				continue
			}
			if !shouldFailoverRoutedError(err, routeIsBound) {
				return zero, route, usage, attempts, err
			}
			break
		}
	}
	return zero, RouteSelection{}, Usage{}, attempts, lastErr
}

func routeAttemptStatusAndCode(err error, reasoningEffortRejected bool) (int, string) {
	if !reasoningEffortRejected {
		return statusAndCode(err)
	}
	httpErr := AsHTTPError(err)
	return httpErr.UpstreamStatus, "reasoning_effort_rejected"
}

func coordinationLeaseError(ctx context.Context) error {
	if ctx != nil && errors.Is(context.Cause(ctx), ErrCoordinationLeaseLost) {
		return ErrCoordinationLeaseLost
	}
	return nil
}

func finishProviderResourceAttempt(ctx context.Context, store Store, resourceID string, leaseID string, err error, usage Usage) {
	if resourceID == "" {
		return
	}
	if errors.Is(err, ErrCoordinationLeaseLost) {
		store.ReleaseProviderResourceCapacity(resourceID, leaseID)
		return
	}
	store.FinishProviderResourceAttempt(ctx, resourceID, leaseID, providerAttemptOutcome(err), usage)
}
