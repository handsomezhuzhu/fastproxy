# FastProxy 构建与操作流程
# 常用: make web build test e2e docker
.PHONY: help web build test vet e2e docker fmt clean

help:
	@echo "make web     构建前端 (web/ -> admin/dist)"
	@echo "make build   构建后端二进制 (embed 已构建的前端)"
	@echo "make test    运行 Go 测试"
	@echo "make vet     静态检查"
	@echo "make e2e     容器端到端测试（需 Docker）"
	@echo "make docker  构建正式镜像（多阶段: node + go + alpine）"
	@echo "make fmt     gofmt"

web:
	cd web && npm ci --no-audit --no-fund && npm run build

build:
	go build -trimpath -ldflags "-s -w" -o fastproxy .

test:
	go test ./... -count=1

vet:
	go vet ./...

e2e:
	bash scripts/e2e.sh

docker:
	docker build -t fastproxy:latest .

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './web/*')

clean:
	rm -f fastproxy fastproxy-e2e-linux devserver-bin
	rm -rf admin/dist web/dist dist
