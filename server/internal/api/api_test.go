package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hodgewen/docs-mcp/server/internal/search"
)

const testToken = "test-push-token"

// errorCodePattern 约束机器可读错误码为小写下划线串。
var errorCodePattern = regexp.MustCompile(`^[a-z_]+$`)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := search.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return NewServer(store, testToken)
}

// do 发出请求并返回 recorder，body 非空时按 JSON 推送。
func do(t *testing.T, s *Server, method, target, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// pushBody 组装一篇带 frontmatter 的推送请求体。
func pushBody(path, frontmatter, content string) string {
	doc, _ := json.Marshal(map[string]string{
		"path":    path,
		"content": frontmatter + content,
	})
	return "[" + string(doc) + "]"
}

func goodDoc(path, title, content string) string {
	return pushBody(path, "---\ntitle: "+title+"\n---\n", content)
}

// requireError 断言状态码与统一错误格式，返回机器可读错误码。
func requireError(t *testing.T, rec *httptest.ResponseRecorder, status int) string {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("期望状态 %d，实际 %d，body=%s", status, rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("错误响应应为 JSON，实际 Content-Type %q", ct)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("错误响应不是合法 JSON: %v", err)
	}
	if !errorCodePattern.MatchString(body.Error.Code) {
		t.Errorf("错误码应为小写下划线串，实际 %q", body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Error("错误 message 不应为空")
	}
	return body.Error.Code
}

func TestPushAuth(t *testing.T) {
	s := newTestServer(t)
	body := goodDoc("a.md", "文档 A", "sqlite 内容")

	rec := do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", "", body)
	if code := requireError(t, rec, http.StatusUnauthorized); code != "unauthorized" {
		t.Errorf("无令牌错误码应为 unauthorized，实际 %q", code)
	}

	rec = do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", "wrong-token", body)
	requireError(t, rec, http.StatusUnauthorized)

	rec = do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("正确令牌应推送成功，实际 %d，body=%s", rec.Code, rec.Body.String())
	}
}

func TestPushInvalidSlug(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodPut, "/api/v1/libraries/Bad_Slug/documents", testToken, goodDoc("a.md", "A", "x"))
	if code := requireError(t, rec, http.StatusBadRequest); code != "invalid_slug" {
		t.Errorf("错误码应为 invalid_slug，实际 %q", code)
	}
}

func TestPushInvalidBody(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", testToken, "{不是数组")
	if code := requireError(t, rec, http.StatusBadRequest); code != "invalid_request" {
		t.Errorf("错误码应为 invalid_request，实际 %q", code)
	}
}

func TestPushRejectsBatchAndKeepsLibrary(t *testing.T) {
	s := newTestServer(t)
	if rec := do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", testToken,
		goodDoc("old.md", "旧文档", "stable 内容")); rec.Code != http.StatusOK {
		t.Fatalf("首次推送失败: %d", rec.Code)
	}

	// 同批含缺 title 与 YAML 非法的文档，整批拒绝。
	bad := strings.TrimSuffix(goodDoc("ok.md", "新文档", "fresh 内容"), "]") +
		`,` + mustDocJSON(t, "no-title.md", "---\ndescription: 缺标题\n---\n正文\n") + `]`
	rec := do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", testToken, bad)
	if code := requireError(t, rec, http.StatusBadRequest); code != "invalid_frontmatter" {
		t.Errorf("缺 title 错误码应为 invalid_frontmatter，实际 %q", code)
	}

	badYAML := strings.TrimSuffix(goodDoc("ok.md", "新文档", "fresh 内容"), "]") +
		`,` + mustDocJSON(t, "bad-yaml.md", "---\ntitle: [未闭合\n---\n正文\n") + `]`
	rec = do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", testToken, badYAML)
	if code := requireError(t, rec, http.StatusBadRequest); code != "invalid_frontmatter" {
		t.Errorf("YAML 非法错误码应为 invalid_frontmatter，实际 %q", code)
	}

	// 库内容不变：旧文档仍可取回，新文档未入库。
	rec = do(t, s, http.MethodGet, "/api/v1/libraries/alpha/documents/old.md", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("整批拒绝后旧文档应可取回，实际 %d", rec.Code)
	}
	rec = do(t, s, http.MethodGet, "/api/v1/libraries/alpha/documents/ok.md", "", "")
	requireError(t, rec, http.StatusNotFound)
}

func mustDocJSON(t *testing.T, path, content string) string {
	t.Helper()
	doc, err := json.Marshal(map[string]string{"path": path, "content": content})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(doc)
}

func TestReadEndpointsAfterPush(t *testing.T) {
	s := newTestServer(t)
	body := strings.TrimSuffix(goodDoc("guide/intro.md", "入门指南", "sqlite 全文检索教程"), "]") +
		`,` + mustDocJSON(t, "api.md", "---\ntitle: API 参考\ndescription: 接口列表\n---\nsqlite 接口说明\n") + `]`
	if rec := do(t, s, http.MethodPut, "/api/v1/libraries/alpha/documents", testToken, body); rec.Code != http.StatusOK {
		t.Fatalf("推送失败: %d，body=%s", rec.Code, rec.Body.String())
	}

	// 搜索：命中并返回高亮片段，library 过滤生效。
	rec := do(t, s, http.MethodGet, "/api/v1/search?q=sqlite", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("搜索失败: %d", rec.Code)
	}
	var searchBody struct {
		Results []search.Result `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &searchBody); err != nil {
		t.Fatalf("解析搜索响应: %v", err)
	}
	if len(searchBody.Results) != 2 {
		t.Fatalf("期望 2 条命中，实际 %d", len(searchBody.Results))
	}
	first := searchBody.Results[0]
	if first.Library != "alpha" || first.Title == "" || !strings.Contains(first.Snippet, "<mark>") {
		t.Errorf("命中项缺 library/title/高亮: %+v", first)
	}

	rec = do(t, s, http.MethodGet, "/api/v1/search?q=sqlite&library=beta", "", "")
	var filtered struct {
		Results []search.Result `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("解析过滤搜索响应: %v", err)
	}
	if len(filtered.Results) != 0 {
		t.Errorf("library=beta 应无命中，实际 %+v", filtered.Results)
	}

	// 取文档：全文与元数据。
	rec = do(t, s, http.MethodGet, "/api/v1/libraries/alpha/documents/api.md", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("取文档失败: %d", rec.Code)
	}
	var doc struct {
		Library     string `json:"library"`
		Path        string `json:"path"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("解析文档响应: %v", err)
	}
	if doc.Library != "alpha" || doc.Path != "api.md" || doc.Title != "API 参考" ||
		doc.Description != "接口列表" || doc.Content != "sqlite 接口说明\n" {
		t.Errorf("取回文档不符: %+v", doc)
	}

	// 列库。
	rec = do(t, s, http.MethodGet, "/api/v1/libraries", "", "")
	var libs struct {
		Libraries []string `json:"libraries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &libs); err != nil {
		t.Fatalf("解析库列表响应: %v", err)
	}
	if len(libs.Libraries) != 1 || libs.Libraries[0] != "alpha" {
		t.Errorf("库列表不符: %+v", libs)
	}
}

func TestGetDocumentNotFound(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, http.MethodGet, "/api/v1/libraries/alpha/documents/missing.md", "", "")
	if code := requireError(t, rec, http.StatusNotFound); code != "not_found" {
		t.Errorf("错误码应为 not_found，实际 %q", code)
	}
}

func TestUnifiedErrorFormat(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		name   string
		method string
		target string
		token  string
		body   string
		status int
	}{
		{"无令牌", http.MethodPut, "/api/v1/libraries/alpha/documents", "", "[]", http.StatusUnauthorized},
		{"slug 非法", http.MethodPut, "/api/v1/libraries/BAD/documents", testToken, "[]", http.StatusBadRequest},
		{"缺查询参数", http.MethodGet, "/api/v1/search", "", "", http.StatusBadRequest},
		{"文档不存在", http.MethodGet, "/api/v1/libraries/alpha/documents/x.md", "", "", http.StatusNotFound},
		{"路由不存在", http.MethodGet, "/api/v1/ghost", "", "", http.StatusNotFound},
		{"方法不允许", http.MethodPost, "/api/v1/search", "", "", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireError(t, do(t, s, tc.method, tc.target, tc.token, tc.body), tc.status)
		})
	}
}
