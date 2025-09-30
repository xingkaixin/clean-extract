# Clean Extract 配置规格文档

## 配置文件概述

Clean Extract 使用 TOML 格式的配置文件 `config.toml` 来定义文件保留策略和优先级规则。配置文件必须位于程序运行的工作目录中。

## 配置文件结构

### 基本格式
```toml
# config.toml

KeepExtensions = ["txt", "pdf", "jpg", "png"]
Priority = ["pdf", "txt", "jpg", "png"]
```

## 配置项详细说明

### 1. KeepExtensions
**类型**: `[]string` (字符串数组)
**必需**: 否
**默认值**: 无（必须配置至少一个扩展名）

**功能**: 定义要保留的文件扩展名列表

**说明**:
- 不需要包含点号（`.`）
- 扩展名匹配不区分大小写
- 只有在此列表中的文件才会被保留
- 其他文件将被移动到 `.remove` 目录

**示例**:
```toml
# 只保留文档和图片文件
KeepExtensions = ["pdf", "docx", "jpg", "png", "gif"]

# 保留代码文件
KeepExtensions = ["go", "js", "html", "css", "json"]

# 保留多媒体文件
KeepExtensions = ["mp4", "avi", "mp3", "flac", "srt"]
```

### 2. Priority
**类型**: `[]string` (字符串数组)
**必需**: 否
**默认值**: 使用 `KeepExtensions` 的顺序

**功能**: 定义文件扩展名的优先级顺序

**说明**:
- 数组中的索引决定优先级（索引越小优先级越高）
- 当存在同名不同扩展名的文件时，按此优先级保留
- 必须是 `KeepExtensions` 的子集
- 如果未配置，将自动使用 `KeepExtensions` 的顺序

**优先级处理示例**:
```toml
KeepExtensions = ["pdf", "txt", "docx", "jpg"]
Priority = ["pdf", "docx", "txt", "jpg"]

# 对于文件 "report" 的多个版本：
# - report.pdf (优先级最高，保留)
# - report.docx (优先级次之，移动到 .remove)
# - report.txt (优先级再次，移动到 .remove)
# - report.jpg (优先级最低，移动到 .remove)
```

## 配置文件示例

### 示例1: 文档优先配置
```toml
# 适用于文档处理场景
KeepExtensions = ["pdf", "docx", "txt", "rtf", "odt"]
Priority = ["pdf", "docx", "txt", "rtf", "odt"]
```

### 示例2: 开发环境配置
```toml
# 适用于代码和文档混合环境
KeepExtensions = ["go", "js", "py", "java", "md", "json", "yaml"]
Priority = ["md", "go", "py", "js", "java", "json", "yaml"]
```

### 示例3: 媒体处理配置
```toml
# 适用于媒体文件处理
KeepExtensions = ["mp4", "avi", "mkv", "mp3", "flac", "srt", "ass"]
Priority = ["mp4", "mkv", "avi", "srt", "ass", "mp3", "flac"]
```

### 示例4: 游戏MOD配置
```toml
# 适用于游戏MOD文件处理
KeepExtensions = ["esp", "esm", "bsa", "txt", "ini"]
Priority = ["esp", "esm", "bsa", "ini", "txt"]
```

## 配置验证规则

### 1. 语法验证
- 必须是有效的 TOML 格式
- 数组元素必须是字符串
- 不支持嵌套结构

### 2. 逻辑验证
- `KeepExtensions` 不能为空
- `Priority` 中的所有项目必须在 `KeepExtensions` 中
- 扩展名不能重复

### 3. 错误处理
程序启动时会进行配置验证，发现错误时会：

**配置文件不存在**:
```
错误: 无法读取配置文件: open config.toml: no such file or directory
```

**TOML语法错误**:
```
错误: 无法解析配置文件: toml: line 3: expected a comma (',') or array end (']')
```

**逻辑验证错误**:
程序会记录警告并尝试修复，如：
- `Priority` 为空时使用 `KeepExtensions` 的顺序
- 发现未在 `KeepExtensions` 中的 `Priority` 项目时会忽略

## 高级配置场景

### 1. 条件保留策略
通过合理的 `Priority` 配置实现条件保留：

```toml
# 优先保留源文件，其次编译文件
KeepExtensions = ["c", "cpp", "h", "o", "exe", "txt"]
Priority = ["c", "cpp", "h", "txt", "o", "exe"]
```

### 2. 质量优先策略
按文件质量优先级配置：

```toml
# 图片质量优先：原始格式 > 压缩格式
KeepExtensions = ["png", "bmp", "tiff", "jpg", "gif", "webp"]
Priority = ["png", "bmp", "tiff", "webp", "jpg", "gif"]
```

### 3. 多语言文档策略
处理多语言文档：

```toml
# 中文优先，其次英文
KeepExtensions = ["pdf", "docx", "txt"]
Priority = ["pdf", "docx", "txt"]

# 注意：实际的语言识别需要文件名约定配合
```

## 配置文件维护

### 1. 版本控制
- 将 `config.toml` 纳入版本控制
- 为不同环境创建不同的配置文件
- 使用配置文件模板简化部署

### 2. 配置备份
```bash
# 备份当前配置
cp config.toml config.toml.backup

# 创建环境特定配置
cp config.toml config.production.toml
cp config.toml config.development.toml
```

### 3. 配置验证工具
可以创建简单的验证脚本：

```bash
#!/bin/bash
# validate-config.sh

if [ ! -f "config.toml" ]; then
    echo "错误: config.toml 文件不存在"
    exit 1
fi

# 使用 go 程序验证配置
echo "验证配置文件..."
./clean-extract /tmp 2>&1 | grep -q "配置加载失败"
if [ $? -eq 0 ]; then
    echo "配置文件验证失败"
    exit 1
else
    echo "配置文件验证通过"
fi
```

## 配置最佳实践

### 1. 明确性原则
- 扩展名列表要明确，避免使用通配符
- 优先级顺序要有明确的业务逻辑
- 配置项要有注释说明用途

### 2. 最小化原则
- 只保留真正需要的文件类型
- 避免过度复杂的优先级规则
- 定期清理不需要的配置项

### 3. 一致性原则
- 团队内统一配置文件格式
- 使用标准的扩展名命名
- 保持配置文件的可读性

### 4. 测试原则
- 在测试环境中验证配置效果
- 使用小规模数据测试配置规则
- 记录配置变更和效果对比

## 配置文件模板

创建项目时可以使用的模板：

```toml
# Clean Extract 配置文件模板
#
# 说明：根据项目需求修改以下配置
#
# KeepExtensions: 定义要保留的文件扩展名
# Priority: 定义文件优先级（优先级从高到低）

# ========== 请根据项目需求修改以下配置 ==========

KeepExtensions = [
    # 在这里添加要保留的文件扩展名
    # 示例：
    # "pdf", "docx", "txt", "jpg", "png"
]

Priority = [
    # 在这里定义优先级顺序
    # 如果不配置，将使用 KeepExtensions 的顺序
    # 示例：
    # "pdf", "docx", "txt", "jpg", "png"
]

# ========== 配置说明 ==========
#
# 1. 扩展名不需要点号（.）
# 2. 匹配不区分大小写
# 3. Priority 必须是 KeepExtensions 的子集
# 4. 数组中索引越小优先级越高
#
# ========== 常用扩展名参考 ==========
#
# 文档: pdf, docx, txt, rtf, odt, md
# 图片: jpg, png, gif, bmp, tiff, webp
# 音频: mp3, flac, wav, aac, ogg
# 视频: mp4, avi, mkv, mov, wmv, flv
# 代码: go, js, py, java, cpp, html, css
# 压缩: zip, rar, 7z, tar, gz
# 数据: json, xml, csv, yaml, sql
```