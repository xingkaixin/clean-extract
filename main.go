package main

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/kdomanski/iso9660"
	"github.com/nwaples/rardecode"
	"github.com/pelletier/go-toml/v2"
)

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
	// 日志记录器
	logger *log.Logger
)

// --- Manifest 数据结构 ---
type ManifestEntry struct {
	Filename          string
	Filepath          string
	SourceArchiveName string
	SourceArchivePath string
}

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

	logger.Printf("配置加载成功: KeepExtensions=%v, Priority=%v", config.KeepExtensions, config.Priority)
	return nil
}

// --- 日志配置 ---
func setupLogging() *os.File {
	logFile, err := os.Create("processing.log")
	if err != nil {
		log.Fatalf("无法创建日志文件: %v", err)
	}
	// 将日志同时输出到文件和控制台
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logger = log.New(multiWriter, "", log.LstdFlags)
	logger.Println("日志系统已初始化。")
	return logFile
}

// --- 核心功能 ---

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
func cleanDirectory(path string) {
	logger.Printf("开始清理目录: %s", path)

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
				logger.Printf("发现保留文件: %s (扩展名: %s, 分组键: %s)", p, ext, groupKey)
			} else {
				logger.Printf("发现非保留文件: %s (扩展名: %s)", p, ext)
			}
		}
		return nil
	})

	logger.Printf("文件分组统计: %d个基础文件名有同名文件", len(fileGroups))

	// 第二步：对每个文件组按优先级处理
	processedFiles := make(map[string]bool)
	for groupKey, files := range fileGroups {
		logger.Printf("处理文件组: %s, 文件数量: %d", groupKey, len(files))
		if len(files) == 1 {
			processedFiles[files[0]] = true
			logger.Printf("文件组 %s 只有一个文件，保留: %s", groupKey, files[0])
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

		logger.Printf("文件组 %s 优先级排序结果:", groupKey)
		for i, file := range files {
			logger.Printf("  %d. %s (优先级: %d)", i+1, file, getFilePriority(filepath.Ext(file)))
		}

		// 保留优先级最高的文件，移动其他文件
		keepFile := files[0]
		processedFiles[keepFile] = true
		logger.Printf("文件组 %s 保留文件: %s (优先级最高)", groupKey, keepFile)

		for i := 1; i < len(files); i++ {
			processedFiles[files[i]] = true
			logger.Printf("文件组 %s 移动文件: %s", groupKey, files[i])
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
			logger.Printf("移动非保留文件: %s", file)
			moveToRemove(file)
		}
	}
	logger.Printf("共处理 %d 个非保留文件", nonKeepCount)

	// 删除空目录
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() && p != path {
			entries, _ := os.ReadDir(p)
			if len(entries) == 0 {
				logger.Printf("删除空目录: %s", p)
				os.Remove(p)
			}
		}
		return nil
	})
}

// moveToRemove 将文件移动到同目录下的.remove目录
func moveToRemove(filePath string) {
	dir := filepath.Dir(filePath)
	removeDir := filepath.Join(dir, ".remove")

	// 创建.remove目录
	if err := os.MkdirAll(removeDir, os.ModePerm); err != nil {
		logger.Printf("错误: 无法创建.remove目录 %s: %v", removeDir, err)
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
		logger.Printf("错误: 无法移动文件 %s 到 %s: %v", filePath, destPath, err)
		return
	}

	logger.Printf("移动文件: %s -> %s", filePath, destPath)
}

// processArchive 是处理单个压缩文件的核心函数
func processArchive(archivePath string, wg *sync.WaitGroup, manifestChan chan<- ManifestEntry) {
	defer wg.Done()

	logger.Printf("正在处理: %s", archivePath)
	extractDir := strings.TrimSuffix(archivePath, filepath.Ext(archivePath))

	if err := os.MkdirAll(extractDir, os.ModePerm); err != nil {
		logger.Printf("错误: 无法为 %s 创建解压目录: %v", archivePath, err)
		return
	}

	var extractErr error
	switch strings.ToLower(filepath.Ext(archivePath)) {
	case ".zip":
		extractErr = extractZip(archivePath, extractDir)
	case ".rar":
		extractErr = extractRar(archivePath, extractDir)
	case ".iso":
		extractErr = extractIso(archivePath, extractDir)
	}

	if extractErr != nil {
		logger.Printf("错误: 解压 %s 失败: %v", archivePath, extractErr)
		return
	}

	logger.Printf("成功解压: %s", archivePath)
	cleanDirectory(extractDir)

	// 收集保留的文件信息并发送到 channel
	filepath.Walk(extractDir, func(p string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			absArchivePath, _ := filepath.Abs(archivePath)
			absFilePath, _ := filepath.Abs(p)

			manifestChan <- ManifestEntry{
				Filename:          info.Name(),
				Filepath:          absFilePath,
				SourceArchiveName: filepath.Base(archivePath),
				SourceArchivePath: absArchivePath,
			}
		}
		return nil
	})
}

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
		logger.Fatalf("配置加载失败: %v", err)
	}

	logger.Println("================== 开始执行 ==================")

	var archives []string
	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && ARCHIVE_EXTS[strings.ToLower(filepath.Ext(path))] {
			archives = append(archives, path)
		}
		return nil
	})

	if len(archives) == 0 {
		logger.Println("在指定目录中未找到任何压缩文件。")
		logger.Println("================== 执行完毕 ==================")
		return
	}

	var wg sync.WaitGroup
	manifestChan := make(chan ManifestEntry, 10000) // 更大的缓冲区

	// 创建并发控制器，限制 ISO 文件的并发处理
	isoSemaphore := make(chan struct{}, 2) // 最多同时处理 2 个 ISO 文件

	for _, archive := range archives {
		archive := archive // 创建局部变量避免闭包问题
		wg.Add(1)
		go func() {
			// 如果是 ISO 文件，获取信号量
			if strings.ToLower(filepath.Ext(archive)) == ".iso" {
				isoSemaphore <- struct{}{} // 获取信号量
				defer func() { <-isoSemaphore }() // 释放信号量
			}

			processArchive(archive, &wg, manifestChan)
		}()
	}

	// 等待所有解压任务完成
	wg.Wait()
	close(manifestChan)

	// --- 收集并写入 CSV ---
	csvFile, err := os.Create("file_manifest.csv")
	if err != nil {
		logger.Fatalf("错误: 无法创建 CSV 文件: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	writer.Write([]string{"filename", "filepath", "source_archive_name", "source_archive_path"})
	for entry := range manifestChan {
		writer.Write([]string{entry.Filename, entry.Filepath, entry.SourceArchiveName, entry.SourceArchivePath})
	}

	logger.Println("文件清单 'file_manifest.csv' 已成功生成。")
	logger.Println("================== 执行完毕 ==================")
}

// decodeChineseFilename 智能解码中文字符串
func decodeChineseFilename(name string) string {
	// 如果已经是UTF-8且能正常显示，直接返回
	if isValidUTF8(name) && !containsGarbled(name) {
		return name
	}

	// 尝试GB18030解码
	decoder := simplifiedchinese.GB18030.NewDecoder()
	decoded, err := decoder.String(name)
	if err == nil && !containsGarbled(decoded) {
		return decoded
	}

	// 如果GB18030失败，尝试GBK
	decoder2 := simplifiedchinese.GBK.NewDecoder()
	decoded2, err2 := decoder2.String(name)
	if err2 == nil && !containsGarbled(decoded2) {
		return decoded2
	}

	// 都失败则返回原字符串
	return name
}

// isValidUTF8 检查字符串是否为有效的UTF-8
func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}

// containsGarbled 检查字符串是否包含明显的乱码字符
func containsGarbled(s string) bool {
	// 检查是否包含常见的乱码模式
	garbledPatterns := []string{
		"锛�", "鏃�", "骞�", "鏈�", "鐢�", "鍖�", "鍥�", "鍥�",
		"甯�", "鐢�", "鍦�", "鍖�", "鍖�", "鍗�", "闂�", "闂�",
	}

	for _, pattern := range garbledPatterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}

	// 检查是否有过多的连续中文字符（可能是乱码）
	chineseCount := 0
	for _, r := range s {
		if isChineseChar(r) {
			chineseCount++
		}
	}

	// 如果中文字符占比过高且字符串较长，可能是乱码
	if len(s) > 10 && float64(chineseCount)/float64(len([]rune(s))) > 0.8 {
		return true
	}

	return false
}

// isChineseChar 检查字符是否为中文字符
func isChineseChar(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// --- 解压函数 ---

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// 智能解码中文文件名
		fileName := decodeChineseFilename(f.Name)

		fpath := filepath.Join(dest, fileName)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}

func extractRar(src, dest string) error {
	r, err := rardecode.OpenReader(src, "")
	if err != nil {
		return err
	}
	defer r.Close()

	for {
		header, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// 智能解码中文文件名
		fileName := decodeChineseFilename(header.Name)
		fpath := filepath.Join(dest, fileName)

		if header.IsDir {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
		file, err := os.Create(fpath)
		if err != nil {
			return err
		}
		defer file.Close()

		if _, err := io.Copy(file, r); err != nil {
			return err
		}
	}
	return nil
}

func extractIso(src, dest string) error {
	// 检查内存使用情况，防止内存溢出
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 如果已使用内存超过 1GB，拒绝处理大 ISO 文件
	const maxMemoryUsage = 1024 * 1024 * 1024 // 1GB
	if m.Alloc > maxMemoryUsage {
		return fmt.Errorf("系统内存使用过高 (%.2f GB)，拒绝处理大 ISO 文件以防止内存溢出", float64(m.Alloc)/1024/1024/1024)
	}

	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	// 检查文件大小，如果超过 100MB 则警告
	fileInfo, err := f.Stat()
	if err != nil {
		return err
	}

	if fileInfo.Size() > 100*1024*1024 { // 100MB
		logger.Printf("警告: 正在处理大 ISO 文件: %s (%.2f GB)", src, float64(fileInfo.Size())/1024/1024/1024)
	}

	img, err := iso9660.OpenImage(f)
	if err != nil {
		return fmt.Errorf("无法打开 ISO 镜像: %w", err)
	}

	root, err := img.RootDir()
	if err != nil {
		return fmt.Errorf("无法获取根目录: %w", err)
	}

	// 使用新的保护性遍历函数
	return walkIsoSafe(root, dest)
}

func walkIsoSafe(file *iso9660.File, path string) error {
	// 再次检查内存使用
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 如果内存使用过高，停止处理
	const memoryLimit = 1 * 1024 * 1024 * 1024 // 降低到 1GB
	if m.Alloc > memoryLimit {
		return fmt.Errorf("内存使用过高 (%.2f GB)，停止处理以防止系统崩溃", float64(m.Alloc)/1024/1024/1024)
	}

	// 在 GetChildren() 调用前再次检查，这是最危险的点
	logger.Printf("开始处理目录: %s，当前内存使用: %.2f GB", path, float64(m.Alloc)/1024/1024/1024)

	children, err := file.GetChildren()
	if err != nil {
		return fmt.Errorf("无法获取子目录: %w", err)
	}

	// GetChildren() 后立即检查内存
	runtime.ReadMemStats(&m)
	if m.Alloc > memoryLimit {
		return fmt.Errorf("GetChildren() 后内存使用过高 (%.2f GB)，停止处理", float64(m.Alloc)/1024/1024/1024)
	}

	// 限制单个目录的文件数量，防止恶意文件
	if len(children) > 1000 {
		logger.Printf("警告: 目录 %s 包含过多文件 (%d)，可能影响性能", path, len(children))
	}

	for _, child := range children {
		// 检查是否应该停止处理（内存保护）
		runtime.GC() // 手动触发垃圾回收
		runtime.ReadMemStats(&m)
		if m.Alloc > memoryLimit {
			return fmt.Errorf("达到内存限制，停止处理")
		}

		// 智能解码中文文件名
		childName := decodeChineseFilename(child.Name())
		childPath := filepath.Join(path, childName)

		if child.IsDir() {
			if err := os.MkdirAll(childPath, os.ModePerm); err != nil {
				return fmt.Errorf("无法创建目录 %s: %w", childPath, err)
			}

			if err := walkIsoSafe(child, childPath); err != nil {
				return err
			}
		} else {
			// 处理文件
			if err := extractIsoFile(child, childPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractIsoFile(child *iso9660.File, destPath string) error {
	// 检查文件大小，防止过大文件
	if child.Size() > 100*1024*1024 { // 100MB
		logger.Printf("警告: 跳过过大文件: %s (%.2f MB)", destPath, float64(child.Size())/1024/1024)
		return nil
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("无法创建文件 %s: %w", destPath, err)
	}
	defer f.Close()

	// 使用带缓冲的复制，限制内存使用
	buf := make([]byte, 32*1024) // 32KB 缓冲区
	_, err = io.CopyBuffer(f, child.Reader(), buf)
	if err != nil {
		return fmt.Errorf("无法写入文件 %s: %w", destPath, err)
	}

	return nil
}

