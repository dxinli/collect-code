# 10: 幂等与终态（bbolt 两阶段）

**What to build:** 同一条指令重复投递或进程崩溃重启后，阀门不会被重复动作，且云端能明确知道该换新 seq 还是拿到原结果。

**Blocked by:** 09

**Status:** ready-for-agent

- [ ] 同一 (device,seq) 重发不执行第二次；已知终态回 4003 且 replay=true
- [ ] BeginAccepted 后崩溃（Close 不调 Finish）重开：该 seq 回 4004，不重放
- [ ] 换新 seq 可正常执行；回退 seq 回 4001
- [ ] 同 service 的下行始终只保留最新一条，被取代的上报 cmd_dropped(reason=superseded)
- [ ] 清理任务删除过期终态记录并物化 last_seq
