# 01a: 工程骨架、配置解析、日志契约

**What to build:** 从零 clone 到能启动与解析配置：二进制可跑、四层 YAML 可读、日志契约固化、交叉编译产物可用。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

**Merged:** historical

- [x] `version`/`config-check`/`run` 三个子命令可运行；`run` 收到 SIGTERM 优雅退出
- [x] 四层 YAML（config/links/profiles/devices）可解析；未知键报错
- [x] 日志为 JSON，字段固定含 device/point/seq/link/reason；`redact_fields` 生效
- [x] `env:VAR` 形式的凭据可解析，且日志中不出现明文
- [x] `Makefile` 产出 `linux/amd64` 与 `linux/arm64`；用 `file`/`ldd` 断言静态且架构正确

## Comments

- 2026-11-17 实现完成（acb11b7 + 评审修复）。
- `make check` 全绿：fmt、print-check、vet、test、amd64/arm64 静态构建与断言、示例配置校验。
- `make test-race` 全绿。
- 双轴 code review 后修复：URL 解析失败整值脱敏、必填错误跨文件聚合、
  `SOURCE_DATE_EPOCH`、`Point.write`、`redact_fields: null` 归一化、
  信号 cancel 路径与二次信号默认动作、CI 禁止直接打印。
- 计划与缺口台账已同步（null 实测行为、诊断列号、G 段 ASSUMED 默认值）。
