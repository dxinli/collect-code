# 06b: 内嵌 MQTT broker 与云端桩

**What to build:** 一条命令起可被真实二进制连接的假云端，并能注入下行。

**Blocked by:** 01a

**Status:** done

- [x] 内嵌纯 Go broker 监听 `127.0.0.1:0`；`--manifest` 输出实际地址
- [x] 云端桩订阅上行并可通过 `GET /_sim/captured` 查询
- [x] `POST /_sim/invoke` 注入完整下行报文
- [x] `broker_disconnect` 可模拟断线并恢复
- [x] 无外部依赖（不要求 docker/apt）

## Comments

- 2026-11-21 实现完成，待 PR 审核；审核通过后把 `Status` 改为 `done`。
- 实现提交：`fbfde0f`（runtime/manifest/capture/control/http）、`b2590b5`（mochi broker + HTTP API + 独立 MQTT 客户端）、`98ff78f`（simulate CLI）、`b9d3319`（app 注入）、`bfc5539`（import 边界）、`564c019`（进程级测试）、`7279a0b`（spec）。
- 验证：`make ci` 全绿（fmt/print/deps/time/go-fix/vet/test、linux/amd64 + linux/arm64 静态构建、`go test -race ./...`、golangci-lint 0 issues）；真实二进制收到 SIGTERM 退出 0，端口释放。
- 实现说明：plan 的 step 3（HTTP API）与 broker 同属一次性可测的传输层，合并在 `b2590b5`；mochi 的 `HookBase`/`OnConnectAuthenticate`/`OnACLCheck` 在根包 `mqtt`（不在 `hooks` 子包），spec 与实现以根包为准。
- 未包含：06a 的 Modbus 从站、12a/12b 的 ChirpStack 桩、07 的 E2E；完整 ticket 06 的代码走读待 06a 完成后单独提交。
- 2026-11-22 按用户要求将 `Status` 改为 `done`，并同步 `ticket-dependency.html` 中 06b 的状态。
