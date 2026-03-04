package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrDeletionTokenExpired = errors.New("deletion token expired")
	ErrTokenAlreadyUsed     = errors.New("token already used")
)

type DeletionClaims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

type DeletionTokenService struct {
	secret []byte
	ttl    time.Duration
}

func NewDeletionTokenService(secret string, ttl time.Duration) *DeletionTokenService {
	return &DeletionTokenService{secret: []byte(secret), ttl: ttl}
}

func (s *DeletionTokenService) Issue(userID string) (token string, jti string, exp time.Time, err error) {
	now := time.Now()
	exp = now.Add(s.ttl)
	jti = uuid.NewString()
	claims := &DeletionClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Audience:  []string{"account_delete"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString(s.secret)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("sign deletion token: %w", err)
	}
	return signed, jti, exp, nil
}

func (s *DeletionTokenService) Verify(tokenStr string) (*DeletionClaims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &DeletionClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithAudience("account_delete"))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrDeletionTokenExpired
		}
		return nil, fmt.Errorf("parse deletion token: %w", err)
	}
	claims, ok := t.Claims.(*DeletionClaims)
	if !ok || !t.Valid || claims.ID == "" || claims.UserID == "" {
		return nil, errors.New("invalid deletion token")
	}
	return claims, nil
}
