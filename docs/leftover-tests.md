# 遗留测试事项

时点：2026-11-05。以下为本次改动尚未在真实账号/真实设备上验证的项目，下一次集中测试时逐条确认。

## 待测清单

1. **真实提交下的 practice 计数**
   - 准备一个练习集，目录形如 `{cf}/practice/<set>/<problem>/`，并存在 `practice.json`。
   - 先正常计时并 Accepted 一次，使状态为 `accepted`。
   - 再执行 `cf timer s p_<set> <problem>`，用 `cf submit` 在同一个计时段内 Accepted。
   - 期望：`cf db sync` 后 `status` 变为 `practice_1`，`problem.practice` 为 1；重复到第 9 次后保持 `practice_9`。
   - 反向用例：普通计时中的 Accepted 不增加 practice；首次 Accepted 发生在 practice 计时中时仍为 `accepted`。
   - 重点检查 `cf submit` 判定 Accepted 后询问“是否关闭计时器”的两条分支都不影响计数。

2. **practice 计时入口**
   - `cf timer s p_<set> <problem>` 能按 `practice.json` 的 `display` 找到题目目录。
   - `practice.json` 缺失时按 `<contestId><index>` 回退。
   - 题目不在练习集、练习集不存在时均报错且不写计时数据。
   - 默认时间戳练习集、自定义 `<name>`、gym、group、acmsguru 各测一次。
   - `cf timer status` 显示 `题目 (p_<set>)`；`cf timer q` 的总结里事件带 `[practice <set>]`。

3. **练习集生成**
   - `cf lpractice`、`cf lrepractice`、`cf lnrepractice` 在真实 `~/.cf/timer.db` 上分别抽取全部 / `accepted` / `pending` 的题，数量与状态正确。
   - `cf opractice` 的 `--difficulty` 单值与区间边界、`--tags` 多标签（当前为 OR）、数量不足警告。
   - 生成目录中样例、`statement.md`、代码文件、`practice.json` 均存在；无模板时不生成代码文件。
   - 练习集名为中文或包含空格、斜杠时的目录名清洗结果。

4. **数据库迁移**
   - 用旧版 `~/.cf/timer.db`（只有 `status`、或还有 `tests_passed`）启动新二进制。
   - 期望自动补 `practice` 列、删除冗余 `tests_passed` 列，旧数据可继续查询。
   - 迁移后执行一次 `cf db sync`，确认历史 `count_time.json` 的状态和 practice 计数重算正确。

5. **并发与锁**
   - 计时器写入事件的同时执行 `cf db sql --write`，确认 WAL/busy_timeout 下不出现 `database is locked`。
   - 同时运行两个 `cf db sync`，确认结果幂等、事件不重复。

6. **查询与输出**
   - `cf db time <problem>`、`cf db time <contest>`、`cf db stats` 的 practice/status 列与 JSON 字段。
   - `--status practice_1`、`--status practice_9`、非法值 `practice_10` 的报错。
   - `cf db sql` 只读写入拒绝、`--write` 正常。

7. **补全脚本**
   - bash/zsh/fish 中 `cf lp<Tab>`、`cf lrep<Tab>`、`cf lnrep<Tab>`、`cf op<Tab>` 能补全到对应命令。

## 已知限制

- 本地数据库只包含被计时或 `cf db sync` 导入过的题目；`lnrepractice` 的 `pending` 包含仅由比赛元数据导入、从未计时的题目。
- `opractice` 的 tags 过滤为“命中任意一个”，不是 AND。
- group 比赛与 acmsguru 没有在线元数据，名称、div、起止时间留空。
- 练习集下载样例和题面需要有效的 Codeforces 会话；题面失败只警告。
- 首次 Accepted 即使发生在 practice 计时中仍记 `accepted`，practice 计数从下一次 practice Accepted 开始。
- `cf timer start gym 100001a` 这类多 token specifier 仍只解析第一个 token（既有行为，本次未改）。
- 新增 SQLite 依赖后 `vendor/` 由约 11M 增至约 140M，二进制由约 8M 增至约 12M。
