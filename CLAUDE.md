# 项目约定

## 文档同步红线

命令列表、参数、示例、帮助文案在以下四处重复出现，任何一处发生变更，必须同步修改全部四处：

1. `cf.go` 的 `usage` 字符串（`cf --help` 的实际输出）
2. `README.md`（顶部导航、Usage 说明、内嵌的帮助块）
3. `README_zh_CN.md`（中文说明与中文帮助块）
4. `docs/commands.md`（完整命令参考）

新增或修改命令时按顺序执行：

1. 修改 `cf.go` 的 usage：命令列表、Options、Examples。
2. 重新构建并导出帮助文本：

   ```bash
   go build -ldflags "-s -w" -o cf cf.go
   ./cf --help > /tmp/cf_help.txt
   ```

3. `README.md` 的内嵌帮助块直接取自 `/tmp/cf_help.txt`（去掉第一行标题），保证与程序输出一致。
4. `README_zh_CN.md` 是中文翻译版，需要手工同步命令列表、参数、示例和中文说明，不能直接替换成英文帮助。
5. `docs/commands.md` 补充或更新对应命令章节，包括：用途、语法、参数、示例、生成的文件或目录、限制条件。
6. 最后运行 `./cf --help`、`go test ./...`、`go vet ./...` 核对。

修改 `cf pull`、`cf sync`、`cf clone` 等涉及文件落盘的命令时，文档里的路径必须与代码实际行为一致，不能沿用旧描述。

## 构建与验证

```bash
go build -ldflags "-s -w" -o cf cf.go
go test ./...
go vet ./...
```
