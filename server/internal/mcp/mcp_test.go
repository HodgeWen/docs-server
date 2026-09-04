package mcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hodgewen/docs-mcp/server/internal/search"
)

// newTestSession 建库、写入测试文档、起 HTTP 服务并连好 MCP client
// （Connect 内含 initialize），返回会话与存储。
func newTestSession(t *testing.T) (*mcpsdk.ClientSession, *search.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := search.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	seed := map[string][]search.Document{
		"alpha": {
			{Path: "guide.md", Title: "入门指南", Description: "上手教程", Content: "sqlite 全文检索教程"},
			{Path: "api.md", Title: "API 参考", Content: "sqlite 接口说明"},
		},
		"beta": {
			{Path: "readme.md", Title: "Beta 说明", Content: "sqlite 迁移说明"},
		},
	}
	for slug, docs := range seed {
		if err := store.ReplaceLibrary(ctx, slug, docs); err != nil {
			t.Fatalf("ReplaceLibrary %s: %v", slug, err)
		}
	}

	ts := httptest.NewServer(NewHandler(store))
	t.Cleanup(ts.Close)

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session, store
}

func callTool(t *testing.T, session *mcpsdk.ClientSession, name string, args any) *mcpsdk.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("CallTool %s 返回错误: %+v", name, res.Content)
	}
	return res
}

// decodeStructured 把 tool 结果的结构化内容经 JSON 往返解码到 v。
func decodeStructured(t *testing.T, res *mcpsdk.CallToolResult, v any) {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("Marshal structuredContent: %v", err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("解析 structuredContent %s: %v", raw, err)
	}
}

func TestListToolsExposesExactlyThree(t *testing.T) {
	session, _ := newTestSession(t)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	want := []string{"get_document", "list_libraries", "search"}
	if !slices.Equal(names, want) {
		t.Errorf("tools 应恰好为 %v，实际 %v", want, names)
	}
}

func TestSearchToolMatchesStoreResults(t *testing.T) {
	session, store := newTestSession(t)
	res := callTool(t, session, "search", map[string]any{"query": "sqlite"})

	var out struct {
		Results []search.Result `json:"results"`
	}
	decodeStructured(t, res, &out)

	// MCP 结果应与 REST 读路径一致：两者都是 store.Search 的同名字段序列化。
	want, err := store.Search(context.Background(), "sqlite", "", 0)
	if err != nil {
		t.Fatalf("store.Search: %v", err)
	}
	if len(out.Results) != 3 {
		t.Fatalf("期望 3 条命中，实际 %d: %+v", len(out.Results), out.Results)
	}
	for i, r := range out.Results {
		if r != want[i] {
			t.Errorf("第 %d 条与 search 包结果不一致：MCP %+v，store %+v", i, r, want[i])
		}
		if r.Library == "" || r.Path == "" || r.Title == "" {
			t.Errorf("命中项缺 library/path/title: %+v", r)
		}
		if !strings.Contains(r.Snippet, "<mark>") {
			t.Errorf("片段应含高亮标记: %+v", r)
		}
	}
}

func TestSearchToolLibraryFilter(t *testing.T) {
	session, _ := newTestSession(t)
	res := callTool(t, session, "search", map[string]any{"query": "sqlite", "library": "beta"})

	var out struct {
		Results []search.Result `json:"results"`
	}
	decodeStructured(t, res, &out)
	if len(out.Results) != 1 || out.Results[0].Library != "beta" {
		t.Errorf("library=beta 应只命中 beta 库，实际 %+v", out.Results)
	}
}

func TestSearchToolMissingQuery(t *testing.T) {
	session, _ := newTestSession(t)
	// 缺 query：输入 schema 校验失败，SDK 返回 IsError 的 tool 级错误。
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"library": "alpha"},
	})
	if err != nil {
		t.Fatalf("缺 query 应为 tool 级错误而非协议错误: %v", err)
	}
	if !res.IsError {
		t.Errorf("缺必填参数 query 应返回错误结果: %+v", res.Content)
	}

	// query 为空串：schema 校验通过，handler 返回 tool 级错误。
	res, err = session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": ""},
	})
	if err != nil {
		t.Fatalf("空 query 应为 tool 级错误而非协议错误: %v", err)
	}
	if !res.IsError {
		t.Errorf("空 query 应返回错误结果: %+v", res.Content)
	}
}

func TestGetDocumentTool(t *testing.T) {
	session, _ := newTestSession(t)
	res := callTool(t, session, "get_document", map[string]any{"library": "alpha", "path": "api.md"})

	var doc struct {
		Library     string `json:"library"`
		Path        string `json:"path"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Content     string `json:"content"`
	}
	decodeStructured(t, res, &doc)
	if doc.Library != "alpha" || doc.Path != "api.md" || doc.Title != "API 参考" ||
		doc.Content != "sqlite 接口说明" {
		t.Errorf("取回文档不符: %+v", doc)
	}
}

func TestGetDocumentToolNotFound(t *testing.T) {
	session, _ := newTestSession(t)
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "get_document",
		Arguments: map[string]any{"library": "alpha", "path": "missing.md"},
	})
	if err != nil {
		t.Fatalf("文档不存在应为 tool 级错误而非协议错误: %v", err)
	}
	if !res.IsError {
		t.Errorf("不存在的文档应返回错误结果: %+v", res.Content)
	}
}

func TestListLibrariesTool(t *testing.T) {
	session, _ := newTestSession(t)
	res := callTool(t, session, "list_libraries", map[string]any{})

	var out struct {
		Libraries []string `json:"libraries"`
	}
	decodeStructured(t, res, &out)
	if !slices.Equal(out.Libraries, []string{"alpha", "beta"}) {
		t.Errorf("库列表应为 [alpha beta]，实际 %v", out.Libraries)
	}
}
