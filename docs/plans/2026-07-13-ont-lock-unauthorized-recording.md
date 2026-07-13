# ONT Lock: Unauthorized Devices 记录与自清理

> **背景**：`auto_lock_enabled=true` 时，未授权设备决策为 LOCKED，而 `RecordUnauthorizedDevice` 仅在 PENDING 时触发，导致 Unauthorized 列表永远为空。运维无法在 UI 上看到哪些设备处于锁定状态，也无法通过 Unauthorized 标签页将其批量加入白名单。

## 目标行为

设备上线 -> 未授权 -> LOCKED（执行 Lock=1）-> **同时**出现在 Unauthorized 列表 -> 运维确认后加白名单 -> 设备解锁 -> **自动从列表消失**。

## 改动点（6 项）

### 1. 扩大记录条件 + 状态守卫

文件：`backend/services/controller/internal/api/lock_engine.go`，`evaluateAndMaybeCommand` 方法（约 272 行）

当前：
```go
if decision.Status == db.LockStatusPending {
    _ = tdb.RecordUnauthorizedDevice(...)
}
```

改为：当 `Reason == UNAUTHORIZED` 时记录，但加状态守卫避免写放大（IP 轮询每 60 秒触发一次 evaluate）：
```go
shouldRecord := decision.Reason == db.LockReasonUnauthorized
if shouldRecord && found && prev.LastIP == reportedIP && prev.LastStatus == decision.Status {
    shouldRecord = false
}
if shouldRecord {
    _ = tdb.RecordUnauthorizedDevice(...)
}
```

### 2. evaluate 自清理兜底

同一方法内，当评估结果为已授权、且设备之前不是 UNLOCKED 时，删除 unauthorized 条目并写审计：

```go
if found && prev.LastStatus != db.LockStatusUnlocked && decision.Reason == db.LockReasonAuthorized {
    _, _ = tdb.DeleteUnauthorizedDevices(ctx, []string{device.SN})
    a.recordLockAudit(ctx, tdb, tenantSlug, db.LockAuditLog{
        SN:     device.SN,
        Action: "unauthorized_auto_resolved",
        Details: bson.M{"source": "evaluate_ip_authorized"},
    })
}
```

状态守卫（`prev.LastStatus != UNLOCKED`）确保稳态运行时不产生多余写。

### 3. `upsertLockPolicy` 补全第三条路径

文件：`backend/services/controller/internal/api/lock.go`，`upsertLockPolicy` 方法

当前：创建/更新 WHITELIST 策略后只追发命令，不清理 unauthorized 条目。
补充：当策略类型为 WHITELIST 时，删除该 SN 的 unauthorized 条目 + 写审计（action=`unauthorized_auto_resolved`）。

### 4. `saveLockConfig` 关 master 时清空

文件：`backend/services/controller/internal/api/lock.go`，`saveLockConfig` 方法

当 `master_enabled` 从 true -> false 时，`DeleteMany({})` 清空整个 unauthorized 集合 + 写审计（action=`unauthorized_cleared_master_disabled`）。

### 5. 审计留痕

所有删除 unauthorized 条目的点统一写 `lock_audit_logs`：

| 触发点 | action | 现有/新增 |
|---|---|---|
| `batchWhitelistFromUnauthorized` | `unauthorized_batch_whitelist` | 已有 |
| `batchDeleteUnauthorizedDevices` | `unauthorized_dismiss` | 已有 |
| `upsertLockPolicy`（加白名单） | `unauthorized_auto_resolved` | 新增 |
| evaluate 兜底（IP 漂入 CIDR） | `unauthorized_auto_resolved` | 新增 |
| `saveLockConfig`（关 master） | `unauthorized_cleared_master_disabled` | 新增 |

### 6. 数据流

```
设备上线 (未授权)
  -> evaluate -> LOCKED + UNAUTHORIZED
  -> 状态/IP 变化 -> 写入 lock_unauthorized_devices (状态守卫)
  -> 运维在 Unauthorized 标签页看到设备
  -> 运维加白名单 (3 条路径: Unauthorized 批量 / Policy 直接 / IP 漂入 CIDR)
  -> evaluate -> UNLOCKED + AUTHORIZED -> 自清理删除条目 + 审计
  -> Unauthorized 标签页设备消失
  -> Audit 标签页可追溯
```

## 不改动的部分

- `RecordUnauthorizedDevice` 的 upsert 逻辑不变（仍然是 $set + $setOnInsert）
- `EvaluateLockDecision` 决策逻辑不变（四条出口路径不变）
- 前端无需改动（Unauthorized 列表 API 不变，只是数据来源变完整了）
