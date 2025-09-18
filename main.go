package main

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	for _, archive := range archives {
		wg.Add(1)
		go processArchive(archive, &wg, manifestChan)
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

// decodeChineseFilename 解码中文字符串
func decodeChineseFilename(name string) (string, error) {
	// 尝试GB18030解码
	decoder := simplifiedchinese.GB18030.NewDecoder()
	decoded, err := decoder.String(name)
	if err != nil {
		return "", err
	}
	return decoded, nil
}

// --- 解压函数 ---

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// 尝试解码中文文件名
		fileName := f.Name
		if decodedName, err := decodeChineseFilename(f.Name); err == nil {
			fileName = decodedName
		}

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

		fpath := filepath.Join(dest, header.Name)
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
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := iso9660.OpenImage(f)
	if err != nil {
		return err
	}

	root, err := img.RootDir()
	if err != nil {
		return err
	}

	return walkIso(root, dest)
}

func walkIso(file *iso9660.File, path string) error {
	children, err := file.GetChildren()
	if err != nil {
		return err
	}

	for _, child := range children {
		childPath := filepath.Join(path, child.Name())
		if child.IsDir() {
			os.MkdirAll(childPath, os.ModePerm)
			walkIso(child, childPath)
		} else {
			f, err := os.Create(childPath)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(f, child.Reader()); err != nil {
				return err
			}
		}
	}
	return nil
}
