# Oktopus Fork vs Official (OktopUSP) 代码对比分析

## 一、总体概述

| 维度 | 官方 (OktopUSP) | 自定义分叉 (oktopus) |
|------|-----------------|-------------------|
| 功能定位 | 基础 USP/CWMP 控制器 | **多租户 SaaS 平台** |
| API 端点 | ~37 个 | **~68 个** |
| 服务模块 | 7 个 | **8 个（含 firmware-upload）** |
| 固件管理 | 外部文件服务器 | **内置固件上传/存储** |
| 活动管理 | 无 | **Firmware Upgrade Campaigns** |
| 脚本引擎 | 无 | **USP Script Execution Engine** |
| 批量操作 | 无 | **Mass Actions (批量脚本执行)** |
| 多租户 | 无 | **完整租户隔离 (DB/消息/镜像)** |

---

## 二、目录结构差异

### 只在自定义版本中的文件（共 31 个新增文件）

```
controller/internal/api/
├── ca_cert.go          # 租户 CA 证书管理
├── campaign_engine.go  # 固件升级活动引擎（异步）
├── campaign_engine_test.go
├── campaigns.go        # 活动 CRUD API
├── deviceinfo.go       # 设备缓存信息 API
├── firmware.go         # 固件上传/管理 API
├── fwupdate.go         # 固件更新 API
├── handlers_test.go
├── history.go          # 设备消息历史
├── mass_actions.go     # 批量操作 API
├── pagination_test.go
├── script_execution_test.go
├── scripts.go          # 脚本 CRUD + 执行引擎
├── tenant.go           # 租户 CRUD API
└── topology.go         # 网络拓扑 API

controller/internal/db/
├── campaigns.go        # 活动数据库模型
├── device_info.go      # 设备信息缓存
├── firmware.go         # 固件模型 + CRUD
├── fw_policy.go        # 单设备升级策略
├── mass_actions.go     # 批量操作模型
├── message.go          # 消息历史记录
├── metrics.go          # 设备指标历史
├── scripts.go         # 脚本模型 + 执行记录
├── tenant.go           # 租户模型 + CA 证书
└── upgrade_log.go      # 升级日志模型

controller/internal/usp/
├── message_interceptor.go  # USP 消息拦截
└── message_storage.go      # 消息持久化

mtp/adapter/internal/events/usp_handler/
└── async.go           # 设备状态异步回调

utils/
└── firmware-upload/   # 固件文件存储服务（自定义新增）

backend/services/tests (部分服务测试文件)
├── acs/handler_test.go
├── controller/bridge/bridge_test.go
├── controller/cwmp/cwmp_test.go
├── controller/api/pagination_test.go
├── controller/api/handlers_test.go
├── controller/api/script_execution_test.go
├── controller/api/campaign_engine_test.go
├── controller/db/db_integration_test.go
└── usp/usp_utils/utils_test.go
```

### 只在官方版本中的文件

```
mtp/mqtt/internal/listeners/health  # MQTT 健康检查监听器
```

---

## 三、API 路由差异（关键）

### 自定义版本新增的 API 端点

| 方法 | 路径 | 功能 |
|------|------|------|
| GET | `/api/tenants` | 列出所有租户 |
| POST | `/api/tenants` | 创建租户（自动建库+KV） |
| GET | `/api/tenants/{slug}` | 获取租户信息 |
| PUT | `/api/tenants/{slug}` | 更新租户 |
| DELETE | `/api/tenants/{slug}` | 删除租户（完整清理） |
| GET | `/api/tenants/{slug}/device-password` | 获取设备共享密码 |
| PUT | `/api/tenants/{slug}/device-password` | 设置设备共享密码 |
| GET | `/api/tenants/{slug}/ca-certs` | 列出 CA 证书 |
| POST | `/api/tenants/{slug}/ca-certs` | 添加 CA 证书 |
| DELETE | `/api/tenants/{slug}/ca-certs/{certId}` | 删除 CA 证书 |
| GET | `/api/tenants/{slug}/users` | 列出租户用户 |
| POST | `/api/tenants/{slug}/users` | 注册用户 |
| DELETE | `/api/tenants/{slug}/users/{user}` | 删除用户 |
| PUT | `/api/tenants/{slug}/users/password` | 修改自身密码 |
| PUT | `/api/tenants/{slug}/users/password/{user}` | 管理员修改他人口令 |
| GET | `/api/tenants/{slug}/firmware` | 列出固件 |
| POST | `/api/tenants/{slug}/firmware` | 上传固件（文件/URL） |
| PUT | `/api/tenants/{slug}/firmware/{id}` | 更新固件 |
| DELETE | `/api/tenants/{slug}/firmware/{id}` | 删除固件 |
| PUT | `/api/tenants/{slug}/firmware/{id}/phase` | 更新固件阶段 |
| GET | `/api/tenants/{slug}/scripts` | 列出脚本 |
| POST | `/api/tenants/{slug}/scripts` | 创建脚本 |
| GET | `/api/tenants/{slug}/scripts/{id}` | 获取脚本详情 |
| PUT | `/api/tenants/{slug}/scripts/{id}` | 更新脚本 |
| DELETE | `/api/tenants/{slug}/scripts/{id}` | 删除脚本 |
| POST | `/api/tenants/{slug}/scripts/{id}/execute/{sn}/{mtp}` | 执行脚本 |
| GET | `/api/tenants/{slug}/scripts/{id}/executions` | 执行历史 |
| GET | `/api/tenants/{slug}/scripts/{id}/executions/{execId}` | 执行详情 |
| GET | `/api/tenants/{slug}/campaigns` | 列出活动 |
| POST | `/api/tenants/{slug}/campaigns` | 创建活动 |
| PUT | `/api/tenants/{slug}/campaigns/{id}` | 更新活动 |
| DELETE | `/api/tenants/{slug}/campaigns/{id}` | 删除活动 |
| GET | `/api/tenants/{slug}/campaigns/{id}/logs` | 活动升级日志 |
| GET | `/api/tenants/{slug}/mass-actions` | 列出批量操作 |
| POST | `/api/tenants/{slug}/mass-actions/script` | 批量执行脚本 |
| GET | `/api/tenants/{slug}/mass-actions/{id}` | 批量操作详情 |
| POST | `/api/tenants/{slug}/mass-actions/{id}/cancel` | 取消批量操作 |
| GET | `/api/tenants/{slug}/device/{sn}/{mtp}/info` | 设备实时信息 |
| GET | `/api/tenants/{slug}/device/{sn}/{mtp}/interfaces` | 网络接口 |
| GET | `/api/tenants/{slug}/device/{sn}/{mtp}/performance` | 性能数据 |
| GET | `/api/tenants/{slug}/device/{sn}/{mtp}/topology` | 网络拓扑 |
| GET | `/api/tenants/{slug}/device/{sn}/{mtp}/wifi-usp` | USP WiFi 数据 |
| GET | `/api/tenants/{slug}/device/{sn}/cached-info` | 缓存信息（离线） |
| GET | `/api/tenants/{slug}/device/{sn}/history` | 消息历史 |
| DELETE | `/api/tenants/{slug}/device/{sn}/history` | 清空历史 |
| GET | `/api/tenants/{slug}/device/{sn}/metrics` | 指标历史 |
| GET | `/api/tenants/{slug}/device/{sn}/fw-policy` | 升级策略 |
| PUT | `/api/tenants/{slug}/device/{sn}/fw-policy` | 设置升级策略 |
| GET | `/api/tenants/{slug}/device/{sn}/upgrade-logs` | 升级日志 |
| PUT | `/api/tenants/{slug}/device/{sn}/{mtp}/reboot` | 重启设备 |
| PUT | `/api/tenants/{slug}/device/{sn}/{mtp}/factory-reset` | 恢复出厂 |
| PUT | `/api/tenants/{slug}/device/{sn}/{mtp}/restart-agent` | 重启 Agent |

### 官方版本移除/未实现的端点

```
/api/auth/register     → 合并到租户创建流程
/api/auth/password     → 改为 /users/password
/device/{sn}/history   → 官方无此功能
/device/{sn}/metrics   → 官方无此功能
/device/{sn}/fw-policy → 官方无此功能
```

---

## 四、数据库模型差异

### 租户隔离（自定义版本）

官方版本使用单一 MongoDB 数据库，自定义版本每个租户独立：

```
account-mngr 数据库（共享）
└── users, tenants

tenant_<slug>_general 数据库（租户隔离）
├── templates
├── firmware          ← 新增
├── scripts           ← 新增
├── script_executions ← 新增
├── mass_actions      ← 新增
├── device_info
├── campaigns         ← 新增
├── fw_policies       ← 新增
└── upgrade_logs      ← 新增

tenant_<slug>_usp 数据库（租户隔离）
├── messages
├── messages_errors
└── device_metrics    ← 新增
```

### 新增的数据模型

| 模型 | 用途 |
|------|------|
| `Tenant` | 租户信息 + CA 证书 |
| `Firmware` | 固件元数据（SHA256、Phase、FileSize） |
| `Script` | 脚本（含步骤：GET/SET/ADD/DELETE/OPERATE/CONDITION/DELAY） |
| `ScriptExecution` | 脚本执行记录 |
| `Campaign` | 固件升级活动（vendor/model/hw_version 匹配） |
| `MassAction` | 批量操作（脚本执行在多设备） |
| `DeviceFWPolicy` | 单设备升级策略（campaign/skip/manual） |
| `FirmwareUpgradeLog` | 升级历史（状态流转） |
| `DeviceMetric` | 设备指标时序数据 |

---

## 五、中间件与安全差异

### JWT Claims 扩展

自定义版本在 JWT 中增加了 `TenantSlug` 字段，官方版本仅有 `TenantID`。

```go
// 自定义版本
type JWTClaim struct {
    Username   string
    Email      string
    TenantID   string  // MongoDB ObjectID hex
    TenantSlug string  // ← 新增：URL slug
    Level      int     // 0=SuperAdmin, 1=TenantAdmin, 2=Operator
}

// 官方版本
type JWTClaim struct {
    Username string
    Email    string
    TenantID string
    Level    int
}
```

### 新增中间件层

```
AuthMiddleware → TenantMiddleware → Handler
                    ↑
              验证租户 slug 存在于数据库且状态为 active
```

### 设备认证（新增）

```
NATS KeyValue Bucket: devices-auth-<tenant_slug>
├── __tenant_password__  (租户共享密码)
└── <device_sn>           (设备独立密码)
```

---

## 六、消息通信差异

### NATS 主题扩展

官方版本 NATS 主题不含租户隔离，自定义版本所有主题都包含 `{tenant_slug}` 前缀：

```
# 官方
device.usp.v1.<sn>
device.cwmp.v1.<sn>

# 自定义（租户隔离）
device.usp.v1.<tenant_slug>.<sn>
device.cwmp.v1.<tenant_slug>.<sn>
```

### 消息拦截（新增）

`message_interceptor.go` 和 `message_storage.go` 实现了 USP 消息的拦截和持久化，将所有进出的消息存储到 `tenant_<slug>_usp.messages` 集合。

---

## 七、关键文件改动统计

| 文件 | 改动行数 | 说明 |
|------|---------|------|
| `controller/internal/api/api.go` | ~123 行 | 大量新增路由 |
| `controller/internal/api/user.go` | ~166 行 | 用户管理重构 |
| `controller/internal/db/db.go` | ~90 行 | 租户 DB 隔离逻辑 |
| `controller/internal/api/device.go` | ~60 行 | 设备管理扩展 |
| `mtp/adapter/internal/reqs/reqs.go` | ~57 行 | 请求处理逻辑扩展 |
| `controller/internal/api/usp.go` | ~43 行 | USP 协议操作 |
| `controller/internal/bridge/bridge.go` | ~22 行 | NATS 桥接扩展 |
| `mtp/adapter/internal/events/events.go` | ~14 行 | 事件处理扩展 |

---

## 八、测试覆盖差异

| 测试文件 | 内容 |
|---------|------|
| `acs/handler_test.go` | ACS 处理器测试 |
| `controller/bridge/bridge_test.go` | NATS 桥接测试 |
| `controller/cwmp/cwmp_test.go` | CWMP 协议测试 |
| `controller/api/handlers_test.go` | API 端点测试 |
| `controller/api/pagination_test.go` | 分页逻辑测试 |
| `controller/api/script_execution_test.go` | 脚本执行引擎测试 |
| `controller/api/campaign_engine_test.go` | 活动引擎测试 |
| `controller/db/db_integration_test.go` | 数据库集成测试 |
| `usp/usp_utils/utils_test.go` | USP 工具函数测试 |
| `mtp/adapter/events/usp_handler/info_test.go` | USP 处理器测试 |

---

## 九、自定义新增服务：firmware-upload

```
utils/firmware-upload/
├── cmd/upload-server/main.go   # 文件上传服务（port 8006）
├── internal/handlers/upload.go
├── internal/handlers/delete.go
├── internal/storage/            # 文件存储抽象
│   ├── filesystem.go          # 本地文件系统
│   └── storage.go             # 接口定义
└── build/Dockerfile
```

功能：
- 接收固件文件 multipart 上传
- 按租户 slug 隔离存储路径
- 提供文件下载服务（`/firmwares/{tenant}/{filename}`）
- 提供删除接口（供固件删除时级联清理）

---

## 十、升级策略（自定义版本独有）

```
Campaign（活动）
  ↓ 自动匹配 vendor+model+hw_version
Device (设备)
  ↓ 读取 fw_policy
  ├── campaign → 跟随活动升级
  ├── skip     → 跳过所有升级
  └── manual   → 立即触发指定固件升级

UpgradeLog（升级日志）
  └── 状态：pending → downloading → success/failed
  └── 支持重试（retry_count）
  └── 90 天自动过期（TTL index）
```

---

## 十一、脚本引擎（自定义版本独有）

支持 7 种步骤类型：

| 类型 | 说明 | 参数 |
|------|------|------|
| `GET` | 读取参数路径 | param_paths, max_depth |
| `SET` | 设置参数值 | obj_path, param_settings |
| `ADD` | 创建对象 | obj_path, param_settings |
| `DELETE` | 删除对象 | obj_paths |
| `OPERATE` | 执行操作 | command, command_key, input_args |
| `CONDITION` | 条件分支 | condition (==/!=/>/</contains/exists), on_true/on_false |
| `DELAY` | 延时 | duration_ms |

支持功能：
- 变量插值 `{{VAR_NAME}}`
- 保存结果 `save_result_as`
- 错误处理 `on_error`（abort/continue/skip_to）
- 无限循环检测（最大 500 次迭代）
- 执行历史持久化（30 天 TTL）