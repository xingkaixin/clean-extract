package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// getFilePriority 获取文件扩展名的优先级，数字越小优先级越高
func getFilePriority(ext string) int {
	ext = strings.ToLower(ext)
	for i, priorityExt := range config.Priority {
		if "."+priorityExt == ext {
			return i
		}
	}
	return len(config.Priority) // 未配置的扩展名优先级最低
}

// contains 检查字符串是否在切片中
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// isKeepExtension 判断扩展名是否在保留列表中
func isKeepExtension(ext string) bool {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	return contains(config.KeepExtensions, ext)
}

// cleanDirectory 清理目录，按优先级保留文件，移动不需要的文件到.remove目录
func cleanDirectory(path string) ProcessStats {
	fileLogger.Printf("开始清理目录: %s", path)

	// 第一步：收集所有文件，按目录+基础文件名分组
	fileGroups := make(map[string][]string)
	var allFiles []string

	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(p))
			allFiles = append(allFiles, p)
			if isKeepExtension(ext) {
				// 使用目录路径+基础名作为分组键，避免不同目录下的同名文件被分到同一组
				dir := filepath.Dir(p)
				baseName := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
				groupKey := filepath.Join(dir, baseName)
				fileGroups[groupKey] = append(fileGroups[groupKey], p)
				fileLogger.Printf("发现保留文件: %s (扩展名: %s, 分组键: %s)", p, ext, groupKey)
			} else {
				fileLogger.Printf("发现非保留文件: %s (扩展名: %s)", p, ext)
			}
		}
		return nil
	})

	fileLogger.Printf("文件分组统计: %d个基础文件名有同名文件", len(fileGroups))

	// 第二步：对每个文件组按优先级处理
	processedFiles := make(map[string]bool)
	for groupKey, files := range fileGroups {
		fileLogger.Printf("处理文件组: %s, 文件数量: %d", groupKey, len(files))
		if len(files) == 1 {
			processedFiles[files[0]] = true
			fileLogger.Printf("文件组 %s 只有一个文件，保留: %s", groupKey, files[0])
			continue
		}
		if len(files) == 0 {
			continue
		}

		// 按优先级排序并显示
		for i := 0; i < len(files)-1; i++ {
			for j := i + 1; j < len(files); j++ {
				if getFilePriority(filepath.Ext(files[i])) > getFilePriority(filepath.Ext(files[j])) {
					files[i], files[j] = files[j], files[i]
				}
			}
		}

		fileLogger.Printf("文件组 %s 优先级排序结果:", groupKey)
		for i, file := range files {
			fileLogger.Printf("  %d. %s (优先级: %d)", i+1, file, getFilePriority(filepath.Ext(file)))
		}

		// 保留优先级最高的文件，移动其他文件
		keepFile := files[0]
		processedFiles[keepFile] = true
		fileLogger.Printf("文件组 %s 保留文件: %s (优先级最高)", groupKey, keepFile)

		for i := 1; i < len(files); i++ {
			processedFiles[files[i]] = true
			fileLogger.Printf("文件组 %s 移动文件: %s", groupKey, files[i])
			moveToRemove(files[i])
		}
	}

	// 第三步：处理不在保留列表中的文件
	nonKeepCount := 0
	for _, file := range allFiles {
		if processedFiles[file] {
			continue // 已处理过，跳过
		}
		ext := strings.ToLower(filepath.Ext(file))
		if !isKeepExtension(ext) {
			nonKeepCount++
			fileLogger.Printf("移动非保留文件: %s", file)
			moveToRemove(file)
		}
	}
	fileLogger.Printf("共处理 %d 个非保留文件", nonKeepCount)

	// 删除空目录
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() && p != path {
			entries, _ := os.ReadDir(p)
			if len(entries) == 0 {
				fileLogger.Printf("删除空目录: %s", p)
				os.Remove(p)
			}
		}
		return nil
	})

	// 返回统计信息
	stats := ProcessStats{
		TotalFiles:   len(allFiles),
		KeptFiles:    len(allFiles) - nonKeepCount,
		RemovedFiles: nonKeepCount,
		Success:      true,
	}
	return stats
}

// dirExistsAndHasContent 检查目录是否存在且有文件（非空）
func dirExistsAndHasContent(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false // 目录不存在
	}
	if !info.IsDir() {
		return false // 不是目录
	}

	// 检查目录是否有内容
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false // 无法读取目录
	}

	// 统计非目录文件数量
	fileCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			fileCount++
		}
	}

	return fileCount > 0 // 至少有一个文件
}

// moveToRemove 将文件移动到同目录下的.remove目录
func moveToRemove(filePath string) {
	dir := filepath.Dir(filePath)
	removeDir := filepath.Join(dir, ".remove")

	// 创建.remove目录
	if err := os.MkdirAll(removeDir, os.ModePerm); err != nil {
		fileLogger.Printf("错误: 无法创建.remove目录 %s: %v", removeDir, err)
		return
	}

	// 移动文件
	baseName := filepath.Base(filePath)
	destPath := filepath.Join(removeDir, baseName)

	// 如果目标文件已存在，添加数字后缀
	counter := 1
	for {
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(baseName)
		nameWithoutExt := strings.TrimSuffix(baseName, ext)
		destPath = filepath.Join(removeDir, fmt.Sprintf("%s_%d%s", nameWithoutExt, counter, ext))
		counter++
	}

	if err := os.Rename(filePath, destPath); err != nil {
		fileLogger.Printf("错误: 无法移动文件 %s 到 %s: %v", filePath, destPath, err)
		return
	}

	fileLogger.Printf("移动文件: %s -> %s", filePath, destPath)
}