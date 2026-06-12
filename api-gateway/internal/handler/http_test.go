package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	redirectpb "gotiny/proto/redirect"
	urlpb "gotiny/proto/url"
	userpb "gotiny/proto/user"

	"gotiny/api-gateway/internal/handler"
)

type stubURLClient struct {
	createResp *urlpb.CreateShortURLResponse
	createErr  error
	listResp   *urlpb.ListURLsResponse
	listErr    error
	deleteResp *urlpb.DeleteURLResponse
	deleteErr  error
	getResp    *urlpb.GetURLResponse
	getErr     error
}

func (s *stubURLClient) CreateShortURL(_ context.Context, _ *urlpb.CreateShortURLRequest, _ ...grpc.CallOption) (*urlpb.CreateShortURLResponse, error) {
	return s.createResp, s.createErr
}
func (s *stubURLClient) GetURL(_ context.Context, _ *urlpb.GetURLRequest, _ ...grpc.CallOption) (*urlpb.GetURLResponse, error) {
	return s.getResp, s.getErr
}
func (s *stubURLClient) ListURLs(_ context.Context, _ *urlpb.ListURLsRequest, _ ...grpc.CallOption) (*urlpb.ListURLsResponse, error) {
	return s.listResp, s.listErr
}
func (s *stubURLClient) DeleteURL(_ context.Context, _ *urlpb.DeleteURLRequest, _ ...grpc.CallOption) (*urlpb.DeleteURLResponse, error) {
	return s.deleteResp, s.deleteErr
}

type stubRedirectClient struct {
	resolveResp      *redirectpb.ResolveResponse
	resolveErr       error
	invalidateResp   *redirectpb.InvalidateCacheResponse
	invalidateErr    error
}

func (s *stubRedirectClient) Resolve(_ context.Context, _ *redirectpb.ResolveRequest, _ ...grpc.CallOption) (*redirectpb.ResolveResponse, error) {
	return s.resolveResp, s.resolveErr
}
func (s *stubRedirectClient) InvalidateCache(_ context.Context, _ *redirectpb.InvalidateCacheRequest, _ ...grpc.CallOption) (*redirectpb.InvalidateCacheResponse, error) {
	return s.invalidateResp, s.invalidateErr
}

type stubUserClient struct {
	registerResp *userpb.AuthResponse
	registerErr  error
	loginResp    *userpb.AuthResponse
	loginErr     error
	refreshResp  *userpb.AuthResponse
	refreshErr   error
	logoutResp   *userpb.LogoutResponse
	logoutErr    error
}

func (s *stubUserClient) Register(_ context.Context, _ *userpb.RegisterRequest, _ ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return s.registerResp, s.registerErr
}
func (s *stubUserClient) Login(_ context.Context, _ *userpb.LoginRequest, _ ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return s.loginResp, s.loginErr
}
func (s *stubUserClient) RefreshToken(_ context.Context, _ *userpb.RefreshRequest, _ ...grpc.CallOption) (*userpb.AuthResponse, error) {
	return s.refreshResp, s.refreshErr
}
func (s *stubUserClient) Logout(_ context.Context, _ *userpb.LogoutRequest, _ ...grpc.CallOption) (*userpb.LogoutResponse, error) {
	return s.logoutResp, s.logoutErr
}

func newHandler(urlC *stubURLClient, redirC *stubRedirectClient, userC *stubUserClient) *handler.HTTPHandler {
	if urlC == nil {
		urlC = &stubURLClient{}
	}
	if redirC == nil {
		redirC = &stubRedirectClient{}
	}
	if userC == nil {
		userC = &stubUserClient{}
	}
	return handler.NewHTTPHandler(urlC, redirC, userC)
}

// --- Shorten tests (now require user_id in context) ---

func TestShortenValidURL(t *testing.T) {
	h := newHandler(
		&stubURLClient{createResp: &urlpb.CreateShortURLResponse{ShortCode: "abc1234"}},
		nil, nil,
	)

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	req = req.WithContext(context.WithValue(req.Context(), handler.ExportedUserIDContextKey, int64(1)))
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["short_url"] != "abc1234" {
		t.Errorf("expected short_url=abc1234, got %q", resp["short_url"])
	}
}

func TestShortenMissingURL(t *testing.T) {
	h := newHandler(nil, nil, nil)

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestShortenInvalidScheme(t *testing.T) {
	h := newHandler(nil, nil, nil)

	body := bytes.NewBufferString(`{"url":"ftp://example.com/file"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for ftp scheme, got %d", w.Code)
	}
}

func TestShortenURLTooLong(t *testing.T) {
	h := newHandler(nil, nil, nil)

	longURL := "https://example.com/" + strings.Repeat("a", 2030)
	body, _ := json.Marshal(map[string]string{"url": longURL})
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewReader(body))
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-long URL, got %d", w.Code)
	}
}

func TestShortenServiceUnavailable(t *testing.T) {
	h := newHandler(
		&stubURLClient{createErr: status.Error(codes.Unavailable, "down")},
		nil, nil,
	)

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	req = req.WithContext(context.WithValue(req.Context(), handler.ExportedUserIDContextKey, int64(1)))
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestShortenPoolExhausted(t *testing.T) {
	h := newHandler(
		&stubURLClient{createErr: status.Error(codes.ResourceExhausted, "pool exhausted")},
		nil, nil,
	)

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	req = req.WithContext(context.WithValue(req.Context(), handler.ExportedUserIDContextKey, int64(1)))
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// --- Redirect tests (unchanged, still public) ---

func TestRedirectSuccess(t *testing.T) {
	h := newHandler(
		nil,
		&stubRedirectClient{resolveResp: &redirectpb.ResolveResponse{OriginalUrl: "https://example.com"}},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/abc1234", nil)
	req.SetPathValue("code", "abc1234")
	w := httptest.NewRecorder()

	h.Redirect(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com" {
		t.Errorf("expected Location=https://example.com, got %q", loc)
	}
}

func TestRedirectNotFound(t *testing.T) {
	h := newHandler(
		nil,
		&stubRedirectClient{resolveErr: status.Error(codes.NotFound, "not found")},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/abc1234", nil)
	req.SetPathValue("code", "abc1234")
	w := httptest.NewRecorder()

	h.Redirect(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRedirectEmptyCode(t *testing.T) {
	h := newHandler(nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("code", "")
	w := httptest.NewRecorder()

	h.Redirect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRedirectInvalidCode(t *testing.T) {
	h := newHandler(nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/../etc/passwd", nil)
	req.SetPathValue("code", "../etc/passwd")
	w := httptest.NewRecorder()

	h.Redirect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid code, got %d", w.Code)
	}
}

func TestShortenEmptyHost(t *testing.T) {
	h := newHandler(nil, nil, nil)

	body := bytes.NewBufferString(`{"url":"https:///path"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty host, got %d", w.Code)
	}
}

func TestShortenInvalidJSON(t *testing.T) {
	h := newHandler(nil, nil, nil)

	body := bytes.NewBufferString(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// --- Auth endpoint tests ---

func TestRegister_Success(t *testing.T) {
	h := newHandler(nil, nil, &stubUserClient{
		registerResp: &userpb.AuthResponse{
			AccessToken: "at", RefreshToken: "rt", ExpiresIn: 900,
		},
	})

	body := bytes.NewBufferString(`{"email":"user@example.com","password":"password123"}`)
	req := httptest.NewRequest(http.MethodPost, "/register", body)
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestRegister_Conflict(t *testing.T) {
	h := newHandler(nil, nil, &stubUserClient{
		registerErr: status.Error(codes.AlreadyExists, "email exists"),
	})

	body := bytes.NewBufferString(`{"email":"taken@example.com","password":"password123"}`)
	req := httptest.NewRequest(http.MethodPost, "/register", body)
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestLogin_Success(t *testing.T) {
	h := newHandler(nil, nil, &stubUserClient{
		loginResp: &userpb.AuthResponse{
			AccessToken: "at", RefreshToken: "rt", ExpiresIn: 900,
		},
	})

	body := bytes.NewBufferString(`{"email":"user@example.com","password":"password123"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	h := newHandler(nil, nil, &stubUserClient{
		loginErr: status.Error(codes.Unauthenticated, "invalid credentials"),
	})

	body := bytes.NewBufferString(`{"email":"user@example.com","password":"wrong"}`)
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDeleteURL_Forbidden(t *testing.T) {
	h := newHandler(
		&stubURLClient{deleteErr: status.Error(codes.PermissionDenied, "not owner")},
		nil, nil,
	)

	req := httptest.NewRequest(http.MethodDelete, "/urls/abc1234", nil)
	req.SetPathValue("code", "abc1234")
	req = req.WithContext(context.WithValue(req.Context(), handler.ExportedUserIDContextKey, int64(99)))
	w := httptest.NewRecorder()

	h.DeleteURL(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestListURLs_EmptyList(t *testing.T) {
	h := newHandler(
		&stubURLClient{listResp: &urlpb.ListURLsResponse{Urls: nil, Total: 0}},
		nil, nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/urls", nil)
	req = req.WithContext(context.WithValue(req.Context(), handler.ExportedUserIDContextKey, int64(1)))
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	urls, ok := resp["urls"].([]any)
	if !ok || len(urls) != 0 {
		t.Errorf("expected empty urls array, got %v", resp["urls"])
	}
}
