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

	"github.com/kdomanski/iso9660"
	"github.com/nwaples/rardecode"
)

// --- 全局配置 ---
var (
	// 支持的压缩文件扩展名
	ARCHIVE_EXTS = map[string]bool{".zip": true, ".rar": true, ".iso": true}
	// 解压后需要保留的文件扩展名
	KEEP_EXTS = map[string]bool{".htm": true, ".html": true, ".pdf": true, ".xls": true, ".xlsx": true}
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

// cleanDirectory 清理目录，只保留指定扩展名的文件，并删除空子目录
func cleanDirectory(path string) {
	logger.Printf("开始清理目录: %s", path)
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && !KEEP_EXTS[strings.ToLower(filepath.Ext(p))] {
			logger.Printf("删除文件: %s", p)
			os.Remove(p)
		}
		return nil
	})

	// 再次遍历以删除空目录
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

// --- 解压函数 ---

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
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
