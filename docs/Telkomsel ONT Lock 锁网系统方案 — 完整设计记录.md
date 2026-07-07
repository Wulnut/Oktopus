# Telkomsel ONT Lock 锁网系统方案 — 完整设计记录

> **实现说明（一期）**：一期仅保留**白名单准入**，黑名单功能及相关互斥逻辑已移除。不在白名单的设备默认 LOCKED，满足 SN + IP 双白名单条件才 UNLOCKED。下文保留原始设计记录的完整性，第八节「黑名单强控与动态追杀」在一期未启用。

**定位** — 本页是平台侧的完整方案设计记录，配套风险分析页见文末「来源」。核心主张：基于开源 **Oktopus（OktopUSP）** 平台，采用「**标准协议扩展  旁路微服务  存储融合  单表互斥**」的解耦架构。**双白名单校验**：SN 在白名单 **且** 设备上报 IP 落在授权运营商网段 → UNLOCKED；黑名单 → LOCKED；任一不满足 → LOCKED。本文档仅覆盖**平台侧**职责；ONT 侧实现由固件团队负责。

# 一、设计原则

- **高内聚低耦合**：边缘适配  核心服务内聚，不造独立烟囱。
- **不污染开源核心**：Oktopus 标准协议核心保持原生，便于跟进上游；锁网逻辑以插件 / 边车微服务形式外挂。
- **物理隔离适配 ATP 约束**：用网关层收敛起运营商指定端口，内部走标准端口。
- **黑白名单合并为一个准入控制模块**：底层单表  主键互斥，上层做覆盖提示。
- **平台 / 设备职责分离**：本文档只描述平台侧行为，ONT 侧执行由固件团队负责。

# 二、整体拓扑

控制流已确认走标准 **CWMP（TR069）**，复用 Oktopus 北向 API 下发 SetParameterValues；MQTT（TR369）仅作在线设备的唤醒 / 实时推送辅助通道，不承担主控指令。

```mermid
flowchart TD
    ONT["ONT 终端"]
    ONT -->|"HTTPS 30443/5443 / MQTT 31770~31794"| GW["SLB / Nginx 网关<br>端口映射 + 转发"]
    GW -->|"HTTPS 转发内部 CWMP"| CORE["Oktopus Core<br>TR-069 / TR-369 标准"]
    GW -->|"MQTT 转发内部 USP over MQTT"| CORE
    CORE -. "设备上线事件" .-> LOCK["oktopus-lock-handler<br>Go 旁路微服务"]
    LOCK <-->|"IP 网段 + SN 双白名单"| REDIS[("Redis 热缓存<br>黑白名单 SN Set / Bloom")]
    LOCK <-->|"读写策略"| PG[("PostgreSQL<br>device_lock_policy")]
    LOCK -->|"下发 Set / SetParameterValues"| CORE
    CORE -->|"指令经标准协议"| ONT
    LOCK -->|"上报 / 回执日志"| KAFKA[["Kafka"]]
    KAFKA --> GP[("Greenplum 日志 / 审计")]
    LOCK <-->|"北向 API"| BSS["运营商 OSS / BSS<br>Web UI"]
```



# 三、接入层：网关与端口适配



## 端口与协议适配（对齐 ATP）

- **HTTPS**：SLB / Nginx 监听 `30443` / `5443`，转发到 Oktopus 内部标准 CWMP 端口（默认 7547 或自定义）。
- **MQTT**：前置网关开放 `31770~31794`，映射到内部 MQTT Broker（可复用 Oktopus 底层 Mochi MQTT 或独立 Mosquitto）。



## IP 网段校验数据来源

- IP 取自 **TR069 / TR369 数据模型中设备上报的 WAN IP**（Oktopus 在标准设备管理流程中已采集），旁路服务直接从设备表读取，**无需 patch Oktopus**。
- 校验方式：IP 白名单以 CIDR 网段录入，判断设备上报 IP 是否命中任一授权网段。
- 关键前提：目标网络需向 ONT 分配可路由 WAN IP；若部署 CGNAT 导致上报私网 IP，网段校验会失效（见风险页 P12）。



# 四、协议与数据模型（厂商自定义节点）



## 节点设计

- **TR069 节点**：`Device.X_TELKOMSEL_OntLock.`
- **TR369 节点**：`Device.X_TELKOMSEL_OntLock.`

对象下只保留 **两个参数**：一个布尔锁控、一个只读 WAN IP 采集。

## 参数设计


| 参数            | 读写 | 类型    | 说明                                                                                       |
| --------------- | ---- | ------- | ------------------------------------------------------------------------------------------ |
| `Lock`          | 读写 | Boolean | `1` = 锁定设备；`0` = 解锁设备。平台通过 `SetParameterValues` 下发。                       |
| `InternetWanIP` | 只读 | String  | 设备当前 WAN IP（Internet WAN IP）。平台通过 `GetParameterValues` 采集，用于 IP 网段校验。 |




## 指令下发与状态确认（平台侧行为）

云端锁 / 解 = Oktopus 通过标准 **CWMP（TR069）******`SetParameterValues` 下发 `Lock`（布尔值），并等待设备回执。`InternetWanIP` 由平台在设备 Inform / 被唤醒时通过 `GetParameterValues` 采集。TR369（MQTT）仅用于在线设备的实时唤醒 / 推送辅助，不承担主控。

# 五、云端架构（基于 Oktopus）



## 1 Oktopus 核心（保持原生）

负责标准 TR069 / TR369 协议解析、通道维持、设备主表维护、业务配置下发。

## 2 旁路微服务 oktopuslockhandler（Go）

捕获设备上线、承载锁网业务逻辑。接入方式：

- **方案 A（无侵入，首选）**：监听 Kafka 设备状态 Topic 或 PG `LISTEN/NOTIFY`，新设备 Inform 即触发校验。
- **方案 B（轻度侵入）**：在 Oktopus 设备上线 / 鉴权代码处预留 Webhook / Plugin 接口。



## 3 双条件校验引擎（SN  IP 网段）

```go
func EvaluateDeviceStatus(sn, reportedIp string) string {
    // 1. 黑名单一票否决
    if isBlacklisted(sn) {
        return "LOCKED"   // 下发 SetParameterValues(Lock, 1)
    }
    // 2. SN + IP 双白名单
    if isSnInWhitelist(sn) && isIpInAllowedSegment(reportedIp) {
        return "UNLOCKED" // 下发 SetParameterValues(Lock, 0)
    }
    // 3. 默认锁定
    return "LOCKED"       // 下发 SetParameterValues(Lock, 1)
}
// reportedIp 取自 GetParameterValues("Device.X_TELKOMSEL_OntLock.InternetWanIP")
```



## 4 指令下发可靠性与幂等

- 下发 LOCK / UNLOCK 生成全局唯一 `CommandID`。
- 未在超时（如 30s）内收到 Ack → 标记「待重试」→ 后台定时任务持续重试。



# 六、存储与中间件整合



## PostgreSQL（设备与配置拓扑）

在 Oktopus 的 PG 中追加锁网扩展表，与设备主表通过 SN 关联。

## Redis（热缓存，防穿透）

- 激活的黑白名单 SN 缓存在 Redis Set 或 Bloom Filter 中。
- Bloom 只做初筛，命中后回查精确 Set / PG 二次确认（防假阳性误锁）。



## Kafka（异步解耦）

ONT 上报的注册、心跳、回执先入 Kafka，由消费者异步处理校验、更新数据库。

## Greenplum（性能与日志）

承接 Kafka 抽取的流水日志，供前端查询指令下发历史与审计（用途边界待确认）。

# 七、准入控制与互斥设计（黑白名单合并）

> **一期注**：一期已移除黑名单，`device_lock_policy` 表仅保留 WHITELIST 记录。同一 SN 的 upsert 仍然覆盖更新。状态机简化为：不在白名单（默认 LOCKED）→ 加入白名单（UNLOCKED）→ 移出白名单（回落 LOCKED）。



## 单表  主键互斥

```sql
CREATE TABLE device_lock_policy (
    sn              VARCHAR(64) PRIMARY KEY,
    policy_type     VARCHAR(20) NOT NULL,   -- WHITELIST / BLACKLIST
    allowed_ip_range VARCHAR(50),            -- 仅 WHITELIST 有效，授权运营商网段 CIDR
    reason_code     VARCHAR(32),             -- 仅 BLACKLIST 有效
    description     TEXT,
    operator_id     VARCHAR(64),
    status          BOOLEAN DEFAULT TRUE,    -- 是否激活
    created_at      TIMESTAMP DEFAULT NOW(),
    updated_at      TIMESTAMP DEFAULT NOW()
);
```

写入用 `ON CONFLICT (sn) DO UPDATE`，状态瞬时切换。

## 设备生命周期状态机

```mermid
stateDiagram-v2
    [*] --> Unknown
    Unknown --> Whitelist: 正常出库 / 开户
    Whitelist --> Blacklist: 欠费 / 报失 / 流失
    Blacklist --> Whitelist: 补缴 / 洗白
    Whitelist --> [*]
    Blacklist --> [*]
    state "未知 (默认 LOCKED)" as Unknown
    state "白名单 (UNLOCK)" as Whitelist
    state "黑名单 (LOCKED)" as Blacklist
```



## 前端防呆交互

- **单条覆盖提示**：把白名单设备拉黑时弹窗确认。
- **批量冲突策略（默认策略 2）**：严格报错抛弃 / 强制覆盖（推荐）/ 同批次自相矛盾判无效。



# 八、黑名单强控与动态追杀（一期未启用）

> **一期注**：以下黑名单逻辑在一期未实现。当前模型为默认拒绝——设备不在白名单即 LOCKED，等效于「封禁」。如需后续恢复主动黑名单能力，`PolicyType` 字段已保留扩展点。



## 黑名单 = 最高优先级（一票否决）

命中黑名单即无条件、即时下发 LOCKED。

## 场景 A：静态拦截（上线 / 重启）

设备 Inform / Connect → lockhandler 查黑名单 → Set LockStatus=LOCKED。

## 场景 B：动态追杀（当前在线）

1. 触发内部事件流；
2. 查 Oktopus 当前在线设备表；
3. 对在线设备立即主动推送：**TR069**：HTTP Connection Request 强制唤醒，随后会话内下发 LOCKED；**TR369**：直接用已建 MQTT 长连接推送修改 LockStatus 的 USP 消息；
4. 平台收到回执。



## 黑名单解除（洗白 / 缴费复机）

解除后重新评估白名单 → 满足则立即下发 `UNLOCKED`。

## 审计日志

所有黑名单操作记录审计流水，定期同步到 Greenplum 供合规审计。

# 九、批量导入与北向 API



## 字段设计

- **白名单**：`SN`（必填）、`Allowed_IP_Range`（必填，授权运营商网段 CIDR）、`Batch_No`（系统生成 / 可选）。
- **黑名单**：`SN`（必填）、`Reason`（欠费 / 报失 / 电商流失…）。



## 异步处理链路



## 北向 RESTful API

- `POST /api/v1/lock/whitelist/batch`
- `POST /api/v1/lock/blacklist/batch`
- `POST /api/v1/lock/blacklist`
- `DELETE /api/v1/lock/blacklist/{sn}`



# 九A、全局配置模块（对齐需求 §24）

两个平台侧全局开关，存储在 PostgreSQL，**不下发到 ONT**。

## 1 锁网总开关（Master Lock Switch）

- **作用**：全局启用 / 关闭整个 ONT Lock 功能。
- **关闭时**：校验引擎对所有设备一律放行，不再下发 `LOCKED` 指令（功能熔断）。
- **默认值**：开启。



## 2 自动锁定开关（Auto Lock Configuration）

- **作用**：开启后，新注册且未通过双白名单校验的设备**自动**下发 `LOCKED`。
- **关闭时**：未授权设备只记录到「未授权设备列表」，不自动锁定，返回 `PENDING` 待人工处理。
- **默认值**：**开启**（ATP 预置条件）。
- **与总开关的关系**：总开关优先级最高。总开关关闭时，自动锁定开关不生效。



## 校验引擎整合

```go
func EvaluateDeviceStatus(sn, reportedIp string) string {
    // 0. 总开关：关闭则一律放行（功能熔断）
    if !masterSwitchEnabled() {
        return "UNLOCKED"
    }
    // 1. 黑名单一票否决
    if isBlacklisted(sn) {
        return "LOCKED"
    }
    // 2. SN + IP 双白名单
    if isSnInWhitelist(sn) && isIpInAllowedSegment(reportedIp) {
        return "UNLOCKED"
    }
    // 3. 自动锁定开关
    if autoLockEnabled() {
        return "LOCKED"
    }
    return "PENDING" // 记录到未授权列表，不自动锁定
}
```



## 北向 API

- `GET /api/v1/lock/config` — 查询当前开关状态
- `PUT /api/v1/lock/config` — 更新开关（需审计日志）



# 十、ATP 负面安全场景的平台侧协同

本章只描述**平台侧的职责与行为**。设备侧能力（持久化、防刷机、防火墙保持等）由固件团队负责，不在本文档范围。


| 测试项    | 场景            | 平台侧行为                                                                                       |
| --------- | --------------- | ------------------------------------------------------------------------------------------------ |
| 541 / 542 | 断网 / 端口封堵 | 心跳超时置离线，**不清除**数据库锁定状态；设备重新注册后重新发起双白名单校验。                   |
| 543       | 恢复出厂        | 识别「恢复出厂后首次注册」的设备，重新做双白名单校验；合法则 Oktopus 标准 ZTP 重新推送业务配置。 |
| 544       | 第三方固件拦截  | 利用 Oktopus 原生固件分发能力，推送**厂商私钥签名**固件。                                        |




# 十一、避坑点（平台侧）

1. **设备上报 IP 可能是 CGNAT 私网地址**：目标网络需向 ONT 分配可路由 WAN IP，否则网段校验失效；需确认 Indihome / RT/RW 地址分配策略。
2. **指令通道形态（已确认）**：控制流走标准 CWMP（TR069 `SetParameterValues`）。MQTT（TR369）仅作为在线设备的唤醒 / 实时推送辅助通道，不承担主控指令。建议仍把指令通道抽象成 `CommandTransport` 接口，便于后续低成本回切。
3. **可用性单点**：平台宕机或停电恢复 → 设备雪崩涌向平台，需补 failopen/close 策略与限流排队。



# 十二、平台侧开发路线图（一期）


| 小组            | 主要任务                                                                                                                                            |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| **后端组 Go**   | 设计 `device_lock_policy` 互斥表；编写 oktopuslockhandler 双条件校验引擎；Excel/CSV 异步解析器 批量写 北向 API；对接 Oktopus 唤醒机制实现动态追杀。 |
| **前端 / UI**   | 在 Oktopus 控制台扩展「设备准入控制」菜单；黑白名单查询 / 单条录入（覆盖警告）/ 批量上传任务看板。                                                  |
| **运维 / 网关** | 配置 SLB/Nginx 端口映射（30443/5443） MQTT 端口区间转发联调。                                                                                       |




# 十三、待会议确认的开放问题

以下问题由原始需求文档与本设计记录  风险分析页对照后得出，按「是否阻断开工」分级。仅保留**平台侧**需确认的问题；ONT 侧能力由固件团队负责，不在本清单。

## 问题总览表


| 等级  | 问题                                                                                 | 需求来源            | 决策方      |
| ----- | ------------------------------------------------------------------------------------ | ------------------- | ----------- |
| ✅ P01 | 去 IP 校验冲突 **已解决：IP 校验恢复**                                               | §背景 / §226        | 云平台      |
| ✅ P02 | ATP 是否接受标准 USP/CWMP 报文 **已解决：使用标准 CWMP 开发，复用 Oktopus 北向 API** | §31 / ATP 端口约束  | 客户 云平台 |
| 🟡 P11 | 存量设备锁网固件如何下发                                                             | §234 标注「需评估」 | 产品        |
| ✅ P12 | 锁网总开关 自动锁定开关 **已解决：云端配置**                                         | §24                 | 云平台      |
| 🟡 P13 | Greenplum 用途边界（性能数据 vs 审计日志）                                           | §252                | 云平台      |
| 🟡 P14 | SLB 双节点互备架构与停电恢复限流                                                     | §217 标注「需评估」 | 运维        |
| 🟢 P21 | 未授权设备「一键批量加白」UI 流程                                                    | §232                | 前端        |




## ✅ 已解决

**P01 IP 校验恢复**：确认需求要求 SN  IP 双白名单。IP 取自 TR069/TR369 设备上报的 WAN IP，按授权运营商网段 CIDR 比对，无需 patch Oktopus。原风险页 P02 解决，新增 P12（CGNAT 私网 IP 风险）。

**P02 ATP 协议形态**：确认使用标准 **CWMP（TR069）**开发。控制流复用 Oktopus 北向 API 下发 `SetParameterValues`，**无需自建独立指令服务**。MQTT（TR369）降级为在线设备的唤醒 / 实时推送辅助通道。原风险页 P01 解决，方向性闸门解除。

**P12 全局开关**：两个开关均为云端配置（见第九 A 节），总开关默认开启，自动锁定开关默认开启（对齐 ATP 预置条件）。

## 🟡 P1：影响范围与工作量

**P11 存量设备锁网固件如何下发**：需求 §234 标注「需评估」。平台侧需确认：一期是否包含「OTA 推送锁网固件」这条工作流？若包含，后端需对接固件分发能力。-> 这个不是我们考虑的范围内的东西

**P13 Greenplum 用途边界**：需求 §252 写「存储设备在线性能数据」，设计里用作审计日志。需确认：Greenplum 只存在线性能数据还是也承担审计日志？如果只存性能数据，审计日志放哪里？

**P14 SLB 双节点互备  停电恢复**：需求 §217 标注「需评估」。需确认：双节点 ActiveStandby 还是 ActiveActive？停电恢复场景的限流与排队策略。  当前第一阶段先不考虑双节点备份

## 🟢 P2：实现细节

**P21 未授权设备「一键批量加白」UI 流程**：需确认：一键加白是否需要二次确认？批量加白后是否立即对在线设备下发解锁？

**会议建议顺序**：P0 已全部解决（方向锁定：标准 CWMP，复用 Oktopus 北向 API）。会议先过 P11（固件下发范围），P13 / P14 并行确认，P2 在方向锁定后决策。

# 十四、来源

整理自「Telkomsel ONT Lock 锁网系统」周末技术讨论。底层平台：开源 Oktopus（OktopUSP，Go）。需求背景：面向印尼 Telkomsel 的 ONT 锁网与 **SN  IP 网段双白名单**管控，需通过 ATP 54x 系列安全验收。本文档仅覆盖平台侧职责。配套文档：[风险与可行性分析](https://seirobotics.feishu.cn/wiki/MSmcwyAzqidPhTklm2Ecqrkjnvh)（飞书同目录）。

> (注：内容由 AI 生成，请谨慎参考）
