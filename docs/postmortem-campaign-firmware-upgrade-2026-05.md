# Oktopus Campaign 固件升级故障排查 — 技术总结与反思

> **日期**：2026-05-19 ~ 2026-05-20  
> **环境**：tenant `sei`，MQTT/USP 设备，生产 VM `gcp-america-sei-tr369`  
> **状态**：已解决（controller 镜像更新至含修复版本后正常）

---

## 1. 背景与现象

### 1.1 业务诉求

- 使用 **Firmware Campaign**（非 Mass Actions）对在线设备做批量固件升级。
- 单设备 **Manual** 升级可用；Campaign（含 anytime / 时间窗口）长期不触发或 batch 失败。

### 1.2 典型日志现象（按阶段）

| 阶段 | 日志 | 含义 |
|------|------|------|
| A | `nats: no responders available for request` + `campaign_engine.go:251` | Controller 向错误 NATS subject 发请求 |
| B | `2 device(s) match campaign ...` + `no eligible devices` | 硬件匹配成功，但 eligibility 过滤掉全部设备 |
| C | 启动日志 `campaign_engine.go:57`、`api.go:196` | 生产仍在跑**旧 controller 二进制** |

---

## 2. 根因分析（共四类）

### 2.1 NATS Subject 错误（Batch 完全失败）

**原因**：`RunCampaignBatch` → `getMatchingOnlineDevices` 向  
`adapter.usp.v1.<tenant>.devices` 发 Request-Reply，而 adapter 仅订阅  
`adapter.usp.v1.<tenant>.devices.retrieve`。

**修复**（commit `27c3c83`）：统一走 `getDevicesNoHTTP(filter, nc, tenantSlug)`，subject 为 `devices.retrieve`，并传入 `status` / `limit` / `skip` 分页参数。

**教训**：Controller 与 Adapter 的 NATS 契约应以 adapter `reqs.go` 订阅列表为唯一真相；batch 与 HTTP API 应复用同一查询路径。

---

### 2.2 历史 `success` 升级日志误挡重试（Batch 误报 no eligible）

**原因**：设备曾成功升到固件 A，后版本回落或 Campaign 改指向固件 B；batch 仍因 `upgrade_logs.status == success` 跳过，未比对当前 `device.Version`。

**数据例证**：

- Campaign 目标：`V4.0.0-2604301844`
- Adapter 设备版本：`V4.0.0-260430`
- `upgrade_logs` 中对 `1844` 的 `success` 记录仍存在 → 旧逻辑不再下发升级

**修复**（commit `95d10b3`）：

- 是否跳过以 **`device.Version == fw.BuildVersion`** 为准；
- `pending` / `downloading` / `failed`+重试次数 仍按原逻辑处理；
- batch 增加 `skip` 原因日志，便于运维判读。

**教训**：升级日志表示「历史事件」，不能替代「当前版本」状态机。

---

### 2.3 版本已一致（预期不升级）

**原因**：Campaign 绑定固件 `build_version` 与设备 `version` 字符串完全一致时，`handleCampaignPolicy` / batch 均会跳过。

**教训**：配置 Campaign 前核对 adapter 中设备 `version` 与固件 `build_version`；这不是 bug。

---

### 2.4 生产镜像未真正更新（部署问题）

**原因**：仅 `docker load` 未 `--force-recreate`，或构建机未 `git pull`，容器仍跑旧二进制。

**鉴别特征（新 vs 旧）**：

| 检查项 | 旧镜像 | 新镜像 |
|--------|--------|--------|
| 启动 | 无 `Provisioned databases for tenant` | 有（`MigrateAllTenantIndexes`） |
| 行号 | `campaign_engine.go:57`，失败在 `:251` | 订阅在 `:59`，失败在 `:260` |
| API | `api.go:196` | `api.go:198` |
| 二进制 | `grep -aq "devices.retrieve" /controller` 失败 | 成功 |

**正确生产步骤（仅 controller）**：

```bash
./build.sh controller          # 构建机
docker save oktopusp/controller:latest -o controller.tar
scp controller.tar prod:~/compose/
docker load -i controller.tar
docker compose stop controller && docker compose rm -f controller
docker compose up -d controller
```

---

## 3. 其它关联问题（非本次主因）

| 项 | 说明 |
|----|------|
| `fw-policy=manual` | 曾手动升级的设备会走 `handleManualPolicy`，不走 Campaign；测试需改 policy 或换未手动升过的设备 |
| 硬件大小写 | `GetCampaignByHardware` 与唯一索引改为 collation + TrimSpace（`27c3c83`） |
| Controller 重启 | 已在线设备不会自动 batch，需重连 MQTT 或保存 Campaign 触发 batch |
| `message_interceptor: Record does not have NoSessionContext` | 连接类 USP 记录，与 Campaign batch 无直接关系 |

---

## 4. 代码改动清单（Controller 为主）

| 文件 | 改动要点 |
|------|----------|
| `campaign_engine.go` | `devices.retrieve`、分页扫描、eligibility 修复、skip 日志 |
| `utils.go` | `getDevicesNoHTTP` |
| `db/campaigns.go` | collation 查询、`CreateCampaign` TrimSpace |
| `db/tenant.go` | `campaigns_hardware_ci` 索引、启动迁移 |
| `cmd/controller/main.go` | `MigrateAllTenantIndexes` |
| `campaign_engine_unit_test.go` / `db_integration_test.go` | 硬件匹配与 collation 测试 |

**无需为本次问题升级**：`adapter`、`mqtt`、`mqtt-adapter`（adapter 早已监听 `devices.retrieve`）。

---

## 5. 验证清单

- [ ] 构建机：`grep -aq "devices.retrieve" /controller` 在镜像内为 OK  
- [ ] 生产启动日志含 `Provisioned databases for tenant "sei"`  
- [ ] 行号为 `campaign_engine.go:59`、`api.go:198`  
- [ ] 保存 Campaign 后出现 `match campaign ... hardware`，且无 `no responders`  
- [ ] 目标版本高于设备当前版本时出现 `starting batch upgrade` / `triggering firmware upgrade`  
- [ ] `upgrade_logs` 有新记录且状态流转正常  

---

## 6. 反思与改进建议

### 6.1 可观测性

- Campaign batch 应对每台被 skip 的设备打印原因（已部分实现）。
- 建议在构建中嵌入 `git commit` / 版本号，启动时打印 `controller version=...`，避免靠行号猜版本。

### 6.2 部署

- 内网生产标准流程：**build → save 单服务 tar → load → rm + up**，文档中强调勿只 load 不重建。
- 提供 `scripts/verify-controller-image.sh`（检查 `devices.retrieve` 字符串）作为发布门禁。

### 6.3 产品行为

- Controller 启动时对**已启用 Campaign** 可选执行一次 batch（需评估负载）。
- UI 提示：设备版本已与 Campaign 目标一致 / 存在 manual policy 时为何不升级。

### 6.4 测试

- 集成测试：mock adapter 订阅 `devices.retrieve`，断言 batch 能收到设备列表。
- 场景测试：success 日志存在但 `device.Version` 已变化时应再次触发升级。

---

## 7. 相关参考

- NATS subject 约定：`CLAUDE.md` — Device / Adapter subjects  
- Adapter 请求监听：`backend/services/mtp/adapter/internal/reqs/reqs.go`  
- Campaign 引擎：`backend/services/controller/internal/api/campaign_engine.go`  
- 离线更新 controller：`deploy/compose/package.sh` 或单镜像 `docker save oktopusp/controller`

---

## 8. 标签（Notion 用）

`Oktopus` `Campaign` `Firmware` `NATS` `USP` `MQTT` `Postmortem` `Controller` `生产部署`
