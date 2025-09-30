## 需求
低频需求
对指定目录内容文件进行操作，对于 `iso` 、`rar`、 `zip` 文件进行解压，解压后目录做清理只保留 `htm` 、`html`、`pdf`、`xls`、`xlsx`
处理完成后压缩包保留
一个目录的文件清单

## 安装

### 1. Go依赖
```bash
go mod tidy
```

### 2. 7z依赖
本工具依赖7z命令来处理ISO文件，请根据操作系统安装对应的7-Zip工具：

#### macOS
```bash
# 使用 Homebrew 安装
brew install p7zip

# 或者使用 MacPorts
sudo port install p7zip
```

#### Linux (Ubuntu/Debian)
```bash
sudo apt-get update
sudo apt-get install p7zip-full
```

#### Linux (CentOS/RHEL/Fedora)
```bash
# CentOS/RHEL
sudo yum install p7zip p7zip-plugins

# Fedora
sudo dnf install p7zip p7zip-plugins
```

#### Windows
1. 从 [7-Zip官网](https://www.7-zip.org/) 下载并安装
2. 确保7z.exe在系统PATH中，或将其放在与clean-extract.exe相同目录

**验证安装**：
```bash
# 检查7z是否可用
7z
# 或
7za
```

## 编译
```bash
make build-all
```

这会编译所有平台版本到dist目录：
- `dist/windows/amd64/clean-extract.exe`
- `dist/darwin/arm64/clean-extract` 等

用 `make clean` 清理构建产物。

## 使用

### 基本用法
```bash
./clean-extract /path/to/outputdir
```

### 配置文件
创建 `config.toml` 文件来配置需要保留的文件扩展名：

```toml
KeepExtensions = ["htm", "html", "pdf", "xls", "xlsx"]
Priority = ["pdf", "xlsx", "xls", "html", "htm"]  # 可选，优先级顺序
```

### 输出
- 处理日志：`processing.log`
- 文件清单：`file_manifest.csv`
- 被移除的文件会移动到各目录的 `.remove` 子目录

### 7z命令自动检测
工具会自动检测当前操作系统的7z命令：
- **Windows**: `7z.exe`
- **macOS**: `7z` → `7za` (备用)
- **Linux**: `7z` → `7za` (备用)

如果找不到7z命令，工具会给出清晰的错误提示。

## 文档

详细的技术文档请参考：

### 📋 规格文档
- **[项目概述](docs/spec/overview.md)** - 项目简介、核心功能和设计原则
- **[架构设计](docs/spec/architecture.md)** - 模块化架构和设计模式
- **[API接口](docs/spec/api.md)** - 命令行接口和核心函数文档
- **[配置规格](docs/spec/configuration.md)** - 配置文件格式和最佳实践
- **[数据流设计](docs/spec/dataflow.md)** - 处理流程和数据流向分析

### 🔧 开发文档
- **代码结构** - 7个专门模块的职责分工
- **性能优化** - 内存管理和磁盘I/O优化策略
- **错误处理** - 异常处理和恢复机制
- **扩展指南** - 如何添加新功能和支持新的压缩格式

### 📈 维护指南
- **配置管理** - 不同环境的配置模板
- **故障排除** - 常见问题和解决方案
- **监控审计** - 日志分析和性能监控