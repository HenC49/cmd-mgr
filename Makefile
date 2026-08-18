BIN := cm
MODULE := cmd-mgr
VERSION ?= 0.1.0
DIST := dist

LDFLAGS := -s -w -X cmd-mgr/internal/cmd.version=$(VERSION)

.PHONY: build build-linux build-windows build-all test vet install integrate clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

build-linux:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-arm64 .

build-windows:
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-windows-amd64.exe .
	GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-windows-arm64.exe .

build-all: build build-linux build-windows

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
