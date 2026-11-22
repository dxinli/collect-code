# ADR-0003：新鲜度由内核统一计算，调度以链路相位锁定

- 状态：Accepted
- 日期：2026-11-17
- 相关：`docs/产品定型.md` §6.1/§6.3.1、D16/D19、`docs/缺口台账.md` B8/B9/C7

## 背景

三条事实同时成立：

1. LoRa 是推送型：没有「轮询」概念，设备来一条上行才有新值。若把最后一次上行缓存起来冒充 `Read` 的返回值，且驱动自称 `good`，内核会把几小时前的旧值当成新鲜数据，甚至把离线设备判成在线。
2. Modbus 是拉取型：一条总线挂多台从站，某台离线时重试会拖住整条总线。
3. 现场网关多数没有可靠 NTP，墙钟会跳。

## 决策

1. **质量初值 ≠ 最终质量**。`Sample.Quality` 由驱动给出观测初值（读失败为 `bad`）；最终 `good/stale` 由内核用 `Sample.At` 与 `freshness_window` 计算。内核只允许把驱动给的 `bad` 保持不变，绝不把 `bad` 升级为 `good`。
2. **`last_success_at` 只由新鲜观测推进**：一次 `Read` 若返回的全部样本都超龄，不推进在线判定。A 级 `txack`（网关已发射）只表示「下发链路可用」，不作为设备在线证据。
3. **调度器相位锁定**：`next = start + k*period`，不使用 `Sleep(period)`。迟到超过一个周期的 tick 记 `missed_tick` 并**跳过**，不补跑（补跑会让后续 tick 连续拥挤，抖动放大）。
4. **单设备独立 deadline**：链路内串行遍历设备，每台设备有自己的读超时；超时即把该设备全部点位标 `bad` 并立刻处理下一台，不进入重试链阻塞总线。单 tick 最坏耗时被 `设备数 × read_deadline` 封顶。
5. **时间只从注入的 `Clock` 进入内核**。`Clock` 是完整时间抽象，不只是 `Now()`：

   ```go
   type Clock interface {
       Now() time.Time
       NewTicker(d time.Duration) Ticker   // 内核不得直接 time.NewTicker
       NewTimer(d time.Duration) Timer     // 内核不得直接 time.After/context.WithTimeout 做业务超时
       Sleep(d time.Duration)
   }
   type Ticker interface{ C() <-chan time.Time; Stop() }
   type Timer  interface{ C() <-chan time.Time; Stop() }
   ```

   **硬规则**：`internal/core/*` 与 `internal/drivers/*` 不得直接调用 `time.Now/NewTicker/NewTimer/After/AfterFunc/Sleep`（`time.Duration` 类型本身照常用）。所有等待与截止走 `Clock`。业务超时**不用** `context.WithTimeout`，因为它的截止由运行时真实定时器驱动，假时钟无法快进；context 只用于取消传播。

   排序与窗口计算用 `Clock`（单调语义），上报 `ts` 用 `Clock.Now()` 的墙钟值。测试中 `Sample.At` 必须由同一个假时钟产出，否则 `Sub` 会跨时钟源、边界断言差一纳秒翻车。

## 后果

- 驱动实现变简单：不需要判断「数据算不算新鲜」，只报 `At` 与观测结果。
- LoRa 驱动的 `Read` 允许返回空结果（`OutcomeUnsupported`），内核据此不刷新任何属性，也不推进在线判定。
- **周期不累积**可用假时钟验证（算术性质）；**真实抖动 ≤ ±10%** 只能在真实时钟的短 soak 里验证——两者分开，别把前者当后者。
- 内核可用假驱动 + 注入式假时钟做确定性单测，不需要 `time.Sleep`，也不需要任何真实协议。
- 代价：内核必须显式接收 `Clock`，不能写成包级全局；驱动层同样接收，避免串口/TCP 超时绕过假时钟。真实网络 I/O 的超时由驱动底层库控制，测试时用内存对端而不是 mock 时间。

## 被否决的方案

- **驱动自报最终质量**：每种协议各有语义，内核无法统一校验，LoRa 旧值会被标成 `good`。
- **`Sleep(period)` 循环**：漂移累积，跑一天就偏几分钟。
- **missed tick 补跑**：连续补跑造成 tick 拥挤，抖动反而更差。
- **链路级唯一超时**：一台离线从站会吞掉整条总线的采集窗口。

## Amendment（2026-11-22）

本 ADR 记录决策与理由；**规范正文**已迁移为：

- 时间与调度：[`REQ-TIME-01`、`REQ-SCHED-01..04`](../spec/spec.md)
- 质量、新鲜度与离线：[`REQ-QUALITY-01..05`](../spec/spec.md)
- 测试判据：[`docs/testing.md`](../testing.md)

上文中的接口片段是决策当时的形状，当前签名以 `internal/interfaces` 为准。本文不改写，以避免历史被追溯性修改。
