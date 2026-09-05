package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestActionTokenReportsPureExpirationSeparately(t *testing.T) {
	service := NewActionTokenService("action-secret")
	token := signTestActionToken(t, service, &ActionClaims{
		UserID: "user-1", Action: "password_reset", TokenType: actionTokenType,
		RegisteredClaims: expiredTestActionRegisteredClaims(
			"jti-1", "user-1", actionTokenIssuer, jwt.ClaimStrings{"password_reset"},
		),
	})
	if _, err := service.Verify(token, "password_reset"); !errors.Is(err, ErrActionTokenExpired) {
		t.Fatalf("expired action token error = %v, want ErrActionTokenExpired", err)
	}
}

func TestActionTokenCompositeClaimFailureIsInvalidNotExpired(t *testing.T) {
	service := NewActionTokenService("action-secret")
	for _, tc := range []struct {
		name     string
		issuer   string
		audience jwt.ClaimStrings
	}{
		{name: "wrong issuer", issuer: "attacker", audience: jwt.ClaimStrings{"password_reset"}},
		{name: "wrong audience", issuer: actionTokenIssuer, audience: jwt.ClaimStrings{"other-action"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := signTestActionToken(t, service, &ActionClaims{
				UserID: "user-1", Action: "password_reset", TokenType: actionTokenType,
				RegisteredClaims: expiredTestActionRegisteredClaims(
					"jti-1", "user-1", tc.issuer, tc.audience,
				),
			})
			_, err := service.Verify(token, "password_reset")
			if err == nil || errors.Is(err, ErrActionTokenExpired) {
				t.Fatalf("composite action claim error = %v, want invalid and not expired", err)
			}
		})
	}
}

func TestExpiredActionTokenWithInvalidRequiredClaimIsInvalid(t *testing.T) {
	service := NewActionTokenService("action-secret")
	for _, tc := range []struct {
		name   string
		mutate func(*ActionClaims)
	}{
		{name: "missing jti", mutate: func(claims *ActionClaims) { claims.ID = "" }},
		{name: "missing user", mutate: func(claims *ActionClaims) { claims.UserID = "" }},
		{name: "subject mismatch", mutate: func(claims *ActionClaims) { claims.Subject = "other-user" }},
		{name: "wrong action", mutate: func(claims *ActionClaims) { claims.Action = "email_change" }},
		{name: "wrong purpose", mutate: func(claims *ActionClaims) { claims.TokenType = "access" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := &ActionClaims{
				UserID: "user-1", Action: "password_reset", TokenType: actionTokenType,
				RegisteredClaims: expiredTestActionRegisteredClaims(
					"jti-1", "user-1", actionTokenIssuer, jwt.ClaimStrings{"password_reset"},
				),
			}
			tc.mutate(claims)
			token := signTestActionToken(t, service, claims)
			_, err := service.Verify(token, "password_reset")
			if err == nil || errors.Is(err, ErrActionTokenExpired) {
				t.Fatalf("expired token with invalid required claim = %v, want invalid and not expired", err)
			}
		})
	}
}

func expiredTestActionRegisteredClaims(jti, subject, issuer string, audience jwt.ClaimStrings) jwt.RegisteredClaims {
	now := time.Now().Add(-2 * time.Minute)
	return jwt.RegisteredClaims{
		ID: jti, Issuer: issuer, Subject: subject, Audience: audience,
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
	}
}

func signTestActionToken(t *testing.T, service *ActionTokenService, claims *ActionClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(service.secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
