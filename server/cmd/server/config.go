// config 负责装配服务端配置：YAML 配置文件 + 环境变量，优先级为
// 环境变量 > 配置文件 > 默认值。
package main

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	envAddr      = "DOCS_ADDR"
	envDBPath    = "DOCS_DB_PATH"
	envPushToken = "DOCS_PUSH_TOKEN"
	envConfig    = "DOCS_CONFIG"

	defaultAddr = ":8080"
)

// config 是服务端全部可配置项。字段为零值表示「未设置」，由默认值或环境变量补齐。
type config struct {
	Addr      string `yaml:"addr"`
	DBPath    string `yaml:"db_path"`
	PushToken string `yaml:"push_token"`
}

// loadConfig 装配配置：先读配置文件（-config 或 DOCS_CONFIG 指定），
// 再用环境变量逐项覆盖，最后校验必填项。
func loadConfig(file string) (config, error) {
	cfg := config{Addr: defaultAddr}

	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return config{}, fmt.Errorf("读取配置文件 %s: %w", file, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return config{}, fmt.Errorf("解析配置文件 %s: %w", file, err)
		}
		if cfg.Addr == "" {
			cfg.Addr = defaultAddr
		}
	}

	if v := os.Getenv(envAddr); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv(envDBPath); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv(envPushToken); v != "" {
		cfg.PushToken = v
	}

	switch {
	case cfg.DBPath == "":
		return config{}, errors.New("缺少 db_path 配置（配置文件或环境变量 DOCS_DB_PATH）")
	case cfg.PushToken == "":
		return config{}, errors.New("缺少 push_token 配置（配置文件或环境变量 DOCS_PUSH_TOKEN）")
	}
	return cfg, nil
}

// configFilePath 决定配置文件路径：优先命令行 -config，其次环境变量；都未设置则不加载文件。
func configFilePath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv(envConfig)
}
