package repository

import "errors"

// ErrMultiCurrencyReportingUnavailable is returned when an endpoint whose
// response has no currency dimension would otherwise add unlike base units.
var ErrMultiCurrencyReportingUnavailable = errors.New("MULTI_CURRENCY_REPORTING_UNAVAILABLE")

var (
	// ErrConcurrentModification reports that a conditional write no longer
	// matched the state read earlier in the surrounding transaction.
	ErrConcurrentModification    = errors.New("concurrent modification")
	ErrUserNotFound              = errors.New("user not found")
	ErrTransactionNotFound       = errors.New("transaction not found")
	ErrRecurringInstanceNotFound = errors.New("recurring instance not found")
	ErrRefreshTokenInvalid       = errors.New("refresh token invalid")
	ErrRefreshTokenReuse         = errors.New("refresh token reuse")
	ErrSessionInvalid            = errors.New("session invalid")
)
