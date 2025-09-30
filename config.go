package main

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// --- 配置加载 ---
func loadConfig() error {
	configData, err := os.ReadFile("config.toml")
	if err != nil {
		return fmt.Errorf("无法读取配置文件: %v", err)
	}

	if err := toml.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("无法解析配置文件: %v", err)
	}

	// 如果没有配置Priority，使用KeepExtensions的顺序
	if len(config.Priority) == 0 {
		config.Priority = make([]string, len(config.KeepExtensions))
		copy(config.Priority, config.KeepExtensions)
	}

	fileLogger.Printf("配置加载成功: KeepExtensions=%v, Priority=%v", config.KeepExtensions, config.Priority)
	return nil
}