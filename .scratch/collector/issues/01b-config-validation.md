# 01b: 配置校验规则与可定位诊断

**What to build:** 改错配置能立刻知道哪一行哪条规则错，并且不靠人读日志猜。

**Blocked by:** 01a

**Status:** done

- [x] 校验规则编号为 V01…；每条规则有一个合法用例与一个非法用例
- [x] 诊断统一格式 `file:line:col: rule_id: message`；跨文件引用错误定位到引用字段所在文件行
- [x] `config-check` 打印生效配置摘要（密码/Token 脱敏）
- [x] `testdata/config/` golden corpus 全部按预期通过或失败
- [x] 未知嵌套键、profile/link 缺失、只读点位写入、数值边界各有用例

## Comments

- 2026-11-19 计划 v4 经用户评审通过；4 条 spec 外规则移出并登记 `01c-config-semantics.md`。
- 实现按功能点提交：`094c601` 位置索引/规则号、`92d22c6` 语义校验、
  `43463c6` 生效摘要、`f4826b4` golden、`e00b247` 评审修复、
  `712db8c` 合法用例入库与 P8/空 access 边界。
- 规则 39 条：V01–V07、V10–V16、V20–V24、V30–V35、V40–V45、
  V50–V53、V60–V63。P1=A 的 points ∪ payload 口径已落地。
- 双轴 code review 修复：31 个空合法目录未入库（干净检出失败）→ 每个用例
  提交 `case.yaml`；V40/V41 文案与定位对齐已批准计划；补 `legal/base`、
  `legal/v60-grpc-invalid-skip`、`v33-access-unset`、params NaN、payload Inf；
  结构诊断改显式规则集合；摘要反射守卫覆盖 Map。
- 验证：`make check` 全绿；`go test -race -count=1 ./...` 全绿；
  `git archive HEAD` 干净检出后 golden/coverage/format 测试全绿。
- `01c` 未实现，保持 `backlog`。
- 2026-11-19 修复 ptrtoerror：`Diagnostic`/`Diagnostics` 的 `Error` 改为指针接收者，
  值类型改用 `String` 保持 `%v` 可读；`Load` 通过 `asError` 返回 `*Diagnostics`。
  提交 `6c7df52`；`golangci-lint run ./internal/config/...` 0 issues。
