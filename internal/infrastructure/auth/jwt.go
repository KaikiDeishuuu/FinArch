package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims contains the JWT payload.
type Claims struct {
	UserID     string `json:"uid"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	PwdVersion int    `json:"pv"`
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
		accessTTL: 15 * time.Minute,
	}
}

// Issue mints a signed access token for the given user.
func (s *JWTService) Issue(userID, email, role string, pwdVersion int) (string, time.Time, error) {
	exp := time.Now().Add(s.accessTTL)
	claims := &Claims{
		UserID:     userID,
		Email:      email,
		Role:       role,
		PwdVersion: pwdVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
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
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
