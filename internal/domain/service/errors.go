package service

import "errors"

type DomainError struct {
	Code string
}

func (e *DomainError) Error() string { return e.Code }

var (
	ErrUsernameTaken          = &DomainError{Code: "username_taken"}
	ErrEmailTaken             = &DomainError{Code: "email_taken"}
	ErrInvalidToken           = &DomainError{Code: "invalid_token"}
	ErrExpiredToken           = &DomainError{Code: "expired_token"}
	ErrAlreadyUsed            = &DomainError{Code: "already_used"}
	ErrNotAuthorized          = &DomainError{Code: "not_authorized"}
	ErrResourceConflict       = &DomainError{Code: "resource_conflict"}
	ErrUserNotFound           = &DomainError{Code: "user_not_found"}
	ErrInternal               = &DomainError{Code: "internal_error"}
	ErrConcurrentModification = &DomainError{Code: "concurrent_modification"}
	ErrInvalidOrUsedToken     = &DomainError{Code: "invalid_or_used_token"}
	ErrRefreshTokenReuse      = &DomainError{Code: "refresh_token_reuse"}
	ErrSessionInvalid         = &DomainError{Code: "session_invalid"}
	ErrSystemUnavailable      = &DomainError{Code: "system_unavailable"}
	ErrInvalidPassword        = &DomainError{Code: "invalid_password"}
	ErrInvalidCredentials     = &DomainError{Code: "invalid_credentials"}
	ErrAccountLocked          = &DomainError{Code: "account_locked"}
	ErrLoginFailed            = &DomainError{Code: "login_failed"}

	ErrEmailNotVerified             = errors.New("email_not_verified")
	ErrResolvedRateEvidenceMismatch = errors.New("resolved_rate_evidence_mismatch")
)

const (
	ActionRegisterVerify   = "register"
	ActionEmailChangeOld   = "email_change_old"
	ActionEmailChangeNew   = "email_change"
	ActionPasswordReset    = "password_reset"
	ActionAccountDelete    = "account_delete"
	ActionBackupExport     = "backup_export"
	ActionDisasterRecovery = "disaster_recovery"
)
