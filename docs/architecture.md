# 架构与包布局

本文件定义组件职责、依赖方向与包布局。行为契约见 `docs/spec/spec.md`；接口签名见 `internal/interfaces/**`；决策理由见 `docs/adr/0001`–`0005`。

## 依赖方向

```text
cmd/collect-code
  └── internal/app            （装配与进程生命周期）
        ├── internal/core/**  （内核）
        │     └── internal/interfaces
        ├── internal/drivers/**、channels/**、adapters/**、store/**
        │     └── 实现 internal/interfaces
        └── internal/config、logging、clock/realclock、sim
```

规则：

1. `internal/core/**` 只依赖 `internal/interfaces`、`internal/logging`（共享结构化日志工具，不含具体实现）与只读配置模型；不得 import 任何具体驱动、通道、适配器或 bbolt 实现。
2. 具体实现的构造与装配只发生在 `cmd/collect-code` 与 `internal/app`。
3. `internal/interfaces` 不依赖仓库内其他业务包。
4. 依赖方向由 `scripts/check-deps.sh` 的传递闭包检查强制执行；时间规则由 `scripts/check-time.sh` 强制执行。
5. `internal/topics` 是纯函数包，供 `internal/channels/**` 与 `internal/adapters/**` 共用；不得反向依赖具体通道、适配器或内核。

## 内核组件

| 组件 | 职责 | 明确不做 |
|---|---|---|
| `scheduler` | 相位锁定 tick、链路内串行、单设备截止时间、missed tick | 不做重试策略；不做协议分支 |
| `state` | 设备状态机、属性快照、质量与新鲜度统一计算 | 不持久化全部历史 |
| `invoker` | 服务调用生命周期、幂等去重、超时与重发、队列治理 | 不解读业务参数；参数校验归下行适配器 |
| `forward` | 两级缓存、补发顺序、容量与淘汰 | 不保证跨重启补发事件 |
| `eventbus` | 进程内事件分发与去重键 | 不做规则或订阅表达式 |
| `status` | 只读健康端点与计数器 | 不做写操作；不展示配置 |

## 南向、北向与适配

| 组件 | 职责 |
|---|---|
| `drivers/modbus` | 共用 PDU 编解码；三种传输：TCP、串口 RTU、RTU-over-TCP |
| `drivers/lorawan` | ChirpStack gRPC 下行、MQTT 事件上行、payload 直传适配器 |
| `channels/mqtt` | MQTT 发布/订阅、重连、遗嘱消息 |
| `adapters/irrigation_v1` | 属性/事件编码、服务调用解析与回执编码、声明式 payload 位域解码 |
| `store/memory`、`store/bbolt` | 同一 Store 合同的内存实现与生产实现 |

## 包布局

当前已存在的包：

```text
cmd/collect-code/            # 入口、子命令、具体实现的装配
internal/app/                # 进程装配、simulate 启动、集成测试
internal/interfaces/         # Driver/Channel/Adapter/Clock/Store 与通用类型
internal/topics/             # 纯函数：Topic/client-id 模板渲染与 MQTT topic/filter 校验
internal/core/engine/        # 最小内核接线（时钟、生命周期）
internal/core/scheduler/     # 相位锁定 tick、链路内串行、单设备截止时间
internal/config/             # 四层 YAML 解析、校验、诊断、摘要
internal/clock/realclock/    # 唯一直接使用运行时时间的包
internal/logging/            # JSON 结构化日志与脱敏
internal/sim/                # 模拟器 runtime、manifest、capture、control、broker、HTTP
internal/testutil/fakeclock/ # 假时钟
internal/testutil/fake/      # 假驱动/通道/适配器
internal/version/            # 版本与构建信息
```

规划中的包（在对应 ticket 实现前不存在）：

```text
internal/core/scheduler/
internal/core/state/
internal/core/invoker/
internal/core/forward/
internal/core/eventbus/
internal/core/status/
internal/drivers/modbus/
internal/drivers/lorawan/
internal/channels/mqtt/
internal/adapters/irrigation_v1/
internal/store/memory/
internal/store/bbolt/
```

规划中的包名是目标结构，不是当前事实；ticket 可以在不改变依赖方向的前提下调整内部拆分。任何把具体实现放进 `internal/core/**` 的改动都违反 `REQ-SEAM-01`/`REQ-SEAM-02`。

## 运行时数据流

1. 链路调度器按 tick 选择到期设备，调用注入的驱动读取。
2. 内核状态组件用样本时间与阈值计算最终质量、推进最后成功时间、判定离线与恢复。
3. 上行适配器把属性与事件编码为报文，北向通道发布。
4. 北向断开时，事件进入有界队列，属性只保留最新快照；恢复后先事件后快照。
5. 下行调用经下行适配器解析与校验，内核做幂等判定、队列治理与分级确认，驱动执行。

## 装配边界

`cmd/collect-code` 与 `internal/app` 是唯一知道全部具体实现的地方。新增协议、通道或适配器时，只新增实现包并在装配处接线；内核与规范不因新增实现而改变。需要改变行为语义时，先按 ADR-0006 走规范变更。
