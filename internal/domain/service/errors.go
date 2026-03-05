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
	ErrSystemUnavailable      = &DomainError{Code: "system_unavailable"}
	ErrInvalidPassword        = &DomainError{Code: "invalid_password"}

	ErrEmailNotVerified = errors.New("email_not_verified")
)

const (
	ActionRegisterVerify = "register"
	ActionEmailChangeOld = "email_change_old"
	ActionEmailChangeNew = "email_change"
	ActionPasswordReset  = "password_reset"
	ActionAccountDelete  = "account_delete"
	ActionBackupExport   = "backup_export"
)
