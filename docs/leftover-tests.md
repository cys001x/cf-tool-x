# 遗留测试事项

时点：2026-11-05。以下为本次改动尚未在真实账号/真实设备上验证的项目，下一次集中测试时逐条确认。

## 待测清单

1. **真实提交下的 practice 计数**
   - 准备一个练习集，目录形如 `{cf}/practice/<set>/<problem>/`，并存在 `practice.json`。
   - 先正常计时并 Accepted 一次，使状态为 `accepted`。
   - 再执行 `cf start p_<set> <problem>`，用 `cf submit` 在同一个计时段内 Accepted。
   - 期望：`cf db sync` 后 `status` 变为 `practice_1`，`problem.practice` 为 1；重复到第 9 次后保持 `practice_9`。
   - 反向用例：普通计时中的 Accepted 不增加 practice；首次 Accepted 发生在 practice 计时中时仍为 `accepted`。
   - 重点检查 `cf submit` 判定 Accepted 后询问“是否关闭计时器”的两条分支都不影响计数。

2. **practice 计时入口**
   - `cf start p_<set> <problem>` 能按 `practice.json` 的 `display` 找到题目目录。
   - `practice.json` 缺失时按 `<contestId><index>` 回退。
   - 题目不在练习集、练习集不存在时均报错且不写计时数据。
   - 默认时间戳练习集、自定义 `<name>`、gym、group、acmsguru 各测一次。
   - shell 内 `status` 显示 `题目 (p_<set>)`；`q` 的总结里事件带 `[practice <set>]`。

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
- `cf start gym 100001a` 这类多 token specifier 仍只解析第一个 token（既有行为，本次未改）。
- `p_<set>_<problem>` 只在当前目录附近能定位到 `cf` 根时生效，且必须单独使用；练习题部分必须以数字开头（练习目录名 `<contestId><index>`），练习集名因此可含下划线。
- 新增 SQLite 依赖后 `vendor/` 由约 11M 增至约 140M，二进制由约 8M 增至约 12M。

## 2026-11-05 复测记录

已通过：

- 真实提交 `397455477`、`397457290` 均为 Accepted，practice 状态从 `practice_1` 递增到 `practice_2`。
- Accepted 后输入 `y` 的分支：关闭计时器、写入 session summary、end 事件带 `practice`。
- 默认时间戳练习集、gym、group、acmsguru 的 `p_<set>` 开始/结束。
- `lrepractice`、`lnrepractice`、`lpractice`、`opractice` 生成练习集；数量不足警告。
- 中文与斜杠的练习集名清洗为 `练习_集`。
- `db stats --status practice_2`、JSON 输出、非法 `practice_10` 报错、只读写入拒绝。
- 两个 `cf db sync` 并发执行；计时器写入与 `cf db sql --write` 并发执行。
- 旧库迁移：补 `status`、`practice`（problem/event 两表），删除 `tests_passed`。
- 删除数据库后 `db sync` 重建，practice 状态恢复。

复测中发现并修复：

1. 同一题存在多个 `count_time.json` 时，后索引的记录会覆盖先前记录的状态。改为按 `problem_key` 聚合数据库中的全部事件后重算 `status`/`practice`。
2. `event` 表缺少 `practice` 列，无法跨记录聚合。新增列、迁移，并用 upsert 回填旧事件。
3. 仅由比赛元数据导入的题目 `display` 为空，练习集目录退化为单字母（如 `c`）。改为回退 `<contestId><index>`。

仍未测：

- 真实连续提交到 `practice_3` 至 `practice_9`；当前由单元测试覆盖递增与上限。
- 真实“首次 Accepted 发生在 practice 计时中”的场景；单元测试覆盖仍记 `accepted`。
- `cf test` 自动 test 事件；本地模板脚本缺少 `./`，Go `exec` 拒绝相对 PATH 可执行文件。
- v1.3.1 发行版不包含本次复测的 3 个修复，需要下一个版本发布。

## 2026-11-06 计时器内执行命令（新增）

计时器前台改为轮询 stdin（Unix `poll`，Windows `WaitForSingleObject`），
移除了常驻读取 goroutine；非计时命令以子进程 `cf <line>` 执行。
入口调整（2026-11-06 追加）：不带参数的 `cf` 打开 shell；计时入口为
`cf start`/`cf end`（shell 内可用 `s`/`e`）。`cf timer` 与 `cf counttime`
已移除，按未知顶层命令输出 npm 风格提示。

已通过（单元测试 + 本地 pty 模拟）：

- 计时器内 `s/e/p/status/q` 仍按计时命令处理。
- `submit`、`completion bash`、`db` 等作为子进程执行，计时器保持运行，命令结束后状态行恢复。
- `open` 在计时器内执行 `cf open`，不再嵌套打开计时器；`timer`/`counttime` 被拒绝。
- 子进程读取 stdin 正常（`cf config` 菜单与 User-Agent 输入实测），父进程不抢输入。
- 引号与转义解析（`submit -f "a b.cpp"`）、空行刷新、`q` 关闭且总结只打印一次。
- 子命令运行中按 Ctrl-C（`cf config` 实测）：子命令退出，计时器继续，父进程未被 SIGINT 杀死。
- `printf 'status\nq\n' | cf` 的逐行执行（单元测试 + 本地二进制实测）。
- `p_<set>_<problem>` 单 token specifier：`infoFromSpecifiers` 解析、`getOneCode` 在练习目录找代码、
  `cf timer s/e p_drill_1136a`（旧入口，逻辑现由 `cf start`/`cf end` 复用）生成带 practice 的事
  件、`cf db time p_drill_1136a` 输出记录
  （单元测试 + 本地二进制实测）；计时器 REPL 内 `s/db time/e` 全流程（pty 模拟）。
  带下划线的练习集名 `p_drill_easy_1136a`、非法形式 `p_drill`/`p__1136a` 已覆盖。

已修复（2026-11-06）：

- 输入行超过终端宽度时状态行换行，`CursorUp(1)` 定位到错误的行，导致输入内容被反复打印。
  状态行现在按终端列宽裁剪（中文按两列计算），输入只显示末尾并以 `…` 开头；终端尺寸每秒
  重新获取；未收全的多字节字符暂不显示。单元测试 `TestFitTail`/`TestInputDisplay`，另有
  80 列 pty 实测（ASCII 与中文输入下状态行最长 79 列，无换行）。

待测（真机 / 真实账号）：

- 计时器内真实 `cf submit`：Accepted 后 `y`（关闭计时器，总结只出现一次）与 `n`（继续计时）两条分支。
- 真实 `cf submit p_drill_1136a`：确认提交 URL、代码来源与普通 specifier 一致（需网络）。
- `cf test p_drill_1136a` 用练习目录中的样例运行；练习目录缺样例时的报错文案。
- Windows Terminal 交互模式：状态行刷新（`WaitForSingleObject` 路径）、Ctrl-C 与中文输入。
- 子命令输出未以换行结尾时，状态行的覆盖情况。
- `cf test` / `cf race` / `cf watch` 等长时间或自带信号处理的命令在计时器内的中断行为。
- 终端窗口尺寸变化（SIGWINCH）时状态行与输入行的对齐。
