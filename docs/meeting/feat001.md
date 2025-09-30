# ISO内部解析方案评估

## 问题描述

当前使用`github.com/diskfs/go-diskfs`库处理ISO文件时存在兼容性问题：
- 外部依赖在某些系统上工作不正常
- 大文件内存管理仍有优化空间
- 中文字符编码兼容性问题

## Linus式解决方案评估

### 方案1：轻量级ISO解析器（推荐）
**核心思想**：实现最小化的ISO 9660解析器，只读取基本结构

**优势**：
- 代码量小（~300行）
- 无外部依赖
- 内存占用低（流式读取）
- 处理95%的标准ISO

**核心数据结构**：
```go
type ISOHeader struct {
    VolumeDescriptor [2048]byte
    PathTables      []byte
    RootDirectory   DirectoryEntry
}

type DirectoryEntry struct {
    Length        uint8
    ExtAttrLength uint8
    ExtentLocation uint32
    DataLength    uint32
    Flags         uint8
    Identifier    string
}
```

### 方案2：多库回退机制
**核心思想**：尝试多个ISO解析库，直到成功

```go
func extractIsoWithFallback(src, dest string) error {
    // 尝试diskfs/go-diskfs
    if err := extractWithDiskfs(src, dest); err == nil {
        return nil
    }

    // 尝试KarpelesLab/iso9660
    if err := extractWithKarpeles(src, dest); err == nil {
        return nil
    }

    // 最后回退到7z
    return extractWith7z(src, dest)
}
```

### 方案3：混合方案（最实用）
**核心思想**：实现基本ISO解析 + 复杂格式回退

```go
func extractIso(src, dest string) error {
    // 先尝试轻量级解析器
    if isStandardISO(src) {
        return extractWithSimpleParser(src, dest)
    }

    // 复杂ISO尝试diskfs
    if err := extractWithDiskfs(src, dest); err == nil {
        return nil
    }

    return fmt.Errorf("无法解析ISO文件，请安装7z")
}
```

## 技术实现要点

### 1. ISO 9660结构解析
- 从16扇区开始读取卷描述符（每个扇区2048字节）
- 解析主卷描述符和路径表
- 递归遍历目录结构

### 2. 流式文件提取
```go
func extractFile(file *os.File, entry DirectoryEntry, path, dest string) error {
    // 定位到文件数据位置
    file.Seek(int64(entry.ExtentLocation)*2048, io.SeekStart)

    // 流式复制，避免内存问题
    remaining := entry.DataLength
    buf := make([]byte, 64*1024) // 64KB缓冲区

    for remaining > 0 {
        toRead := min(buf, remaining)
        n, err := file.Read(buf[:toRead])
        if err != nil {
            return err
        }

        destFile.Write(buf[:n])
        remaining -= uint32(n)
    }

    return nil
}
```

### 3. 字符编码处理
- 支持标准ASCII文件名
- 扩展Joliet格式（Unicode）
- 中文编码智能检测（GB18030/GBK）

## 实现阶段规划

### 第一阶段：基础功能
- [ ] 实现基本ISO 9660解析
- [ ] 支持标准目录结构
- [ ] 流式文件提取
- [ ] 基本错误处理

### 第二阶段：扩展支持
- [ ] Joliet扩展（Unicode文件名）
- [ ] Rock Ridge扩展（Unix权限）
- [ ] 更好的中文编码支持

### 第三阶段：优化完善
- [ ] 性能优化
- [ ] 内存管理优化
- [ ] 完善的错误处理和回退机制

## 评估结论

**优势**：
- 减少外部依赖，提高兼容性
- 更好的内存控制
- 针对中文环境优化
- 代码更简洁易维护

**风险**：
- 需要处理各种ISO格式变体
- 可能无法解析某些特殊ISO
- 需要充分测试

**建议**：优先考虑方案3（混合方案），在保证兼容性的前提下逐步替换外部依赖。

## 后续行动

1. 收集更多ISO样本进行测试
2. 评估当前兼容性问题的具体表现
3. 制定详细的实现计划
4. 分阶段实施和测试