package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docs-mcp.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写配置文件: %v", err)
	}
	return path
}

func TestLoadConfigFromEnvOnly(t *testing.T) {
	t.Setenv(envDBPath, "/tmp/docs.db")
	t.Setenv(envPushToken, "secret")

	cfg, err := loadConfig("")
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Addr != defaultAddr {
		t.Errorf("Addr = %q, 期望默认 %q", cfg.Addr, defaultAddr)
	}
	if cfg.DBPath != "/tmp/docs.db" || cfg.PushToken != "secret" {
		t.Errorf("配置不符: %+v", cfg)
	}
}

func TestLoadConfigFromFileOnly(t *testing.T) {
	path := writeConfigFile(t, `
addr: ":9090"
db_path: /var/lib/docs.db
push_token: from-file
`)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Addr != ":9090" || cfg.DBPath != "/var/lib/docs.db" || cfg.PushToken != "from-file" {
		t.Errorf("配置不符: %+v", cfg)
	}
}

func TestLoadConfigEnvOverridesFile(t *testing.T) {
	path := writeConfigFile(t, `
addr: ":9090"
db_path: /var/lib/docs.db
push_token: from-file
`)
	t.Setenv(envPushToken, "from-env")

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.PushToken != "from-env" {
		t.Errorf("PushToken = %q, 期望环境变量覆盖为 from-env", cfg.PushToken)
	}
	if cfg.Addr != ":9090" || cfg.DBPath != "/var/lib/docs.db" {
		t.Errorf("未覆盖项应保留文件值: %+v", cfg)
	}
}

func TestLoadConfigMissingRequired(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"缺db_path", "push_token: secret\n"},
		{"缺push_token", "db_path: /tmp/docs.db\n"},
		{"全空", "addr: ':9090'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfigFile(t, tc.content)
			if _, err := loadConfig(path); err == nil {
				t.Fatalf("期望报错，实际成功: %s", tc.content)
			}
		})
	}
}

func TestLoadConfigFileErrors(t *testing.T) {
	t.Run("文件不存在", func(t *testing.T) {
		if _, err := loadConfig("/nonexistent/docs-mcp.yaml"); err == nil {
			t.Fatal("期望报错，实际成功")
		}
	})
	t.Run("非法YAML", func(t *testing.T) {
		path := writeConfigFile(t, "db_path: [未闭合\n")
		if _, err := loadConfig(path); err == nil {
			t.Fatal("期望报错，实际成功")
		}
	})
}

func TestConfigFilePath(t *testing.T) {
	t.Run("flag优先", func(t *testing.T) {
		t.Setenv(envConfig, "/from/env.yaml")
		if got := configFilePath("/from/flag.yaml"); got != "/from/flag.yaml" {
			t.Errorf("configFilePath = %q, 期望 flag 路径", got)
		}
	})
	t.Run("env兜底", func(t *testing.T) {
		t.Setenv(envConfig, "/from/env.yaml")
		if got := configFilePath(""); got != "/from/env.yaml" {
			t.Errorf("configFilePath = %q, 期望 env 路径", got)
		}
	})
	t.Run("都未设置", func(t *testing.T) {
		t.Setenv(envConfig, "")
		if got := configFilePath(""); got != "" {
			t.Errorf("configFilePath = %q, 期望空", got)
		}
	})
}
