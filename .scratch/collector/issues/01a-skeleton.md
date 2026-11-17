# 01a: 工程骨架、配置解析、日志契约

**What to build:** 从零 clone 到能启动与解析配置：二进制可跑、四层 YAML 可读、日志契约固化、交叉编译产物可用。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `version`/`config-check`/`run` 三个子命令可运行；`run` 收到 SIGTERM 优雅退出
- [ ] 四层 YAML（config/links/profiles/devices）可解析；未知键报错
- [ ] 日志为 JSON，字段固定含 device/point/seq/link/reason；`redact_fields` 生效
- [ ] `env:VAR` 形式的凭据可解析，且日志中不出现明文
- [ ] `Makefile` 产出 `linux/amd64` 与 `linux/arm64`；用 `file`/`ldd` 断言静态且架构正确
