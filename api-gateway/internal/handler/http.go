package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	redirectpb "gotiny/proto/redirect"
	urlpb "gotiny/proto/url"
)

type HTTPHandler struct {
	urlClient      urlpb.URLServiceClient
	redirectClient redirectpb.RedirectServiceClient
}

func NewHTTPHandler(urlClient urlpb.URLServiceClient, redirectClient redirectpb.RedirectServiceClient) *HTTPHandler {
	return &HTTPHandler{
		urlClient:      urlClient,
		redirectClient: redirectClient,
	}
}

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	ShortURL string `json:"short_url"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

const maxRequestBodySize = 1 << 20 // 1 MB
const maxURLLength = 2048

var shortCodePattern = regexp.MustCompile(`^[0-9A-Za-z]{1,7}$`)

func (h *HTTPHandler) Shorten(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "missing_url", "url field is required")
		return
	}

	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		writeError(w, http.StatusBadRequest, "invalid_url", "url must be a valid http or https URL")
		return
	}

	if len(req.URL) > maxURLLength {
		writeError(w, http.StatusBadRequest, "url_too_long", "url must be 2048 characters or fewer")
		return
	}

	resp, err := h.urlClient.CreateShortURL(r.Context(), &urlpb.CreateShortURLRequest{
		OriginalUrl: req.URL,
	})
	if err != nil {
		handleGRPCError(w, err, "create short url")
		return
	}

	writeJSON(w, http.StatusCreated, shortenResponse{
		ShortURL: resp.ShortCode,
	})
}

func (h *HTTPHandler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "short code is required")
		return
	}
	if !shortCodePattern.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid_code", "short code must be 1-7 alphanumeric characters")
		return
	}

	resp, err := h.redirectClient.Resolve(r.Context(), &redirectpb.ResolveRequest{
		ShortCode: code,
	})
	if err != nil {
		handleGRPCError(w, err, "resolve short code")
		return
	}

	http.Redirect(w, r, resp.OriginalUrl, http.StatusFound)
}

func handleGRPCError(w http.ResponseWriter, err error, operation string) {
	st, ok := status.FromError(err)
	if !ok {
		slog.Error("grpc call failed", "error", err, "operation", operation)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	switch st.Code() {
	case codes.NotFound:
		writeError(w, http.StatusNotFound, "not_found", st.Message())
	case codes.InvalidArgument:
		writeError(w, http.StatusBadRequest, "invalid_argument", st.Message())
	case codes.ResourceExhausted:
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service temporarily unavailable, try again shortly")
	case codes.Unavailable:
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "service temporarily unavailable")
	default:
		slog.Error("grpc call failed", "code", st.Code(), "message", st.Message(), "operation", operation)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: code, Message: message})
}
