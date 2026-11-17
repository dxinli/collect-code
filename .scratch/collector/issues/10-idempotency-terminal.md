# 10: 幂等、两阶段与 seq 契约

**What to build:** 重复投递或崩溃重启后阀门不重复动作，云端能区分回放与未知。

**Blocked by:** 09,02b

**Status:** ready-for-agent

- [ ] 首次 seq 恰好调用一次驱动 Write，参数与目标精确匹配，回 code=0
- [ ] 同 seq 第二次不再 Write，回 4003 + replay=true，data 与首次 final 逐字段一致
- [ ] 子进程崩溃：BeginAccepted 后 os.Exit，重启后同 seq 回 4004 且 Write 次数=0
- [ ] seq 窗口：last_seq、last_seq-1、窗口边界；明显回退回 4001
- [ ] TTL 清理：过期 final 删除、last_seq 保留、accepted 的处理规则明确
