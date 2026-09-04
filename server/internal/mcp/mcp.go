// Package mcp 提供 /mcp 端点的远程 MCP 服务：streamable HTTP，暴露且仅暴露
// search / get_document / list_libraries 三个 tools。所有 handler 直接调用
// search 包，本层不保存文档副本或检索缓存。
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hodgewen/docs-mcp/server/internal/search"
)

// serverVersion 是 initialize 响应里的服务端版本。
const serverVersion = "v0.1.0"

// NewHandler 装配 MCP server 并返回挂在 /mcp 的 streamable HTTP handler，
// 读路径免鉴权。
func NewHandler(store *search.Store) http.Handler {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "docs-mcp", Version: serverVersion}, nil)
	h := &toolHandlers{store: store}
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search",
		Description: "全文检索库文档，bm25 排序、标题加权，返回高亮片段",
	}, h.search)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_document",
		Description: "按库 slug 与路径取回文档全文与元数据",
	}, h.getDocument)
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "list_libraries",
		Description: "列出全部库 slug",
	}, h.listLibraries)
	return mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return srv }, nil)
}

// toolHandlers 持有检索存储，三个 tool 的 handler 都是它的方法。
type toolHandlers struct {
	store *search.Store
}

type searchArgs struct {
	Query   string `json:"query" jsonschema:"检索关键词，必填"`
	Library string `json:"library,omitempty" jsonschema:"限定单个库 slug，可选，缺省跨库"`
}

// searchOutput 与 REST /api/v1/search 响应保持同名字段。
type searchOutput struct {
	Results []search.Result `json:"results"`
}

func (h *toolHandlers) search(ctx context.Context, _ *mcpsdk.CallToolRequest, args searchArgs) (*mcpsdk.CallToolResult, searchOutput, error) {
	if args.Query == "" {
		return nil, searchOutput{}, errors.New("缺少必填参数 query")
	}
	results, err := h.store.Search(ctx, args.Query, args.Library, 0)
	if err != nil {
		return nil, searchOutput{}, err
	}
	if results == nil {
		results = []search.Result{}
	}
	return nil, searchOutput{Results: results}, nil
}

type getDocumentArgs struct {
	Library string `json:"library" jsonschema:"库 slug，必填"`
	Path    string `json:"path" jsonschema:"文档在库内的相对路径，必填"`
}

// documentOutput 与 REST 取文档响应保持同名字段。
type documentOutput struct {
	Library string `json:"library"`
	search.Document
}

func (h *toolHandlers) getDocument(ctx context.Context, _ *mcpsdk.CallToolRequest, args getDocumentArgs) (*mcpsdk.CallToolResult, documentOutput, error) {
	doc, err := h.store.GetDocument(ctx, args.Library, args.Path)
	if errors.Is(err, search.ErrNotFound) {
		return nil, documentOutput{}, fmt.Errorf("文档 %s/%s 不存在", args.Library, args.Path)
	}
	if err != nil {
		return nil, documentOutput{}, err
	}
	return nil, documentOutput{Library: args.Library, Document: doc}, nil
}

type listLibrariesArgs struct{}

// librariesOutput 与 REST /api/v1/libraries 响应保持同名字段。
type librariesOutput struct {
	Libraries []string `json:"libraries"`
}

func (h *toolHandlers) listLibraries(ctx context.Context, _ *mcpsdk.CallToolRequest, _ listLibrariesArgs) (*mcpsdk.CallToolResult, librariesOutput, error) {
	slugs, err := h.store.ListLibraries(ctx)
	if err != nil {
		return nil, librariesOutput{}, err
	}
	if slugs == nil {
		slugs = []string{}
	}
	return nil, librariesOutput{Libraries: slugs}, nil
}
