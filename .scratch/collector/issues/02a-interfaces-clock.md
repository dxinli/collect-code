# 02a: 内核接口与假时钟

**What to build:** 同一套内核代码能在假时钟与假通道下确定性运行，证明时间与传输真的可替换。

**Blocked by:** 01a

**Status:** done

- [x] `interfaces` 冻结 Driver/Channel/UplinkAdapter/DownlinkAdapter/Clock/Store 与通用类型
- [x] FakeClock 支持 Now/Advance/NewTicker/NewTimer/Sleep；覆盖到期顺序、Stop、零/负 duration、goroutine 泄漏
- [x] 有一个最小内核组件实际通过注入的 Clock 取时间（证明接线，不是孤立实现）
- [x] fake Channel/UplinkAdapter/DownlinkAdapter 可注入
- [x] `internal/core/*` 不 import drivers/channels/adapters/bbolt；由 CI 静态检查强制执行

## Comments

- 2026-11-20 计划 v2 按独立评审修订；用户“全部按推荐，计划评审通过”（决策 4.1–4.11 全部按推荐）。
- 接口在 `internal/interfaces`；真实时钟 `internal/clock/realclock`；FakeClock 与 fake Channel/Adapter 在 `internal/testutil/{fakeclock,fake}`。
- 最小内核组件 `internal/core/engine`：`New` 用 `Clock.Now()` 记启动时间，`Run` 等待取消并计算 uptime；`app.Run` 通过 `Deps{Clock}` 接收由 `cmd` 构造的 `realclock`，startup 日志输出 `started_at`。
- `WriteRequest` 增 `ServiceID`/`Confirmed`（14a/12a 需要）；`ServiceReply` 增 typed `Stage`；写错误哨兵补 `ErrDeviceRejected`(4001)/`ErrDeviceInternal`(5044)。
- 15a 的下行队列 seam 明确延后，`Driver` 只覆盖协议读写。
- 静态检查：`scripts/check-deps.sh`（go list 传递闭包）与 `scripts/check-time.sh`（调用 + 别名 import），接入 `make check`，由 `make ci` 执行；已用故意违规文件自检。
- 实现提交：`8f5f5b9` interfaces、`1b35e50` spec、`1fc0dcf` realclock、`f996963` fakeclock、`503ac12` fake、`584691d` engine、`c94282a` app、`7b45f87` CI、`48fbcd0` lint。
- `make ci` 全绿（含 race 与 golangci-lint 0 issues）；`go.mod`/`go.sum` 未变。
- 后续：完整 ticket 02 的代码走读待 02b 完成后单独提交。
