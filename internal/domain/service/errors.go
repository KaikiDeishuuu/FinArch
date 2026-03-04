package service

import "errors"

var (
	ErrInvalidToken     = errors.New("invalid_token")
	ErrExpiredToken     = errors.New("expired_token")
	ErrAlreadyUsed      = errors.New("already_used")
	ErrNotAuthorized    = errors.New("not_authorized")
	ErrResourceConflict = errors.New("resource_conflict")
	ErrUserNotFound     = errors.New("user_not_found")
)

const (
	ActionRegisterVerify = "register"
	ActionEmailChangeOld = "email_change_old"
	ActionEmailChangeNew = "email_change"
	ActionPasswordReset  = "password_reset"
	ActionAccountDelete  = "account_delete"
	ActionBackupExport   = "backup_export"
)
