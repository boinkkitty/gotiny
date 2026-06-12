package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"gotiny/api-gateway/internal/handler"
)

func issueTestToken(secret string, userID float64, expired bool) string {
	now := time.Now()
	exp := now.Add(15 * time.Minute)
	if expired {
		exp = now.Add(-1 * time.Minute)
	}
	claims := jwt.MapClaims{
		"sub": userID,
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret"
	token := issueTestToken(secret, 42, false)

	var capturedUserID int64
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID, _ = r.Context().Value(handler.ExportedUserIDContextKey).(int64)
		w.WriteHeader(http.StatusOK)
	})

	mw := handler.JWTMiddleware([]byte(secret))(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if capturedUserID != 42 {
		t.Errorf("expected user_id=42, got %d", capturedUserID)
	}
}

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	mw := handler.JWTMiddleware([]byte("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_MalformedHeader(t *testing.T) {
	mw := handler.JWTMiddleware([]byte("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "NotBearer token")
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	token := issueTestToken(secret, 42, true)

	mw := handler.JWTMiddleware([]byte(secret))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_WrongSecret(t *testing.T) {
	token := issueTestToken("correct-secret", 42, false)

	mw := handler.JWTMiddleware([]byte("wrong-secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for wrong secret")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
