BIN := cm
MODULE := cmd-mgr
VERSION ?= 0.2.0
DIST := dist

LDFLAGS := -s -w -X cmd-mgr/internal/cmd.version=$(VERSION)

.PHONY: build build-macos build-linux build-windows build-all release test vet install integrate clean

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
