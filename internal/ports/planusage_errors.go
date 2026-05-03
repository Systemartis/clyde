package ports

import "errors"

// ErrPlanUsageUnavailable is returned when no credentials are present or the
// user is on a plan that does not expose plan-quota data. Callers SHOULD hide
// the plan-quota UI rather than treat this as an error worth surfacing.
var ErrPlanUsageUnavailable = errors.New("plan usage unavailable")

// ErrPlanUsageAuth is returned when credentials exist but cannot be used —
// the OAuth refresh token has been revoked or has hit a terminal
// invalid_grant. Callers SHOULD prompt the user to re-authenticate.
var ErrPlanUsageAuth = errors.New("plan usage auth failed")
