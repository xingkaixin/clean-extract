package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: ./clean-extract <目录路径>")
		os.Exit(1)
	}
	rootDir := os.Args[1]

	logFile := setupLogging()
	defer logFile.Close()

	// 加载配置
	if err := loadConfig(); err != nil {
		consoleLogger.Fatalf("配置加载失败: %v", err)
	}

	fileLogger.Println("================== 开始执行 ==================")

	// 控制台输出：开始处理
	consoleLogger.Printf("扫描目录: %s", rootDir)

	var archives []string
	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && ARCHIVE_EXTS[strings.ToLower(filepath.Ext(path))] {
			archives = append(archives, path)
		}
		return nil
	})

	if len(archives) == 0 {
		fileLogger.Println("在指定目录中未找到任何压缩文件。")
		fileLogger.Println("================== 执行完毕 ==================")
		consoleLogger.Printf("未找到任何压缩文件")
		return
	}

	// 控制台输出：找到文件
	consoleLogger.Printf("找到 %d 个压缩文件", len(archives))

	// 使用串行处理，避免内存累积
	var manifestEntries []ManifestEntry

	for _, archive := range archives {
		// 直接处理，收集 manifest 数据
		entries := processArchive(archive)
		manifestEntries = append(manifestEntries, entries...)
	}

	// --- 写入 CSV ---
	csvFile, err := os.Create("file_manifest.csv")
	if err != nil {
		consoleLogger.Fatalf("错误: 无法创建 CSV 文件: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	writer.Write([]string{"filename", "filepath", "source_archive_name", "source_archive_path"})
	for _, entry := range manifestEntries {
		writer.Write([]string{entry.Filename, entry.Filepath, entry.SourceArchiveName, entry.SourceArchivePath})
	}

	fileLogger.Println("文件清单 'file_manifest.csv' 已成功生成。")
	fileLogger.Println("================== 执行完毕 ==================")

	// 控制台输出：完成信息
	consoleLogger.Printf("生成文件清单: file_manifest.csv")
	consoleLogger.Printf("所有任务完成！")
}