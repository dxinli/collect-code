# CLAUDE.md

## Agent skills

### Issue tracker

Issues, specs and tickets live as markdown under `.scratch/<feature-slug>/` in this repo. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `GLOSSARY.md` at the root plus `docs/adr/`. See `docs/agents/domain.md`.

## 项目

智能采集应用：部署在边缘（LoRaWAN 网关 / 融合终端）上的采集引擎。一套二进制，按配置装配南向驱动，北向统一 MQTT JSON。
需求原文见 `docs/需求文档_1.md`、`docs/需求文档_2.md`。真实硬件与协议资料的缺口台账见 `docs/缺口台账.md`。

## 硬约束

- Go，交叉编译 `linux/arm64`，同时保留 `linux/amd64` 用于本机联调；单二进制 + YAML 配置 + systemd；不引入数据库，不引入 Web 后台。
- 南向驱动、北向通道、物模型适配器都必须经接口注入，不允许在引擎内核里直接 import 具体实现。
- ChirpStack 是既有系统：本服务只经其 API 做上下行，绝不改动或抢占其端口/会话。
- 当前无实物：所有硬件相关数值一律标记为 `ASSUMED`，集中登记在 `docs/缺口台账.md`，不得散落在代码里当常量口口相传。
