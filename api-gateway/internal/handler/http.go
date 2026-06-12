package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gotiny/pkg/grpcutil"
	redirectpb "gotiny/proto/redirect"
	urlpb "gotiny/proto/url"
	userpb "gotiny/proto/user"
)

type HTTPHandler struct {
	urlClient      urlpb.URLServiceClient
	redirectClient redirectpb.RedirectServiceClient
	userClient     userpb.UserServiceClient
}

func NewHTTPHandler(
	urlClient urlpb.URLServiceClient,
	redirectClient redirectpb.RedirectServiceClient,
	userClient userpb.UserServiceClient,
) *HTTPHandler {
	return &HTTPHandler{
		urlClient:      urlClient,
		redirectClient: redirectClient,
		userClient:     userClient,
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

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type listURLsResponse struct {
	URLs  []urlItem `json:"urls"`
	Total int32     `json:"total"`
}

type urlItem struct {
	ShortCode   string `json:"short_code"`
	OriginalURL string `json:"original_url"`
	CreatedAt   string `json:"created_at"`
}

const maxRequestBodySize = 1 << 20 // 1 MB
const maxURLLength = 2048

var shortCodePattern = regexp.MustCompile(`^[0-9A-Za-z]{1,7}$`)

func (h *HTTPHandler) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	resp, err := h.userClient.Register(r.Context(), &userpb.RegisterRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		handleGRPCError(w, err, "register")
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	resp, err := h.userClient.Login(r.Context(), &userpb.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		handleGRPCError(w, err, "login")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

func (h *HTTPHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	resp, err := h.userClient.RefreshToken(r.Context(), &userpb.RefreshRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		handleGRPCError(w, err, "refresh")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	})
}

func (h *HTTPHandler) Logout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	_, err := h.userClient.Logout(r.Context(), &userpb.LogoutRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		handleGRPCError(w, err, "logout")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

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

	userID := userIDFromContext(r.Context())
	ctx := grpcutil.ContextWithUserID(r.Context(), userID)

	resp, err := h.urlClient.CreateShortURL(ctx, &urlpb.CreateShortURLRequest{
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

func (h *HTTPHandler) ListURLs(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	limit := int32(20)
	offset := int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			offset = int32(n)
		}
	}

	resp, err := h.urlClient.ListURLs(r.Context(), &urlpb.ListURLsRequest{
		UserId: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		handleGRPCError(w, err, "list urls")
		return
	}

	urls := make([]urlItem, 0, len(resp.Urls))
	for _, u := range resp.Urls {
		urls = append(urls, urlItem{
			ShortCode:   u.ShortCode,
			OriginalURL: u.OriginalUrl,
			CreatedAt:   u.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, listURLsResponse{
		URLs:  urls,
		Total: resp.Total,
	})
}

func (h *HTTPHandler) DeleteURL(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" || !shortCodePattern.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid_code", "valid short code is required")
		return
	}

	userID := userIDFromContext(r.Context())

	_, err := h.urlClient.DeleteURL(r.Context(), &urlpb.DeleteURLRequest{
		ShortCode: code,
		UserId:    userID,
	})
	if err != nil {
		handleGRPCError(w, err, "delete url")
		return
	}

	if _, err := h.redirectClient.InvalidateCache(r.Context(), &redirectpb.InvalidateCacheRequest{
		ShortCode: code,
	}); err != nil {
		slog.Warn("cache invalidation failed (best-effort)", "short_code", code, "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
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
	case codes.AlreadyExists:
		writeError(w, http.StatusConflict, "conflict", st.Message())
	case codes.Unauthenticated:
		writeError(w, http.StatusUnauthorized, "unauthorized", st.Message())
	case codes.PermissionDenied:
		writeError(w, http.StatusForbidden, "forbidden", st.Message())
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
