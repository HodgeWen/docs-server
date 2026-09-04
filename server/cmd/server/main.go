// Command server 是 docs-mcp 的单二进制服务：装配环境变量配置、SQLite 存储
// 与 REST handler，起 HTTP 服务。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"github.com/hodgewen/docs-mcp/server/internal/api"
	"github.com/hodgewen/docs-mcp/server/internal/search"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("服务退出", "err", err)
		os.Exit(1)
	}
}

func run() error {
	addr := os.Getenv("DOCS_MCP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dbPath := os.Getenv("DOCS_MCP_DB_PATH")
	if dbPath == "" {
		return errors.New("缺少环境变量 DOCS_MCP_DB_PATH")
	}
	pushToken := os.Getenv("DOCS_MCP_PUSH_TOKEN")
	if pushToken == "" {
		return errors.New("缺少环境变量 DOCS_MCP_PUSH_TOKEN")
	}

	ctx := context.Background()
	store, err := search.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	srv := &http.Server{
		Addr:    addr,
		Handler: api.NewServer(store, pushToken),
	}
	slog.Info("HTTP 服务启动", "addr", addr, "db", dbPath)
	return srv.ListenAndServe()
}
