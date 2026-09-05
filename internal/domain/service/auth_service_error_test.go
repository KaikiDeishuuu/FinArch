package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"finarch/internal/domain/model"
	"finarch/internal/domain/repository"
	"finarch/internal/infrastructure/auth"
)

type userLookupErrorRepository struct {
	repository.UserRepository
	err error
}

func (r userLookupErrorRepository) GetByEmail(context.Context, string) (model.User, error) {
	return model.User{}, r.err
}

func (r userLookupErrorRepository) GetByID(context.Context, string) (model.User, error) {
	return model.User{}, r.err
}

func TestLoginStoreFailureDoesNotAdvanceLockout(t *testing.T) {
	tracker := auth.NewLoginAttemptTracker(1, time.Hour)
	svc := &AuthService{
		users:   userLookupErrorRepository{err: errors.New("database is closed")},
		tracker: tracker,
	}

	for i := 0; i < 2; i++ {
		if _, err := svc.Login(context.Background(), "user@example.com", "Password123"); !errors.Is(err, ErrSystemUnavailable) {
			t.Fatalf("login outage attempt %d error = %v, want ErrSystemUnavailable", i+1, err)
		}
	}
	if tracker.IsLocked("user@example.com") {
		t.Fatal("repository outage advanced the invalid-credential lockout counter")
	}
}

func TestLoginNotFoundRemainsInvalidCredentials(t *testing.T) {
	tracker := auth.NewLoginAttemptTracker(1, time.Hour)
	svc := &AuthService{
		users:   userLookupErrorRepository{err: fmt.Errorf("lookup: %w", repository.ErrUserNotFound)},
		tracker: tracker,
	}

	if _, err := svc.Login(context.Background(), "missing@example.com", "Password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("missing-user login error = %v, want ErrInvalidCredentials", err)
	}
	if !tracker.IsLocked("missing@example.com") {
		t.Fatal("real invalid credentials did not advance the lockout counter")
	}
}

func TestProfileAndPasswordChangeMapLookupErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{name: "not found", err: repository.ErrUserNotFound, want: ErrUserNotFound},
		{name: "store unavailable", err: errors.New("database is closed"), want: ErrSystemUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &AuthService{users: userLookupErrorRepository{err: tc.err}}
			if _, err := svc.GetUserProfile(context.Background(), "user-1"); !errors.Is(err, tc.want) {
				t.Fatalf("GetUserProfile error = %v, want %v", err, tc.want)
			}
			if err := svc.ChangePassword(context.Background(), "user-1", "Password123", "Replacement123"); !errors.Is(err, tc.want) {
				t.Fatalf("ChangePassword error = %v, want %v", err, tc.want)
			}
		})
	}
}
