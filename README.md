## 需求
低频需求
对指定目录内容文件进行操作，对于 `iso` 、`rar`、 `zip` 文件进行解压，解压后目录做清理只保留 `htm` 、`html`、`pdf`、`xls`、`xlsx`
处理完成后压缩包保留
一个目录的文件清单

## 安装
go mod tidy


## 编译
make build-all

  这会编译所有平台版本到dist目录：
  - dist/windows/amd64/clean-extract.exe
  - dist/linux/amd64/clean-extract
  - dist/darwin/arm64/clean-extract 等

  用make clean清理构建产物。

## 使用
```bash
./clean-extract  /path/to/outputdir
```