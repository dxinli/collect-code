# 19: 交付文档包与构建脚本

**What to build:** 另一个工程师只按文档就能从零部署、联调、排障，并复现验收结果。

**Blocked by:** 15,18

**Status:** ready-for-agent

- [ ] 部署文档（systemd/端口/日志/权限）可从零安装
- [ ] Topic 与字段文档含 seq 契约、分级确认、code 表、事件表
- [ ] Modbus 映射表与 ASSUMED 清单索引齐全
- [ ] 测试说明含模拟器用法与四层测试；排障章节含 config-check/queue flush/event_dropped
- [ ] Makefile 产物与文档步骤一致，冒烟脚本可跑
