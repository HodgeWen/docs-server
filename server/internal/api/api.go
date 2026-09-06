// Package api 提供 /api/v1/ 前缀的 REST 接口：推送接收（Bearer 鉴权）与免鉴权读路径。
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/hodgewen/docs-mcp/server/internal/ingest"
	"github.com/hodgewen/docs-mcp/server/internal/search"
)

// Server 是 REST API 的 HTTP handler，全部路由挂在 /api/v1/ 前缀下。
type Server struct {
	store     *search.Store
	pushToken string
	mux       *http.ServeMux
}

// NewServer 装配路由：推送接口要求 Bearer pushToken，读路径免鉴权。
func NewServer(store *search.Store, pushToken string) *Server {
	s := &Server{store: store, pushToken: pushToken, mux: http.NewServeMux()}
	s.mux.HandleFunc("/api/v1/libraries", s.handleLibraries)
	s.mux.HandleFunc("/api/v1/libraries/{slug}/documents", s.handlePushDocuments)
	s.mux.HandleFunc("/api/v1/libraries/{slug}/documents/{path...}", s.handleGetDocument)
	s.mux.HandleFunc("/api/v1/search", s.handleSearch)
	s.mux.HandleFunc("/", s.handleNotFound)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError 输出统一错误格式 {"error":{"code","message"}}。
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorDetail{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("写响应失败", "err", err)
	}
}

// requireMethod 校验请求方法，不符时输出 405 统一错误。
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "该端点仅支持 "+method)
	return false
}

// authorized 校验推送令牌：无或错令牌输出 401。
func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if ok && subtle.ConstantTimeCompare([]byte(token), []byte(s.pushToken)) == 1 {
		return true
	}
	writeError(w, http.StatusUnauthorized, "unauthorized", "缺少或错误的推送令牌")
	return false
}

type pushResponse struct {
	Library   string `json:"library"`
	Documents int    `json:"documents"`
}

// handlePushDocuments 处理 PUT /api/v1/libraries/{slug}/documents：整库覆盖推送。
func (s *Server) handlePushDocuments(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut) {
		return
	}
	if !s.authorized(w, r) {
		return
	}

	var raws []ingest.RawDocument
	if err := json.NewDecoder(r.Body).Decode(&raws); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求体不是合法的文档数组 JSON")
		return
	}

	slug := r.PathValue("slug")
	if err := ingest.Push(r.Context(), s.store, slug, raws); err != nil {
		var parseErr *ingest.ParseError
		switch {
		case errors.Is(err, ingest.ErrInvalidSlug):
			writeError(w, http.StatusBadRequest, "invalid_slug", err.Error())
		case errors.As(err, &parseErr):
			writeError(w, http.StatusBadRequest, "invalid_frontmatter", parseErr.Error())
		default:
			slog.Error("推送写库失败", "library", slug, "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "推送写库失败")
		}
		return
	}
	writeJSON(w, http.StatusOK, pushResponse{Library: slug, Documents: len(raws)})
}

type searchResponse struct {
	Results []search.Result `json:"results"`
}

// handleSearch 处理 GET /api/v1/search?q=...[&library=...]。
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "缺少查询参数 q")
		return
	}

	results, err := s.store.Search(r.Context(), query, r.URL.Query().Get("library"), 0)
	if err != nil {
		slog.Error("检索失败", "q", query, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "检索失败")
		return
	}
	if results == nil {
		results = []search.Result{}
	}
	writeJSON(w, http.StatusOK, searchResponse{Results: results})
}

type librariesResponse struct {
	Libraries []string `json:"libraries"`
}

// handleLibraries 处理 GET /api/v1/libraries。
func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	slugs, err := s.store.ListLibraries(r.Context())
	if err != nil {
		slog.Error("列出库失败", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "列出库失败")
		return
	}
	if slugs == nil {
		slugs = []string{}
	}
	writeJSON(w, http.StatusOK, librariesResponse{Libraries: slugs})
}

type documentResponse struct {
	Library string `json:"library"`
	search.Document
}

// handleGetDocument 处理 GET /api/v1/libraries/{slug}/documents/{path}[?section=...]。
func (s *Server) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	slug, path := r.PathValue("slug"), r.PathValue("path")
	section := r.URL.Query().Get("section")
	doc, err := s.store.GetDocument(r.Context(), slug, path, section)
	if errors.Is(err, search.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "文档不存在")
		return
	}
	if errors.Is(err, search.ErrSectionNotFound) {
		writeError(w, http.StatusNotFound, "section_not_found", err.Error())
		return
	}
	if err != nil {
		slog.Error("取文档失败", "library", slug, "path", path, "section", section, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "取文档失败")
		return
	}
	writeJSON(w, http.StatusOK, documentResponse{Library: slug, Document: doc})
}

// handleNotFound 兜底未匹配路由，保持错误格式统一。
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "路由不存在")
}
