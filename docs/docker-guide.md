# Docker 容器化开发指南

本指南涵盖 Swagger UI 预览 OpenAPI 文档、Docker 缓存清理，以及 Oktopus 项目的容器化开发方法。

---

## 1. Swagger UI — 预览 OpenAPI 文档

### 方式一：Docker 一键运行

```bash
# 在项目根目录执行，挂载 docs/openapi.yaml 到 Swagger UI
docker run --rm -d \
  --name swagger-ui \
  -p 8080:8080 \
  -v "$(pwd)/docs/openapi.yaml:/openapi.yaml" \
  -e SWAGGER_JSON=/openapi.yaml \
  swaggerapi/swagger-ui

# 浏览器打开 http://localhost:8080
```

参数说明：

| 参数 | 说明 |
|------|------|
| `--rm` | 容器停止后自动删除，不留残留 |
| `-d` | 后台运行 |
| `-p 8080:8080` | 映射容器 8080 端口到本机 |
| `-v $(pwd)/docs/openapi.yaml:/openapi.yaml` | 挂载本地 OpenAPI 文件 |
| `-e SWAGGER_JSON=/openapi.yaml` | 指定 Swagger 读取的 spec 文件 |

停止并清理：

```bash
docker stop swagger-ui   # 容器已用 --rm，停止即删除
```

### 方式二：docker-compose

```yaml
# deploy/compose/docker-compose.swagger.yaml
services:
  swagger-ui:
    image: swaggerapi/swagger-ui
    container_name: swagger-ui
    ports:
      - "8080:8080"
    environment:
      - SWAGGER_JSON=/openapi.yaml
    volumes:
      - ../../docs/openapi.yaml:/openapi.yaml
```

```bash
cd deploy/compose
docker compose -f docker-compose.swagger.yaml up -d
```

### 方式三：开发模式（文件修改实时生效）

```bash
# 使用 Node.js 本地运行 swagger-ui-dist，配合文件监听
npx @apidevtools/swagger-cli validate docs/openapi.yaml

# 或使用 redoc-cli 生成静态 HTML
npx @redocly/cli build-docs docs/openapi.yaml -o docs/api-docs.html
```

---

## 2. Docker 缓存清理

### 渐进式清理（推荐）

```bash
# 1. 查看磁盘使用情况
docker system df

# 2. 清理无用的构建缓存（安全，不影响运行中的容器）
docker builder prune

# 3. 清理悬空镜像（<none>:<none>）
docker image prune

# 4. 清理停止的容器
docker container prune

# 5. 清理未使用的网络
docker network prune

# 6. 清理未使用的卷
docker volume prune
```

### 一键深度清理

```bash
# 清理所有未使用的数据（镜像、容器、网络、构建缓存）
docker system prune -a

# 包含卷（更彻底，注意卷数据会丢失）
docker system prune -a --volumes
```

### 清理特定目标的缓存

```bash
# 只清理构建缓存
docker builder prune --all

# 按时间过滤（清理 24h 前的构建缓存）
docker builder prune --filter "until=24h"

# 清理特定项目的构建缓存
docker builder prune --filter "label=project=oktopus"

# 清理所有未被任何容器引用的镜像
docker image prune -a

# 清理特定镜像的旧版本（保留最新的 3 个）
docker image ls "oktopusp/*" --format "{{.Repository}}" | sort -u | while read img; do
  docker image ls "$img" --format "{{.ID}}" | tail -n +4 | xargs -r docker rmi
done
```

### 查看占用空间最大的镜像

```bash
docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}" | sort -k 3 -h
```

### system prune 各参数对照

| 命令 | 容器 | 镜像 | 网络 | 卷 | 构建缓存 |
|------|------|------|------|-----|---------|
| `docker system prune` | 停止的 | 未使用的 | 未使用的 | — | 是 |
| `docker system prune -a` | 停止的 | 所有未使用的 | 未使用的 | — | 是 |
| `docker system prune -a --volumes` | 停止的 | 所有未使用的 | 未使用的 | 未使用的 | 是 |

---

## 3. Oktopus 容器化开发方法

### 3.1 项目构建体系

Oktopus 使用多层构建脚本，从源码编译到镜像打包：

```
build/Makefile          →  编排所有服务的构建
backend/services/<svc>/build/Makefile  →  单服务构建
deploy/compose/        →  运行时编排
```

### 3.2 常用命令速查

```bash
# ========== 构建 ==========

# 构建所有服务的 Docker 镜像（源码编译）
cd deploy/compose && ./build.sh

# 构建特定服务（仅 controller）
cd deploy/compose && ./build.sh controller

# 构建指定多个服务
cd deploy/compose && ./build.sh controller frontend

# 查看 build.sh 支持的服务列表
grep -E '^\s+[a-z]' deploy/compose/build.sh | head -20

# ========== 运行 ==========

# 生产模式 — 启动全部服务
cd deploy/compose && ./run.sh

# 开发模式 — 前端热重载（修改代码无需重建镜像）
cd deploy/compose && ./run_debug.sh

# 仅启动核心服务（controller + NATS + MongoDB）
cd deploy/compose && docker compose --profile controller --profile nats up -d

# 仅启动消息传输层
cd deploy/compose && docker compose --profile mqtt --profile ws --profile stomp up -d

# 启动所有服务
cd deploy/compose && docker compose --profile nats --profile controller \
  --profile mqtt --profile ws --profile stomp --profile cwmp \
  --profile adapter --profile frontend up -d

# ========== 停止 ==========

# 停止所有服务
cd deploy/compose && ./stop.sh

# 停止并删除所有数据（包括数据库卷）
cd deploy/compose && docker compose down -v

# 仅停止不删除
cd deploy/compose && docker compose stop

# 停止单个服务（如 frontend）
cd deploy/compose
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml stop frontend

# 停止并删除单个服务
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml down frontend

# ========== 日志查看 ==========

# 查看所有服务日志
cd deploy/compose && docker compose logs -f

# 仅看 controller 日志
cd deploy/compose && docker compose logs -f controller

# 仅看 NATS 日志
cd deploy/compose && docker compose logs -f msg_broker

# 查看最近 100 行
cd deploy/compose && docker compose logs --tail 100 controller

# ========== 进入容器调试 ==========

# 进入 controller 容器
docker exec -it controller sh

# 进入 MongoDB 容器
docker exec -it mongo_usp mongosh

# 查看 NATS JetStream 信息
docker exec -it nats nats server report jetstream

# ========== 重启单个服务 ==========

cd deploy/compose && docker compose restart controller
cd deploy/compose && docker compose restart frontend
```

### 3.3 Profile 说明

`docker-compose.yaml` 使用 profile 控制服务启用：

| Profile | 包含服务 | 说明 |
|---------|---------|------|
| `nats` | msg_broker | NATS 消息中间件 |
| `controller` | controller, mongo_usp | API 核心 + 数据库 |
| `mqtt` | mqtt, mqtt-adapter | MQTT 协议支持 |
| `ws` | ws, ws-adapter | WebSocket 协议支持 |
| `stomp` | stomp, stomp-adapter | STOMP 协议支持 |
| `adapter` | adapter | 通用设备适配器 |
| `frontend` | frontend | Next.js Web 界面 |
| `portainer` | portainer | 容器管理面板 |
| `registry` | registry | Docker 镜像仓库 |

### 3.4 开发工作流

**流程一：修改后端代码**

```bash
# 1. 修改 Go 源码
vim backend/services/controller/internal/api/some_file.go

# 2. 重建指定服务镜像
cd deploy/compose && ./build.sh controller

# 3. 重启服务
cd deploy/compose && docker compose --profile controller up -d controller
```

**流程二：仅运行前端（热重载）**

如果只需要开发前端页面，不涉及后端改动：

```bash
cd deploy/compose

# 方式一：使用 run_debug.sh（已包含所有后端服务）
./run_debug.sh

# 方式二：只启动前端容器（独立前端开发）
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml up -d frontend

# 方式三：只启动前端 + 核心后端（controller + NATS + MongoDB）
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml \
  up -d nginx controller frontend
```

- 修改 `frontend/src/` 下的文件，Next.js 自动热重载，**无需重建镜像**
- `node_modules` 和 `.next` 缓存位于容器内部，不污染宿主机
- 前端访问 `http://localhost:3000`，API 请求通过 nginx 转发到 `localhost:8000`

**停止前端容器：**
```bash
# 停止并删除容器
cd deploy/compose
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml down frontend

# 仅停止（不删除）
docker compose -f docker-compose.yaml -f docker-compose.dev.yaml -f docker-compose.debug.yaml stop frontend
```

**流程三：仅开发后端 API（不启动前端）**

```bash
cd deploy/compose
docker compose --profile nats --profile controller --profile mqtt --profile ws --profile stomp up -d

# 使用 curl 测试 API
curl -X PUT http://localhost/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"..."}'
```

### 3.5 运行单元测试

```bash
# 各服务的测试通过 test profile 运行
cd deploy/compose
docker compose -f docker-compose.test.yaml --profile unit run --rm controller
docker compose -f docker-compose.test.yaml --profile integration run --rm controller
```

### 3.6 镜像管理

```bash
# 查看当前项目的所有镜像
docker images "oktopusp/*"

# 查看镜像历史层
docker history oktopusp/controller

# 查看镜像详细信息
docker inspect oktopusp/controller

# 导出镜像（离线分发）
docker save -o oktopus-controller.tar oktopusp/controller

# 加载镜像
docker load -i oktopus-controller.tar

# 打标签并推送
docker tag oktopusp/controller registry.example.com/oktopus/controller:v1.2.3
docker push registry.example.com/oktopus/controller:v1.2.3
```

### 3.7 容器网络调试

```bash
# 查看网络拓扑
docker network ls

# 查看 Oktopus 网络详情（IP 分配）
docker network inspect deploy_compose_usp_network

# 容器间连通性测试
docker exec controller wget -qO- http://mongo_usp:27017
docker exec controller wget -qO- http://msg_broker:8222/healthz

# 查看容器资源使用
docker stats --filter "name=controller"
docker stats --filter "name=mongo_usp"
```

### 3.8 首次运行完整流程

```bash
# 1. 克隆项目
git clone https://github.com/OktopUSP/oktopus.git && cd oktopus

# 2. 生成密钥（首次运行自动执行，也可手动）
cd deploy/compose && ./generate-secrets.sh

# 3. 构建所有镜像
./build.sh

# 4. 启动所有服务
./run.sh

# 5. 打开浏览器 http://localhost
#    - 首次打开会看到注册页面（创建 SuperAdmin）
#    - 登录后创建 Tenant
#    - 切换 Tenant 后可管理设备
```

### 3.9 常见问题

**MongoDB 连接失败**
```bash
# 确认 MongoDB 已启动
docker compose ps mongo_usp

# 检查健康状态
docker exec mongo_usp mongosh --eval "db.adminCommand('ping')"
```

**NATS 连接失败**
```bash
# 查看 NATS 状态
curl http://localhost:8222/healthz

# 检查 JetStream
docker exec nats nats server report jetstream
```

**端口冲突**
```bash
# 查看端口占用
lsof -i :80
lsof -i :27017

# 修改 compose 中的端口映射或停用冲突服务
```

---

## 4. docker compose 命令对照表

| 命令 | 作用 |
|------|------|
| `docker compose up -d` | 后台启动服务 |
| `docker compose down` | 停止并删除容器/网络 |
| `docker compose down -v` | 同时删除卷（数据会丢失） |
| `docker compose restart <svc>` | 重启指定服务 |
| `docker compose stop <svc>` | 停止指定服务（不删除） |
| `docker compose start <svc>` | 启动已停止的服务 |
| `docker compose logs -f <svc>` | 实时查看日志 |
| `docker compose ps` | 查看服务运行状态 |
| `docker compose exec <svc> sh` | 进入容器 shell |
| `docker compose build <svc>` | 重新构建镜像 |
| `docker compose pull` | 拉取最新镜像 |
| `docker compose --profile <name> up -d` | 按 profile 启动 |
