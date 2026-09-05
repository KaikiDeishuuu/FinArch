package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessTokenIssuer   = "finarch-access"
	accessTokenAudience = "finarch-api"
	accessTokenType     = "access"
	DefaultAccessTTL    = 15 * time.Minute
)

var (
	ErrAccessTokenExpired = errors.New("access token expired")
	ErrAccessTokenInvalid = errors.New("access token invalid")
)

// Claims contains the JWT payload.
type Claims struct {
	UserID     string `json:"uid"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	PwdVersion int    `json:"pv"`
	SessionID  string `json:"sid"`
	TokenType  string `json:"token_type"`
	jwt.RegisteredClaims
}

// JWTService issues and validates JWT tokens.
type JWTService struct {
	secret    []byte
	accessTTL time.Duration
}

// NewJWTService creates a JWTService with the given HMAC secret.
func NewJWTService(secret string) *JWTService {
	return &JWTService{
		secret:    []byte(secret),
		accessTTL: DefaultAccessTTL,
	}
}

// Issue mints a signed access token for the given user.
func (s *JWTService) Issue(userID, email, role string, pwdVersion int, sessionIDs ...string) (string, time.Time, error) {
	if len(sessionIDs) != 1 || sessionIDs[0] == "" {
		return "", time.Time{}, errors.New("session id is required")
	}
	now := time.Now().UTC().Truncate(time.Second)
	exp := now.Add(s.accessTTL)
	claims := &Claims{
		UserID:     userID,
		Email:      email,
		Role:       role,
		PwdVersion: pwdVersion,
		SessionID:  sessionIDs[0],
		TokenType:  accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    accessTokenIssuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings{accessTokenAudience},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

// Verify parses and validates a signed token, returns claims on success.
func (s *JWTService) Verify(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(accessTokenIssuer),
		jwt.WithAudience(accessTokenAudience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		var claims *Claims
		if token != nil {
			claims, _ = token.Claims.(*Claims)
		}
		if isPureExpirationValidationError(err) && validAccessClaims(claims) {
			return nil, fmt.Errorf("%w: %v", ErrAccessTokenExpired, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrAccessTokenInvalid, err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || !validAccessClaims(claims) {
		return nil, ErrAccessTokenInvalid
	}
	return claims, nil
}

func validAccessClaims(claims *Claims) bool {
	return claims != nil && claims.TokenType == accessTokenType && claims.UserID != "" &&
		claims.SessionID != "" && claims.Subject == claims.UserID
}

// isPureExpirationValidationError distinguishes an otherwise-valid expired
// token from a token that is both expired and invalid for another reason (for
// example, a wrong issuer or audience). jwt/v5 joins claim failures beneath an
// ErrTokenInvalidClaims wrapper, so checking errors.Is(ErrTokenExpired) alone
// would incorrectly give composite-invalid tokens the access_expired contract.
func isPureExpirationValidationError(err error) bool {
	expired := false
	invalid := false
	var visit func(error)
	visit = func(current error) {
		if current == nil || invalid {
			return
		}
		switch current {
		case jwt.ErrTokenExpired:
			expired = true
			return
		case jwt.ErrTokenInvalidClaims:
			return
		}
		if joined, ok := current.(interface{ Unwrap() []error }); ok {
			for _, child := range joined.Unwrap() {
				visit(child)
			}
			return
		}
		if child := errors.Unwrap(current); child != nil {
			visit(child)
			return
		}
		invalid = true
	}
	visit(err)
	return expired && !invalid
}
