package main

import (
	"log"
	"os"
)

// --- 日志配置 ---
func setupLogging() *os.File {
	logFile, err := os.Create("processing.log")
	if err != nil {
		log.Fatalf("无法创建日志文件: %v", err)
	}

	// 详细日志记录器（写入文件）
	fileLogger = log.New(logFile, "", log.LstdFlags)
	fileLogger.Println("日志系统已初始化。")

	// 简单输出记录器（控制台，无时间戳）
	consoleLogger = log.New(os.Stdout, "", 0)

	return logFile
}