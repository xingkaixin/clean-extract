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
	"unicode"
	"unicode/utf8"
	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/filesystem"
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
		RemovedFiles:  nonKeepCount,
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

// processArchive 是处理单个压缩文件的核心函数
func processArchive(archivePath string) []ManifestEntry {

	// 控制台输出：解压开始
	consoleLogger.Printf("解压开始: %s", filepath.Base(archivePath))

	fileLogger.Printf("正在处理: %s", archivePath)
	extractDir := strings.TrimSuffix(archivePath, filepath.Ext(archivePath))

	// 检查目录是否已存在且有内容（支持手工解压后的情况）
	if dirExistsAndHasContent(extractDir) {
		fileLogger.Printf("目录已存在且有内容，跳过解压直接清理: %s", extractDir)
		consoleLogger.Printf("发现已解压目录: %s", filepath.Base(archivePath))

		// 直接执行清理逻辑
		stats := cleanDirectory(extractDir)

		// 收集保留的文件信息
		var manifestEntries []ManifestEntry
		filepath.Walk(extractDir, func(p string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				absFilePath, err1 := filepath.Abs(p)
				absArchivePath, err2 := filepath.Abs(archivePath)
				if err1 != nil || err2 != nil {
					fileLogger.Printf("警告: 无法获取绝对路径，跳过文件: %s", p)
					return nil
				}

				manifestEntries = append(manifestEntries, ManifestEntry{
					Filename:          info.Name(),
					Filepath:          absFilePath,
					SourceArchiveName: filepath.Base(archivePath),
					SourceArchivePath: absArchivePath,
				})
			}
			return nil
		})

		// 控制台输出：清理结果
		consoleLogger.Printf("清理完成: %s (总计:%d 保留:%d 移除:%d)",
			filepath.Base(archivePath), stats.TotalFiles, stats.KeptFiles, stats.RemovedFiles)

		return manifestEntries
	}

	// 目录不存在，尝试解压
	if err := os.MkdirAll(extractDir, os.ModePerm); err != nil {
		fileLogger.Printf("错误: 无法为 %s 创建解压目录: %v", archivePath, err)
		consoleLogger.Printf("解压失败: %s (无法创建目录)", filepath.Base(archivePath))
		return []ManifestEntry{}
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
		fileLogger.Printf("错误: 解压 %s 失败: %v", archivePath, extractErr)
		consoleLogger.Printf("解压失败: %s (%s)", filepath.Base(archivePath), extractErr.Error())

		// 即使解压失败，如果目录已存在且有内容（手工解压），仍然执行清理
		if dirExistsAndHasContent(extractDir) {
			fileLogger.Printf("解压失败但目录有内容，执行清理: %s", extractDir)
			stats := cleanDirectory(extractDir)

			var manifestEntries []ManifestEntry
			filepath.Walk(extractDir, func(p string, info os.FileInfo, err error) error {
				if !info.IsDir() {
					absFilePath, err1 := filepath.Abs(p)
					absArchivePath, err2 := filepath.Abs(archivePath)
					if err1 != nil || err2 != nil {
						fileLogger.Printf("警告: 无法获取绝对路径，跳过文件: %s", p)
						return nil
					}

					manifestEntries = append(manifestEntries, ManifestEntry{
						Filename:          info.Name(),
						Filepath:          absFilePath,
						SourceArchiveName: filepath.Base(archivePath),
						SourceArchivePath: absArchivePath,
					})
				}
				return nil
			})

			consoleLogger.Printf("清理完成: %s (总计:%d 保留:%d 移除:%d)",
				filepath.Base(archivePath), stats.TotalFiles, stats.KeptFiles, stats.RemovedFiles)
			return manifestEntries
		}
		return []ManifestEntry{}
	}

	fileLogger.Printf("成功解压: %s", archivePath)

	// 清理目录并获取统计信息
	stats := cleanDirectory(extractDir)

	// 收集保留的文件信息
	var manifestEntries []ManifestEntry
	filepath.Walk(extractDir, func(p string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			absFilePath, err1 := filepath.Abs(p)
			absArchivePath, err2 := filepath.Abs(archivePath)
			if err1 != nil || err2 != nil {
				fileLogger.Printf("警告: 无法获取绝对路径，跳过文件: %s", p)
				return nil
			}

			manifestEntries = append(manifestEntries, ManifestEntry{
				Filename:          info.Name(),
				Filepath:          absFilePath,
				SourceArchiveName: filepath.Base(archivePath),
				SourceArchivePath: absArchivePath,
			})
		}
		return nil
	})

	// 控制台输出：解压结果
	consoleLogger.Printf("解压完成: %s (总计:%d 保留:%d 移除:%d)",
		filepath.Base(archivePath), stats.TotalFiles, stats.KeptFiles, stats.RemovedFiles)

	return manifestEntries
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
	// 检查文件大小，如果超过 2GB 则警告
	fileInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if fileInfo.Size() > 2*1024*1024*1024 { // 2GB
		fileLogger.Printf("警告: 正在处理大 ISO 文件: %s (%.2f GB)", src, float64(fileInfo.Size())/1024/1024/1024)
	}

	// 使用独立的函数处理ISO，便于捕获panic
	return extractIsoWithRecovery(src, dest)
}

func extractIsoWithRecovery(src, dest string) (err error) {
	// 防止panic导致程序崩溃
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ISO文件解析panic: %v", r)
		}
	}()

	disk, err := diskfs.Open(src)
	if err != nil {
		return fmt.Errorf("无法打开ISO文件: %w", err)
	}
	// 旧版本没有Close方法，不需要defer

	fs, err := disk.GetFilesystem(0) // 通常ISO只有一个分区
	if err != nil {
		return fmt.Errorf("无法获取文件系统: %w", err)
	}

	// 使用递归遍历，避免一次性加载所有内容
	return walkIsoFilesystem(fs, "/", dest)
}

func walkIsoFilesystem(fs filesystem.FileSystem, path, dest string) error {
	// 防止panic导致程序崩溃
	defer func() {
		if r := recover(); r != nil {
			fileLogger.Printf("警告: ISO文件解析panic: %v，跳过此文件", r)
		}
	}()
	// 在遍历前检查内存
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	const memoryLimit = 2 * 1024 * 1024 * 1024 // 2GB
	if m.Alloc > memoryLimit {
		runtime.GC()
		runtime.ReadMemStats(&m)
		if m.Alloc > memoryLimit {
			return fmt.Errorf("内存使用过高 (%.2f GB)，停止遍历", float64(m.Alloc)/1024/1024/1024)
		}
	}

	entries, err := fs.ReadDir(path)
	if err != nil {
		return fmt.Errorf("无法读取目录 %s: %w", path, err)
	}

	// 限制单次处理的文件数量
	const batchSize = 50
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]

		for _, entry := range batch {
			entryPath := filepath.Join(path, entry.Name())

			// 智能解码中文文件名
			fileName := decodeChineseFilename(entry.Name())
			targetPath := filepath.Join(dest, path, fileName)

			if entry.IsDir() {
				// 创建目录并递归处理
				if err := os.MkdirAll(targetPath, entry.Mode()); err != nil {
					return fmt.Errorf("无法创建目录 %s: %w", targetPath, err)
				}
				if err := walkIsoFilesystem(fs, entryPath, dest); err != nil {
					return err
				}
			} else {
				// 处理文件
				if err := extractIsoFile(fs, entryPath, targetPath, entry.Size()); err != nil {
					return err
				}
			}
		}

		// 每处理完一批文件后强制垃圾回收
		runtime.GC()
	}

	return nil
}

// extractIsoFile 提取单个ISO文件
func extractIsoFile(fs filesystem.FileSystem, srcPath, destPath string, fileSize int64) error {
	// 在文件处理前检查内存
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	const memoryLimit = 2 * 1024 * 1024 * 1024 // 2GB
	if m.Alloc > memoryLimit {
		runtime.GC()
		runtime.ReadMemStats(&m)
		if m.Alloc > memoryLimit {
			return fmt.Errorf("内存使用过高 (%.2f GB)，跳过文件 %s", float64(m.Alloc)/1024/1024/1024, destPath)
		}
	}

	// 检查文件大小，跳过过大文件
	const maxFileSize = 500 * 1024 * 1024 // 500MB
	if fileSize > maxFileSize {
		fileLogger.Printf("警告: 跳过过大文件: %s (%.2f MB)", destPath, float64(fileSize)/1024/1024)
		return nil
	}

	// 读取文件并写入
	srcFile, err := fs.OpenFile(srcPath, os.O_RDONLY)
	if err != nil {
		return fmt.Errorf("无法读取ISO内文件 %s: %w", srcPath, err)
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("无法创建目标文件 %s: %w", destPath, err)
	}
	defer destFile.Close()

	// 使用缓冲区进行流式复制
	buf := make([]byte, 128*1024) // 128KB 缓冲区
	_, err = io.CopyBuffer(destFile, srcFile, buf)

	// 强制垃圾回收
	runtime.GC()

	return err
}



