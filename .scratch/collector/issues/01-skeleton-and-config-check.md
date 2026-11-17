# 01: 工程骨架与配置校验

**What to build:** 从零 `git clone` 到能跑 `version` 与 `config-check`：四层 YAML 能解析、能校验、能给出可定位的报错，并能交叉编译出可上设备的静态二进制。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `collect-code version`、`collect-code config-check --config <dir>` 可运行
- [ ] 四层配置（config/links/profiles/devices）全部字段可解析，未知键报错
- [ ] 校验规则逐条生效：必填、唯一、引用完整（profile/link/point）、数值范围、传输块互斥、端口冲突
- [ ] 错误信息包含文件名与行列号；坏配置退出码非 0
- [ ] `Makefile` 可产出 `linux/amd64` 与 `linux/arm64` 且 `CGO_ENABLED=0` 的静态二进制
