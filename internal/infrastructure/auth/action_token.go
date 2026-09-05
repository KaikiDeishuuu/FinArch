package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	actionTokenIssuer = "finarch-action"
	actionTokenType   = "action"
)

var (
	ErrActionTokenExpired = errors.New("expired_token")
)

type ActionClaims struct {
	UserID    string `json:"uid"`
	Action    string `json:"act"`
	Meta      string `json:"meta,omitempty"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

type ActionTokenService struct {
	secret []byte
}

func NewActionTokenService(secret string) *ActionTokenService {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("finarch/action-token/signing-key/v1"))
	return &ActionTokenService{secret: mac.Sum(nil)}
}

func (s *ActionTokenService) Issue(userID, action, meta string, ttl time.Duration) (token, jti string, exp time.Time, err error) {
	now := time.Now()
	exp = now.Add(ttl)
	jti = uuid.NewString()
	claims := &ActionClaims{
		UserID:    userID,
		Action:    action,
		Meta:      meta,
		TokenType: actionTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Audience:  []string{action},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    actionTokenIssuer,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(s.secret)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("sign action token: %w", err)
	}
	return signed, jti, exp, nil
}

func (s *ActionTokenService) Verify(tokenStr, action string) (*ActionClaims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &ActionClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(actionTokenIssuer),
		jwt.WithAudience(action),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		var claims *ActionClaims
		if t != nil {
			claims, _ = t.Claims.(*ActionClaims)
		}
		if isPureExpirationValidationError(err) && validActionClaims(claims, action) {
			return nil, ErrActionTokenExpired
		}
		return nil, fmt.Errorf("invalid_token")
	}
	claims, ok := t.Claims.(*ActionClaims)
	if !ok || !t.Valid || !validActionClaims(claims, action) {
		return nil, fmt.Errorf("invalid_token")
	}
	return claims, nil
}

func validActionClaims(claims *ActionClaims, action string) bool {
	return claims != nil && claims.ID != "" && claims.UserID != "" &&
		claims.Subject == claims.UserID && claims.Action == action &&
		claims.TokenType == actionTokenType
}
