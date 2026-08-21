BIN := cm
MODULE := cmd-mgr
DIST := dist

# 版本号唯一来源是 internal/version/version.go 的 Version 常量——升版本只改
# 那一处；此处用 awk 反读（可用环境变量 VERSION 覆盖）。
VERSION ?= $(shell awk -F'"' '/^const Version/{print $$2}' internal/version/version.go)

# 版本已在源码常量中，无需 ldflags 注入
LDFLAGS := -s -w

.PHONY: build build-macos build-linux build-windows build-all release tag test vet install integrate clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

build-macos:
	GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-darwin-amd64/$(BIN) .
	GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-darwin-arm64/$(BIN) .

build-linux:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-amd64/$(BIN) .
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-arm64/$(BIN) .

build-windows:
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-windows-amd64/$(BIN).exe .
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-windows-arm64/$(BIN).exe .

build-all: build build-macos build-linux build-windows

# 发布打包：全平台编译 + tar.gz/zip + checksums.txt，产物在 dist/，直接上传 GitHub Release
release: build-macos build-linux build-windows
	cd $(DIST)/$(BIN)-darwin-amd64  && tar -czf ../$(BIN)-darwin-amd64.tar.gz $(BIN)
	cd $(DIST)/$(BIN)-darwin-arm64  && tar -czf ../$(BIN)-darwin-arm64.tar.gz $(BIN)
	cd $(DIST)/$(BIN)-linux-amd64   && tar -czf ../$(BIN)-linux-amd64.tar.gz $(BIN)
	cd $(DIST)/$(BIN)-linux-arm64   && tar -czf ../$(BIN)-linux-arm64.tar.gz $(BIN)
	cd $(DIST)/$(BIN)-windows-amd64 && zip -q ../$(BIN)-windows-amd64.zip $(BIN).exe
	cd $(DIST)/$(BIN)-windows-arm64 && zip -q ../$(BIN)-windows-arm64.zip $(BIN).exe
	cd $(DIST) && { command -v sha256sum >/dev/null 2>&1 && sha256sum *.tar.gz *.zip || shasum -a 256 *.tar.gz *.zip; } > checksums.txt
	@echo "==> 发布产物:" && ls -lh $(DIST)/$(BIN)-*.tar.gz $(DIST)/$(BIN)-*.zip $(DIST)/checksums.txt

test:
	go test ./...

# 按当前版本号打 git tag（如 v0.3.0），安装脚本按 tag 下载 Release 产物
tag:
	git tag "v$(VERSION)"

vet:
	go vet ./...

install: build
	install -m 0755 $(BIN) /usr/local/bin/$(BIN)
	-@./$(BIN) init --install

# 单独安装 shell 集成（make install 已自动执行）
integrate:
	-@./$(BIN) init --install

clean:
	rm -rf $(DIST) $(BIN)
