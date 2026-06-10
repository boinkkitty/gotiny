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

	"gotiny/api-gateway/internal/handler"
)

type stubURLClient struct {
	resp *urlpb.CreateShortURLResponse
	err  error
}

func (s *stubURLClient) CreateShortURL(_ context.Context, _ *urlpb.CreateShortURLRequest, _ ...grpc.CallOption) (*urlpb.CreateShortURLResponse, error) {
	return s.resp, s.err
}

type stubRedirectClient struct {
	resp *redirectpb.ResolveResponse
	err  error
}

func (s *stubRedirectClient) Resolve(_ context.Context, _ *redirectpb.ResolveRequest, _ ...grpc.CallOption) (*redirectpb.ResolveResponse, error) {
	return s.resp, s.err
}

func TestShortenValidURL(t *testing.T) {
	h := handler.NewHTTPHandler(
		&stubURLClient{resp: &urlpb.CreateShortURLResponse{ShortCode: "abc1234"}},
		&stubRedirectClient{},
	)

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
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
	h := handler.NewHTTPHandler(&stubURLClient{}, &stubRedirectClient{})

	body := bytes.NewBufferString(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestShortenInvalidScheme(t *testing.T) {
	h := handler.NewHTTPHandler(&stubURLClient{}, &stubRedirectClient{})

	body := bytes.NewBufferString(`{"url":"ftp://example.com/file"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for ftp scheme, got %d", w.Code)
	}
}

func TestShortenURLTooLong(t *testing.T) {
	h := handler.NewHTTPHandler(&stubURLClient{}, &stubRedirectClient{})

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
	h := handler.NewHTTPHandler(
		&stubURLClient{err: status.Error(codes.Unavailable, "down")},
		&stubRedirectClient{},
	)

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestShortenPoolExhausted(t *testing.T) {
	h := handler.NewHTTPHandler(
		&stubURLClient{err: status.Error(codes.ResourceExhausted, "pool exhausted")},
		&stubRedirectClient{},
	)

	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestRedirectSuccess(t *testing.T) {
	h := handler.NewHTTPHandler(
		&stubURLClient{},
		&stubRedirectClient{resp: &redirectpb.ResolveResponse{OriginalUrl: "https://example.com"}},
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
	h := handler.NewHTTPHandler(
		&stubURLClient{},
		&stubRedirectClient{err: status.Error(codes.NotFound, "not found")},
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
	h := handler.NewHTTPHandler(&stubURLClient{}, &stubRedirectClient{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("code", "")
	w := httptest.NewRecorder()

	h.Redirect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRedirectInvalidCode(t *testing.T) {
	h := handler.NewHTTPHandler(&stubURLClient{}, &stubRedirectClient{})

	req := httptest.NewRequest(http.MethodGet, "/../etc/passwd", nil)
	req.SetPathValue("code", "../etc/passwd")
	w := httptest.NewRecorder()

	h.Redirect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid code, got %d", w.Code)
	}
}

func TestShortenEmptyHost(t *testing.T) {
	h := handler.NewHTTPHandler(&stubURLClient{}, &stubRedirectClient{})

	body := bytes.NewBufferString(`{"url":"https:///path"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty host, got %d", w.Code)
	}
}

func TestShortenInvalidJSON(t *testing.T) {
	h := handler.NewHTTPHandler(&stubURLClient{}, &stubRedirectClient{})

	body := bytes.NewBufferString(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	w := httptest.NewRecorder()

	h.Shorten(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}
