package main

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

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