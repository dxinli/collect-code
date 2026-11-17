# 01b: 配置校验规则与可定位诊断

**What to build:** 改错配置能立刻知道哪一行哪条规则错，并且不靠人读日志猜。

**Blocked by:** 01a

**Status:** ready-for-agent

- [ ] 校验规则编号为 V01…；每条规则有一个合法用例与一个非法用例
- [ ] 诊断统一格式 `file:line:col: rule_id: message`；跨文件引用错误定位到引用字段所在文件行
- [ ] `config-check` 打印生效配置摘要（密码/Token 脱敏）
- [ ] `testdata/config/` golden corpus 全部按预期通过或失败
- [ ] 未知嵌套键、profile/link 缺失、只读点位写入、数值边界各有用例
