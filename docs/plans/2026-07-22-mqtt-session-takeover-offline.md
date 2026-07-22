# MQTT Session Takeover: Ping OK but Platform Offline

> **现象**：设备 MQTT keepalive（Ping Request/Response）正常，平台 UI / Mongo 却显示 Offline。  
> **环境**：GCP Indonesia prod (`34.143.170.232`)，tenant `telkomsel`，设备 `081074484848`（ATEL FH220 `1.0.0-t6`）。  
> **日期**：2026-07-22。  
> **当前判断**：不是网络不通；现场时序与同 ClientID 踢连（session takeover）后旧连接延迟 `OnDisconnect` 高度吻合。现有日志未记录 MQTT ClientID、remote 和 StopCause，因此修复会同时补充诊断日志，用后续现场数据完成最终定性。

## 1. 现场证据

### 1.1 Wireshark

- 源 `10.0.0.48` → 目标 `34.143.170.232:1883`
- 约每 60s 一对 MQTT Ping Request / Ping Response
- 捕获窗口内几乎无 PUBLISH/SUBSCRIBE（仅 keepalive）

说明：TCP/MQTT 会话仍在；Ping 只能证明 broker 层存活，不能证明平台 presence 正确。

### 1.2 mqtt 日志（节选）

```
06:39:51 auth: tenant password matched for telkomsel/081074484848
06:39:52 auth: tenant password matched for telkomsel/081074484848
06:39:52 new device: tenant=telkomsel device=081074484848

06:52:33 auth: tenant password matched for telkomsel/081074484848
06:52:34 auth: tenant password matched for telkomsel/081074484848
06:52:34 new device: tenant=telkomsel device=081074484848

07:24:34 auth: tenant password matched for telkomsel/081074484848
07:24:34 new device: tenant=telkomsel device=081074484848
```

要点：

- 认证走 tenant 共享密码（`devices-auth-telkomsel` 无 per-device key）
- 多次出现约 1 秒内双 CONNECT（同设备快速重连 / 双连接）
- `new device` = `OnSubscribed` 命中 `.../agent/<sn>`

### 1.3 adapter 状态时间线

| UTC | 事件 | 含义 |
|-----|------|------|
| 06:39:52 | online + CreateDevice | 首次入库 Online |
| 06:51:xx | controller USP 有 response | 短暂可用 |
| 06:52:34 | online + info replace | 重连后再次 Online |
| **06:52:43** | **offline** | Online 后约 9s 被写成 Offline |
| 06:52:43–07:24:33 | 无新 online | 平台 Offline；抓包仍可 Ping |
| 07:24:33 | offline | 又一次踢连的旧会话 disconnect |
| 07:24:34 | online + info | 顺序正确，最终恢复 |
| 07:27 | Mongo `status=2 mqtt=2` | 调查时已 Online |

同机 `081074000888` 同期也有频繁 online/offline，属同类重连抖动。

### 1.4 附带噪声（非本次主因）

```
REJECTED: ... expected NoSessionContext record, got MQTTConnect
```

设备上线时先发 `MQTTConnect` record 到 info subject，adapter 拒绝；随后真正的 GetResp 才成功建档。这是 info 路径噪声，不是卡 Offline 的根因。

## 2. 状态机现状

### 2.1 Online 如何产生

文件：`backend/services/mtp/mqtt/internal/listeners/mqtt/hook.go`

- `OnSubscribed`：订阅 `oktopus/usp/v1/<tenant>/agent/<device>` 时发布 status payload `"1"`
- adapter `deviceOnline`：**只打日志 + 向设备发 Get DeviceInfo**，不直接写 Mongo Online
- info GetResp 成功后 `CreateDevice` 才把 `Mqtt=Online` / `Status=Online` 写入 DB

### 2.2 Offline 如何产生

同一 hook 的 `OnDisconnect`：只要 client 上存了 tenant/device user properties，就无条件发布 status `"0"`。

adapter `deviceOffline` → 立刻 `UpdateStatus(Offline)`。

### 2.3 Online / Offline 不对称

| 事件 | DB 行为 |
|------|---------|
| status=`1` | 不写 Online；依赖后续 info |
| info GetResp | `CreateDevice` 写 Online |
| status=`0` | **立刻** `UpdateStatus(Offline)` |

因此后到的一条 `status=0` 会稳定盖掉已写入的 Online，直到下一次完整 subscribe + info。

## 3. 高度疑似根因：Session Takeover 竞态

依赖：`github.com/mochi-co/mqtt/v2 v2.2.16`

同 ClientID 重连时 broker 行为（`attachClient` / `inheritClientSession`）：

1. `DisconnectClient(existing, ErrSessionTakenOver)`
2. 新连接继续 `OnSessionEstablished` → 设备 `SUBSCRIBE` → 我们的 `OnSubscribed` 发 `status=1`
3. 旧连接的 read loop 稍后才结束 → **仍会调用** `hooks.OnDisconnect`
4. 旧 socket 关闭可延迟数秒（mochi 社区亦有 delayed disconnect / takeover 相关 issue）

我们的 `OnDisconnect` **没有**判断 takeover，也没有输出足够的连接身份和断开原因：

```go
func (h *MyHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
    tenant, device := getTenantAndDevice(cl)
    if device == "" {
        return
    }
    statusTopic := "oktopus/usp/v1/" + tenant + "/status/" + device
    _ = server.Publish(statusTopic, []byte("0"), false, 1)
}
```

对比：mochi 自带 redis storage hook 在 `StopCause() == ErrSessionTakenOver` 时直接 return；我们的 presence hook 没有等价守卫。

需要注意：takeover 按 MQTT ClientID 判断，不按 username 或设备 SN 判断。当前日志只证明同一设备身份发生了快速重复认证，尚未直接证明两条连接使用相同 ClientID。修复后的日志必须包含 `cl.ID`、`cl.Net.Remote`、`cl.StopCause()`、`expire` 和 hook 收到的 read error。

### 3.1 失败序（卡 Offline）

```
旧连接被踢 → 新连接 SUBSCRIBE → status=1 → info → DB Online
                …数秒后…
旧连接 OnDisconnect → status=0 → DB Offline   // 新会话仍在 ping
```

对应现场：`06:52:34` online → `06:52:43` offline。

### 3.2 恢复序（碰巧正确）

```
旧连接 OnDisconnect → status=0
新连接 SUBSCRIBE → status=1 → info → DB Online
```

对应现场：`07:24:33` offline → `07:24:34` online。

平台显示完全取决于 **最后一条 status 的先后**，与 MQTT 是否仍能 Ping 无关。

## 4. 相关代码问题（次要）

### 4.1 服务端改写 Will 无效且有害

`OnSubscribed` 中：

```go
cl.Properties.Will = mqtt.Will{
    Qos: 1, TopicName: statusTopic, Payload: []byte("0"), Retain: false,
}
```

mochi `sendLWT` 要求 `Will.Flag != 0` 才发送；这里整结构体重写且 Flag 仍为 0，LWT 不会走该路径。同时会抹掉客户端 CONNECT 自带的 Will。

删除整结构体覆盖后还需处理一个边界：如果客户端 CONNECT 原本携带的 Will 正是当前设备的 Oktopus status topic，它会在 takeover 中绕过 `OnDisconnect` 守卫发送陈旧 Offline。因此修复保留非 status Will，但在订阅建立、确定 tenant/device 后关闭同一 status topic 的 Will，由经过 takeover 校验的 `OnDisconnect` 统一负责 presence Offline。

### 4.2 `UpdateStatus` 日志误导

`adapter/internal/db/status.go` 无论写 Online/Offline 都打印 `"%s is now offline."`，排障时容易误读。

## 5. 当前修复方案

按优先级：

1. **补充可归因日志**  
   `MyHook.OnDisconnect` 记录 ClientID、remote、tenant、device、StopCause、takeover 和 expire；takeover 跳过 Offline 时也记录明确原因。

2. **忽略被接管旧会话的 Offline**  
   项目锁定的 mochi mqtt `v2.2.16` 没有公开 `Client.IsTakenOver()`；使用该版本可用且其 storage hook 采用的 StopCause 条件，不检查 `OnDisconnect` 的 `err` 参数：

   ```go
   if cl.StopCause() == packets.ErrSessionTakenOver {
       return
   }
   ```

   `err` 可能只是 socket read error；`StopCause()` 才是 broker 保存的停止原因。

3. **删除服务端伪造 Will，并隔离 status Will**  
   删除 `OnSubscribed` 对 `cl.Properties.Will` 整体的覆盖。不能通过设置 `Will.Flag` 修复：takeover 时旧连接的 status Will 可能形成第二条绕过 `OnDisconnect` 守卫的 Offline。保留客户端非 status Will；如果原始 Will topic 等于当前设备的 Oktopus status topic，则清除其 Flag，Offline 统一由经过 takeover 校验的 `OnDisconnect` 发布。

4. **自动化回归测试（先测试、后实现）**  
   - takeover StopCause：不允许发布 Offline
   - 普通断开：允许发布 Offline
   - `OnSubscribed`：不得覆盖客户端非 status Will
   - `OnSubscribed`：必须禁用当前设备的 Oktopus status Will
   - 后续集成测试：同 ClientID A/B 接管，等待 A 的 `OnDisconnect` 完成后最终仍为 Online；正常断开 B 后只产生一次 Offline

5. **本次不改 adapter 状态机**  
   `deviceOnline` 延迟到 info GetResp 才写 Online 是既有设计，不是竞态源头；在 adapter 增加 broker 活连接判断也缺少可用连接注册表。本次保持范围在 MQTT presence hook。

6. **后续独立清理**  
   修正 `UpdateStatus` 无论状态都打印 `is now offline` 的误导日志，不与本次行为修复混在一起。

### 5.1 实施记录（2026-07-22）

已实施：

- `MyHook.OnDisconnect` 输出 ClientID、remote、StopCause、expire 和 read error
- `StopCause() == packets.ErrSessionTakenOver` 时跳过 Offline
- 删除 `OnSubscribed` 中服务端伪造 Will；保留非 status Will，禁用当前设备 status Will
- 新增 takeover、普通断开、保留非 status Will 和禁用 status Will 的单元测试
- 新增 Compose `test-mqtt` 服务及 GitLab `test:unit:mqtt` job

版本验证：编译确认 mochi mqtt `v2.2.16` 没有公开 `Client.IsTakenOver()`，因此没有采用该 API。后续仍需在现场日志中确认异常断开记录的 `cause=session taken over`，并补充真实双连接 broker 集成测试。

## 6. 涉及文件

| 文件 | 角色 |
|------|------|
| `backend/services/mtp/mqtt/internal/listeners/mqtt/hook.go` | Online/Offline 发布；根因所在 |
| `backend/services/mtp/adapter/internal/events/usp_handler/status.go` | status `0/1` → Offline / 触发 info |
| `backend/services/mtp/adapter/internal/events/usp_handler/info.go` | GetResp → `CreateDevice(Online)` |
| `backend/services/mtp/adapter/internal/db/status.go` | `UpdateStatus` |
| `backend/services/mtp/adapter/internal/db/device.go` | `CreateDevice` upsert |

## 7. 运维排障速查

怀疑「能 Ping、平台 Offline」时：

```bash
# 设备当前 DB 状态（Online=2）
docker exec mongo_usp mongosh --quiet --eval \
  'db.getSiblingDB("adapter").devices.findOne({sn:"<SN>"},{sn:1,status:1,mqtt:1,tenantid:1})'

# mqtt：认证 / new device（= subscribe）
docker logs --since 2h mqtt 2>&1 | grep '<SN>'

# adapter：online/offline 先后
docker logs --since 2h adapter 2>&1 | grep '<SN>'
```

判断要点：

- 有 Ping + 有近期 `new device` + 随后单独一条 offline、且无更新的 online → 高度疑似 takeover 竞态  
- 完全无 auth / 无 `new device` → 更像设备未 subscribe agent topic，或连错环境
