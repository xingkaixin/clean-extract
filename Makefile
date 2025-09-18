# 交叉编译Makefile
# 用法: make build-all

BINARY_NAME := clean-extract
DIST_DIR := dist

# 目标平台列表
PLATFORMS := \
	windows/amd64 \
	windows/386 \
	linux/amd64 \
	linux/386 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64

# 默认目标
.PHONY: all
all: build-all

# 编译所有平台
.PHONY: build-all
build-all: clean
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		OUTPUT=$(DIST_DIR)/$${platform%/*}/$${platform#*/}/$(BINARY_NAME)$$([ $${platform%/*} = windows ] && echo .exe); \
		echo "Building $$OUTPUT..."; \
		mkdir -p $(DIST_DIR)/$${platform%/*}/$${platform#*/}; \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} CGO_ENABLED=0 \
		go build -ldflags="-s -w" -o $$OUTPUT . || exit 1; \
	done
	@echo "Build completed! Files are in $(DIST_DIR)/"

# 清理构建产物
.PHONY: clean
clean:
	@rm -rf $(DIST_DIR)
	@echo "Cleaned build directory"

# 显示可用目标
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make build-all  - Build for all platforms"
	@echo "  make clean      - Clean build directory"
	@echo "  make help       - Show this help"