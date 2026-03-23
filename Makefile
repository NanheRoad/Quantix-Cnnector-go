.PHONY: run build test clean dev vendor build-linux build-windows build-mac

# 运行项目
run:
	CGO_ENABLED=1 go run cmd/server/main.go

# 编译项目
build:
	go build -o bin/quantix-server cmd/server/main.go

# 运行测试
test:
	go test -v ./...

# 带竞态检测运行测试
test-race:
	go test -race -v ./...

# 清理编译产物
clean:
	rm -rf bin/

# 开发模式（编译+运行）
dev: build
	./bin/quantix-server

# 下载依赖到 vendor
vendor:
	go mod vendor

# 整理依赖
tidy:
	go mod tidy

# 跨平台编译
build-linux:
	GOOS=linux GOARCH=amd64 go build -o bin/quantix-server-linux cmd/server/main.go

build-windows:
	GOOS=windows GOARCH=amd64 go build -o bin/quantix-server.exe cmd/server/main.go

build-mac-arm:
	GOOS=darwin GOARCH=arm64 go build -o bin/quantix-server-mac-arm cmd/server/main.go

build-all: build-linux build-windows build-mac-arm

# 格式化代码
fmt:
	go fmt ./...

# 代码检查
lint:
	golangci-lint run ./...

# 查看依赖
deps:
	go mod graph

# 更新依赖
update:
	go get -u ./...
	go mod tidy
