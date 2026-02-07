# Hexagon 部署 Makefile
#
# 提供 Docker Compose 和 Helm 部署的快捷命令
#
# 使用方式:
#   make help          # 查看所有命令
#   make up            # 一键启动全部服务
#   make dev-up        # 开发模式（连接 docker-dev-env）
#   make helm-install  # Helm 部署到 K8s

.PHONY: help up down dev-up dev-down build logs status clean helm-install helm-upgrade helm-uninstall helm-template

# 默认目标
help: ## 显示帮助信息
	@echo "Hexagon 部署命令："
	@echo ""
	@echo "  Docker Compose（完整模式 - 自包含全套服务）:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | grep -v dev- | grep -v helm- | awk 'BEGIN {FS = ":.*?## "}; {printf "    \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "  Docker Compose（开发模式 - 连接 docker-dev-env）:"
	@grep -E '^dev-[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "    \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "  Helm（K8s 部署）:"
	@grep -E '^helm-[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "    \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# ============== Docker Compose 完整模式 ==============

up: ## 启动全部服务（应用 + 基础设施）
	docker compose up -d

down: ## 停止全部服务
	docker compose down

build: ## 构建 Docker 镜像
	docker compose build

logs: ## 查看应用日志（实时跟踪）
	docker compose logs -f hexagon-app

status: ## 查看服务状态
	docker compose ps

clean: ## 停止服务并删除数据卷
	docker compose down -v

restart: ## 重启应用服务（不重启基础设施）
	docker compose restart hexagon-app hexagon-devui

# ============== Docker Compose 开发模式 ==============

dev-up: ## 启动开发模式（连接 docker-dev-env 基础设施）
	@docker network inspect dev-net > /dev/null 2>&1 || \
		(echo "创建 dev-net 网络..." && docker network create dev-net)
	docker compose -f docker-compose.dev.yml up -d

dev-down: ## 停止开发模式
	docker compose -f docker-compose.dev.yml down

dev-build: ## 构建开发模式镜像
	docker compose -f docker-compose.dev.yml build

dev-logs: ## 查看开发模式日志
	docker compose -f docker-compose.dev.yml logs -f

dev-status: ## 查看开发模式服务状态
	docker compose -f docker-compose.dev.yml ps

# ============== Helm ==============

RELEASE_NAME ?= hexagon
NAMESPACE ?= hexagon
CHART_DIR := helm/hexagon

helm-install: ## 安装到 K8s 集群
	helm install $(RELEASE_NAME) $(CHART_DIR) \
		-n $(NAMESPACE) --create-namespace

helm-upgrade: ## 升级已有部署
	helm upgrade $(RELEASE_NAME) $(CHART_DIR) \
		-n $(NAMESPACE)

helm-uninstall: ## 从 K8s 集群卸载
	helm uninstall $(RELEASE_NAME) -n $(NAMESPACE)

helm-template: ## 预览生成的 K8s 资源（不实际部署）
	helm template $(RELEASE_NAME) $(CHART_DIR)

helm-install-external: ## 安装到 K8s（使用外部基础设施）
	helm install $(RELEASE_NAME) $(CHART_DIR) \
		-n $(NAMESPACE) --create-namespace \
		--set qdrant.enabled=false \
		--set redis.enabled=false \
		--set postgres.enabled=false \
		--set external.qdrant.url=$(QDRANT_URL) \
		--set external.redis.url=$(REDIS_URL) \
		--set external.postgres.dsn=$(POSTGRES_DSN)
