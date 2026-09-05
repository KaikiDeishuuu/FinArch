package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAccessAndActionTokensCannotBeConfused(t *testing.T) {
	const baseSecret = "shared-base-secret-used-by-constructor"
	access := NewJWTService(baseSecret)
	actions := NewActionTokenService(baseSecret)

	accessToken, _, err := access.Issue("user-1", "user@example.com", "owner", 0, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	actionToken, _, _, err := actions.Issue("user-1", "reset_password", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := access.Verify(accessToken); err != nil {
		t.Fatalf("valid access token rejected: %v", err)
	}
	if _, err := actions.Verify(actionToken, "reset_password"); err != nil {
		t.Fatalf("valid action token rejected: %v", err)
	}
	if _, err := access.Verify(actionToken); err == nil {
		t.Fatal("action token was accepted as an access token")
	}
	if _, err := actions.Verify(accessToken, "reset_password"); err == nil {
		t.Fatal("access token was accepted as an action token")
	}
}

func TestAccessTokenRejectsNonHS256Algorithm(t *testing.T) {
	const secret = "access-secret"
	now := time.Now()
	claims := &Claims{
		UserID:    "user-1",
		SessionID: "session-1",
		TokenType: accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    accessTokenIssuer,
			Subject:   "user-1",
			Audience:  jwt.ClaimStrings{accessTokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewJWTService(secret).Verify(token); err == nil {
		t.Fatal("HS384 token was accepted by HS256-only verifier")
	}
}

func TestAccessTokenRequiresPurposeIssuerAndAudience(t *testing.T) {
	const secret = "access-secret"
	now := time.Now()
	claims := &Claims{
		UserID:    "user-1",
		SessionID: "session-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewJWTService(secret).Verify(token); err == nil {
		t.Fatal("token without access purpose, issuer, and audience was accepted")
	}
}

func TestAccessTokenRequiresSessionAndExpiresInFifteenMinutes(t *testing.T) {
	service := NewJWTService("access-secret")
	if _, _, err := service.Issue("user-1", "user@example.com", "owner", 3); err == nil {
		t.Fatal("access token was issued without a session id")
	}
	token, expiresAt, err := service.Issue("user-1", "user@example.com", "owner", 3, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SessionID != "session-1" || claims.PwdVersion != 3 {
		t.Fatalf("unexpected session claims: %#v", claims)
	}
	if delta := expiresAt.Sub(claims.IssuedAt.Time); delta != DefaultAccessTTL {
		t.Fatalf("access TTL = %s, want %s", delta, DefaultAccessTTL)
	}
}

func TestAccessTokenRejectsMissingSessionClaim(t *testing.T) {
	const secret = "access-secret"
	now := time.Now()
	claims := &Claims{
		UserID: "user-1", TokenType: accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: accessTokenIssuer, Subject: "user-1",
			Audience: jwt.ClaimStrings{accessTokenAudience}, IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewJWTService(secret).Verify(token); err == nil {
		t.Fatal("access token without sid was accepted")
	} else if !errors.Is(err, ErrAccessTokenInvalid) {
		t.Fatalf("missing sid error = %v, want ErrAccessTokenInvalid", err)
	}
}

func TestAccessTokenReportsExpirationSeparately(t *testing.T) {
	const secret = "access-secret"
	now := time.Now().Add(-2 * time.Minute)
	claims := &Claims{
		UserID: "user-1", SessionID: "session-1", TokenType: accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: accessTokenIssuer, Subject: "user-1",
			Audience: jwt.ClaimStrings{accessTokenAudience}, IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewJWTService(secret).Verify(token); !errors.Is(err, ErrAccessTokenExpired) {
		t.Fatalf("expired token error = %v, want ErrAccessTokenExpired", err)
	}
}

func TestAccessTokenCompositeClaimFailureIsInvalidNotExpired(t *testing.T) {
	const secret = "access-secret"
	now := time.Now().Add(-2 * time.Minute)
	for _, tc := range []struct {
		name     string
		issuer   string
		audience jwt.ClaimStrings
	}{
		{name: "wrong issuer", issuer: "attacker", audience: jwt.ClaimStrings{accessTokenAudience}},
		{name: "wrong audience", issuer: accessTokenIssuer, audience: jwt.ClaimStrings{"other-api"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := &Claims{
				UserID: "user-1", SessionID: "session-1", TokenType: accessTokenType,
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer: tc.issuer, Subject: "user-1", Audience: tc.audience,
					IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
				},
			}
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			_, err = NewJWTService(secret).Verify(token)
			if !errors.Is(err, ErrAccessTokenInvalid) || errors.Is(err, ErrAccessTokenExpired) {
				t.Fatalf("composite claim error = %v, want invalid and not expired", err)
			}
		})
	}
}

func TestExpiredAccessTokenWithInvalidRequiredClaimIsInvalid(t *testing.T) {
	const secret = "access-secret"
	for _, tc := range []struct {
		name   string
		mutate func(*Claims)
	}{
		{name: "missing session", mutate: func(claims *Claims) { claims.SessionID = "" }},
		{name: "wrong purpose", mutate: func(claims *Claims) { claims.TokenType = "action" }},
		{name: "subject mismatch", mutate: func(claims *Claims) { claims.Subject = "other-user" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().Add(-2 * time.Minute)
			claims := &Claims{
				UserID: "user-1", SessionID: "session-1", TokenType: accessTokenType,
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer: accessTokenIssuer, Subject: "user-1",
					Audience: jwt.ClaimStrings{accessTokenAudience}, IssuedAt: jwt.NewNumericDate(now),
					ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
				},
			}
			tc.mutate(claims)
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			_, err = NewJWTService(secret).Verify(token)
			if !errors.Is(err, ErrAccessTokenInvalid) || errors.Is(err, ErrAccessTokenExpired) {
				t.Fatalf("expired token with invalid required claim = %v, want invalid and not expired", err)
			}
		})
	}
}
