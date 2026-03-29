package configs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// LoadConfig 加载配置
func LoadConfig(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}

	cfg := Config{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config from json error: %w", err)
	}

	return cfg, nil
}

// SaveConfig 保存配置
func SaveConfig(path string, cfg Config) error {
	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config to json error: %w", err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write config to %q error: %w", path, err)
	}

	return nil
}

type cfgPathContextKey struct{}

type cfgContextKey struct{}

// ContextWithConfig 创建携带配置信息的上下文
func ContextWithConfig(parent context.Context, config Config, path string) context.Context {
	return context.WithValue(context.WithValue(parent, cfgContextKey{}, config), cfgPathContextKey{}, path)
}

// ConfigPathFromContext 从上下文获取配置文件路径
func ConfigPathFromContext(ctx context.Context) string {
	cfgPath, ok := ctx.Value(cfgPathContextKey{}).(string)
	if !ok {
		return ""
	}
	return cfgPath
}

// ConfigFromContext 从上下文获取配置信息
func ConfigFromContext(ctx context.Context) Config {
	cfg, ok := ctx.Value(cfgContextKey{}).(Config)
	if !ok {
		return Config{}
	}
	return cfg
}
