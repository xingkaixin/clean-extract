package main

import "log"

// --- 配置结构 ---
type Config struct {
	KeepExtensions []string `toml:"KeepExtensions"`
	Priority       []string `toml:"Priority"`
}

// --- 全局变量 ---
var (
	// 支持的压缩文件扩展名
	ARCHIVE_EXTS = map[string]bool{".zip": true, ".rar": true, ".iso": true}
	// 配置
	config Config
	// 详细日志记录器（写入文件）
	fileLogger *log.Logger
	// 简单输出记录器（控制台）
	consoleLogger *log.Logger
)

// --- Manifest 数据结构 ---
type ManifestEntry struct {
	Filename          string
	Filepath          string
	SourceArchiveName string
	SourceArchivePath string
}

// --- 处理统计结构 ---
type ProcessStats struct {
	TotalFiles    int
	KeptFiles     int
	RemovedFiles  int
	Success       bool
	ErrorMsg      string
}