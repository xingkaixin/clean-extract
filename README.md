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
- `dist/linux/amd64/clean-extract`
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