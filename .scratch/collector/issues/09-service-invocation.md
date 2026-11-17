# 09: 服务调用：解析、校验、执行、回执

**What to build:** 云端下发的服务调用被校验、执行并回一条可判读的回执；参数错、设备不理、链路断三种情况可区分。

**Blocked by:** 08,03

**Status:** ready-for-agent

- [ ] 解析 service/invoke（service_id/seq/params）并按 profile.services 校验；越界/类型/缺字段回 4001 且 data.field 指明字段
- [ ] 执行写入后按 verify 判定，成功回 code=0 且 data 含 target_opening 与实测值
- [ ] 设备无响应回 5042；链路/驱动不可用回 5040 且不产生 cmd_timeout
- [ ] 回执字段含 msg_id/service_id/seq/code/msg/data/elapsed_ms
