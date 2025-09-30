package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nwaples/rardecode"
)

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

// get7zCommand 获取适用于当前操作系统的7z命令
func get7zCommand() string {
	// 根据操作系统确定7z命令名称
	switch runtime.GOOS {
	case "windows":
		return "7z.exe"
	case "darwin":
		// macOS优先尝试7z，如果没有可以尝试7za
		return "7z"
	default:
		// Linux和其他系统
		return "7z"
	}
}

// extractWith7z 使用7z命令处理ISO文件
func extractWith7z(src, dest string) error {
	// 确保目标目录存在
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("无法创建目标目录: %w", err)
	}

	// 获取适用于当前系统的7z命令
	sevenZipCmd := get7zCommand()

	// 检查7z命令是否存在
	if _, err := exec.LookPath(sevenZipCmd); err != nil {
		// 如果7z不存在，尝试替代方案
		altCmd := getAlternative7zCommand(sevenZipCmd)
		if altCmd == "" {
			return fmt.Errorf("未找到7z命令，请安装7-Zip或p7zip")
		}
		sevenZipCmd = altCmd
		fileLogger.Printf("使用替代7z命令: %s", sevenZipCmd)
	}

	// 使用7z命令解压
	cmd := exec.Command(sevenZipCmd, "x", src, fmt.Sprintf("-o%s", dest), "-y")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s解压失败: %w, 输出: %s", sevenZipCmd, err, string(output))
	}

	// 记录7z输出以便调试
	if len(output) > 0 {
		fileLogger.Printf("%s输出: %s", sevenZipCmd, string(output))
	}

	return nil
}

// getAlternative7zCommand 获取替代的7z命令
func getAlternative7zCommand(_ string) string {
	switch runtime.GOOS {
	case "darwin", "linux":
		// 尝试7za作为替代
		if _, err := exec.LookPath("7za"); err == nil {
			return "7za"
		}
	}
	return ""
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

	// 直接使用7z处理所有ISO文件
	consoleLogger.Printf("使用7z解析器: %s", filepath.Base(src))
	fileLogger.Printf("使用7z解析器处理: %s", src)
	return extractWith7z(src, dest)
}