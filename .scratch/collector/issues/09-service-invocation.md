# 09: 服务调用与写后回读

**What to build:** 云端指令被校验、执行、回读验证，并回一条能判读的结果。

**Blocked by:** 08,02b

**Status:** ready-for-agent

- [ ] 解析/校验 service.invoke；越界/类型/缺字段回 4001 且 data.field 指明字段
- [ ] 写后回读容差内回 0；超差回 5043；帧发出无响应回 5044；链路 down 回 5040
- [ ] 回执字段含 msg_id/service_id/seq/code/msg/data/elapsed_ms
- [ ] target_opening 来自引擎受理时 state，不从设备 payload 解码
- [ ] 失败码矩阵按驱动/阶段逐行测试；4002（设备未启用）与 4041（动态只读）各有用例
