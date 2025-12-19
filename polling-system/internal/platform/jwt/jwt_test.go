package jwt

import (
	"errors"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

func TestManager_GenerateAndParse(t *testing.T) {
	m := NewManager("secret", "issuer")

	token, err := m.Generate(123, "admin", time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != 123 {
		t.Fatalf("expected user_id 123, got %d", claims.UserID)
	}
	if claims.Role != "admin" {
		t.Fatalf("expected role admin, got %q", claims.Role)
	}
	if claims.Issuer != "issuer" {
		t.Fatalf("expected issuer issuer, got %q", claims.Issuer)
	}
}

func TestNewManager_DefaultIssuer(t *testing.T) {
	m := NewManager("secret", "")

	token, err := m.Generate(1, "user", time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Issuer != "polling-system" {
		t.Fatalf("expected default issuer polling-system, got %q", claims.Issuer)
	}
}

func TestManager_ParseRejectsInvalidIssuer(t *testing.T) {
	m1 := NewManager("secret", "issuer-1")
	token, err := m1.Generate(1, "user", time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	m2 := NewManager("secret", "issuer-2")
	if _, err := m2.Parse(token); !errors.Is(err, jwtlib.ErrTokenInvalidIssuer) {
		t.Fatalf("expected invalid issuer error, got %v", err)
	}
}

func TestManager_ParseRejectsExpiredToken(t *testing.T) {
	m := NewManager("secret", "issuer")
	token, err := m.Generate(1, "user", -time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := m.Parse(token); !errors.Is(err, jwtlib.ErrTokenExpired) {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestManager_ParseRejectsWrongSecret(t *testing.T) {
	token, err := NewManager("secret-1", "issuer").Generate(1, "user", time.Hour)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := NewManager("secret-2", "issuer").Parse(token); err == nil {
		t.Fatalf("expected error")
	}
}

func TestManager_ParseRejectsUnexpectedSigningMethod(t *testing.T) {
	m := NewManager("secret", "issuer")
	claims := Claims{
		UserID: 1,
		Role:   "user",
		RegisteredClaims: jwtlib.RegisteredClaims{
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwtlib.NewNumericDate(time.Now()),
			Issuer:    "issuer",
		},
	}

	tokenStr, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS512, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("signed: %v", err)
	}

	if _, err := m.Parse(tokenStr); err == nil || !strings.Contains(err.Error(), "unexpected signing method") {
		t.Fatalf("expected unexpected signing method error, got %v", err)
	}
}

