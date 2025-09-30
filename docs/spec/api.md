# Clean Extract API 接口文档

## 命令行接口

### 基本语法
```bash
./clean-extract <目录路径>
```

### 参数说明
- `<目录路径>`: 要处理的根目录路径
- 必需参数，未提供时显示帮助信息并退出

### 使用示例
```bash
# 处理当前目录
./clean-extract .

# 处理指定目录
./clean-extract /path/to/archives

# 处理用户目录
./clean-extract ~/Downloads/archives
```

## 核心函数接口

### 1. 配置管理接口

#### `loadConfig() error`
**功能**: 加载和解析配置文件

**返回值**:
- `error`: 配置加载错误信息

**依赖**:
- 读取当前目录下的 `config.toml` 文件
- 使用 `github.com/pelletier/go-toml/v2` 解析

**行为**:
- 解析 TOML 配置到全局 `config` 变量
- 如果未配置 `Priority`，使用 `KeepExtensions` 的顺序
- 记录配置加载成功的日志

### 2. 日志系统接口

#### `setupLogging() *os.File`
**功能**: 初始化双重日志系统

**返回值**:
- `*os.File`: 日志文件句柄

**行为**:
- 创建 `processing.log` 文件用于详细日志
- 初始化 `fileLogger`（带时间戳的文件日志）
- 初始化 `consoleLogger`（无时间戳的控制台输出）

### 3. 压缩文件处理接口

#### `processArchive(archivePath string) []ManifestEntry`
**功能**: 处理单个压缩文件的核心函数

**参数**:
- `archivePath`: 压缩文件路径

**返回值**:
- `[]ManifestEntry`: 处理后的文件清单

**处理流程**:
1. 检查是否已存在解压目录
2. 根据文件扩展名选择解压方法
3. 执行文件清理和优先级处理
4. 收集文件清单信息

**支持的格式**:
- `.zip` - 使用 `extractZip()`
- `.rar` - 使用 `extractRar()`
- `.iso` - 使用 `extractIso()`

### 4. 文件清理接口

#### `cleanDirectory(path string) ProcessStats`
**功能**: 清理目录，按优先级保留文件

**参数**:
- `path`: 要清理的目录路径

**返回值**:
- `ProcessStats`: 处理统计信息

**处理步骤**:
1. **文件分组**: 按目录+基础文件名分组
2. **优先级排序**: 对每组文件按配置的优先级排序
3. **文件移动**: 保留最高优先级文件，移动其他文件到 `.remove` 目录
4. **非保留文件**: 移动不在保留列表中的文件
5. **空目录清理**: 删除处理后的空目录

#### `moveToRemove(filePath string)`
**功能**: 将文件移动到 `.remove` 目录

**参数**:
- `filePath`: 要移动的文件路径

**行为**:
- 创建 `.remove` 目录（如果不存在）
- 处理文件名冲突（添加数字后缀）
- 执行文件移动操作

### 5. 中文编码接口

#### `decodeChineseFilename(name string) string`
**功能**: 智能解码中文文件名

**参数**:
- `name`: 原始文件名

**返回值**:
- `string`: 解码后的文件名

**解码策略**:
1. 检查是否已经是有效的UTF-8且无乱码
2. 尝试 GB18030 解码
3. 尝试 GBK 解码
4. 如果都失败，返回原字符串

#### `containsGarbled(s string) bool`
**功能**: 检测字符串是否包含乱码

**参数**:
- `s`: 要检测的字符串

**返回值**:
- `bool`: 是否包含乱码

**检测方法**:
- 常见乱码模式匹配
- 中文字符占比分析

### 6. 解压函数接口

#### `extractZip(src, dest string) error`
**功能**: 解压ZIP文件

**参数**:
- `src`: ZIP文件路径
- `dest`: 目标目录路径

**特性**:
- 智能处理中文文件名
- 保持文件权限
- 创建目录结构

#### `extractRar(src, dest string) error`
**功能**: 解压RAR文件

**参数**:
- `src`: RAR文件路径
- `dest`: 目标目录路径

**依赖**:
- `github.com/nwaples/rardecode`

#### `extractIso(src, dest string) error`
**功能**: 解压ISO文件

**参数**:
- `src`: ISO文件路径
- `dest`: 目标目录路径

**特性**:
- 大文件支持（>2GB警告）
- 使用7z命令行工具
- 跨平台兼容

## 数据结构接口

### Config 结构体
```go
type Config struct {
    KeepExtensions []string `toml:"KeepExtensions"`
    Priority       []string `toml:"Priority"`
}
```

### ManifestEntry 结构体
```go
type ManifestEntry struct {
    Filename          string
    Filepath          string
    SourceArchiveName string
    SourceArchivePath string
}
```

### ProcessStats 结构体
```go
type ProcessStats struct {
    TotalFiles    int
    KeptFiles     int
    RemovedFiles  int
    Success       bool
    ErrorMsg      string
}
```

## 全局变量接口

### 日志记录器
```go
var (
    fileLogger    *log.Logger  // 文件日志记录器
    consoleLogger *log.Logger  // 控制台日志记录器
)
```

### 配置和常量
```go
var (
    ARCHIVE_EXTS map[string]bool // 支持的压缩文件扩展名
    config       Config          // 全局配置
)
```

## 错误处理接口

### 错误类型
- **配置错误**: `loadConfig()` 返回的TOML解析错误
- **文件系统错误**: 文件操作相关的系统错误
- **解压错误**: 压缩文件损坏或格式不支持
- **编码错误**: 中文编码转换失败

### 错误处理策略
- **非致命错误**: 记录日志，继续处理其他文件
- **致命错误**: 输出错误信息，程序退出
- **部分失败**: 跳过问题文件，处理后续内容

## 输出接口

### 控制台输出
- 处理进度信息
- 统计结果摘要
- 错误和警告信息

### 文件输出
- **processing.log**: 详细的处理日志
- **file_manifest.csv**: 文件清单（CSV格式）

### CSV格式
```csv
filename,filepath,source_archive_name,source_archive_path
example.txt,/path/to/example.txt,archive.zip,/path/to/archive.zip
```