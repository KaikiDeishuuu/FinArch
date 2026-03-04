package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrActionTokenExpired = errors.New("expired_token")
)

type ActionClaims struct {
	UserID string `json:"uid"`
	Action string `json:"act"`
	Meta   string `json:"meta,omitempty"`
	jwt.RegisteredClaims
}

type ActionTokenService struct {
	secret []byte
}

func NewActionTokenService(secret string) *ActionTokenService {
	return &ActionTokenService{secret: []byte(secret)}
}

func (s *ActionTokenService) Issue(userID, action, meta string, ttl time.Duration) (token, jti string, exp time.Time, err error) {
	now := time.Now()
	exp = now.Add(ttl)
	jti = uuid.NewString()
	claims := &ActionClaims{
		UserID: userID,
		Action: action,
		Meta:   meta,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Audience:  []string{action},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
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
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithAudience(action))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrActionTokenExpired
		}
		return nil, fmt.Errorf("invalid_token")
	}
	claims, ok := t.Claims.(*ActionClaims)
	if !ok || !t.Valid || claims.ID == "" || claims.UserID == "" || claims.Action != action {
		return nil, fmt.Errorf("invalid_token")
	}
	return claims, nil
}
