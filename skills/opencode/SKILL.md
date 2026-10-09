---
name: opencode
description: Delegate coding to the OpenCode CLI on a connected computer via host_shell (opencode run, PR review). Use when the user asks to run OpenCode.
---

# OpenCode CLI

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/autonomous-ai-agents/opencode`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

把编码任务交给用户电脑上的 [OpenCode](https://opencode.ai)（开源、可换模型供应商的编码 agent，有 TUI 和 CLI）。全部经 `host_shell` 执行（先 `list_machines`，再 `load_skill host-shell`）。

## 何时用

- 用户明确要用 OpenCode
- 想让外部编码 agent 独立实现 / 重构 / 评审代码
- 需要并行的隔离任务（各自 worktree）

## 前提

- 已安装：`npm i -g opencode-ai@latest` 或 `brew install anomalyco/tap/opencode`
- 已登录：`opencode auth login` 或供应商环境变量（`OPENROUTER_API_KEY` 等）；`opencode auth list` 至少一个 provider
- 代码任务建议在 git 仓库里

## 二进制解析

不同 shell 可能找到不同的 opencode：`which -a opencode`、`opencode --version`。必要时写全路径：`$HOME/.opencode/bin/opencode run '...'`。

## open-bot 里的执行方式

`host_shell` 是一次性命令：没有 PTY、没有 stdin、stdout 会截断，没有 `workdir` 参数（用 `cd DIR && …`）。

### 一次性任务（首选，不需要 PTY）

```bash
cd ~/project && opencode run 'Add retry logic to API calls and update tests' 2>&1 | tail -n 60
cd ~/project && opencode run 'Review this config for security issues' -f config.yaml -f .env.example
cd ~/project && opencode run 'Refactor auth module' --model openrouter/anthropic/claude-sonnet-4
```

### 长任务（后台 + 日志）

```bash
cd ~/project && nohup opencode run 'Implement OAuth refresh flow and add tests' > /tmp/opencode-oauth.log 2>&1 &
```

轮询：`tail -n 40 /tmp/opencode-oauth.log`；是否结束：`pgrep -fl "opencode run" || echo done`。很长的任务配合 `defer_work`。

### 交互式 TUI（tmux）

```bash
tmux new-session -d -s oc -x 160 -y 50 -c ~/project 'opencode'
tmux send-keys -t oc 'Implement OAuth refresh flow and add tests' Enter
tmux capture-pane -t oc -p -S -40
tmux send-keys -t oc C-c          # 退出（不要发 /exit，它会打开 agent 选择器）
```

TUI 里 Enter 有时要按两次（一次确认文本、一次发送）。继续上次会话：`opencode -c`；指定会话：`opencode -s ses_abc123`。

## 常用参数

| 参数 | 用途 |
|------|------|
| `run 'prompt'` | 一次性执行后退出 |
| `--continue` / `-c` | 继续上次会话 |
| `--session <id>` / `-s` | 继续指定会话 |
| `--agent <name>` | 选择 agent（build / plan） |
| `--model provider/model` | 指定模型 |
| `--format json` | 机器可读输出 |
| `--file <path>` / `-f` | 附加文件 |
| `--thinking` | 显示思考过程 |
| `--variant <level>` | 推理强度（high / max / minimal） |
| `--title <name>` | 会话命名 |

## PR 评审

```bash
cd ~/project && opencode pr 42
# 或在临时克隆里隔离评审
REVIEW=$(mktemp -d) && git clone https://github.com/user/repo.git $REVIEW && cd $REVIEW && gh pr checkout 42 && opencode run 'Review this PR vs main. Report bugs, security risks, test gaps, and style issues.'
```

## 并行

每个任务一个 worktree / 目录（例如 `/tmp/open-bot-scratch/issue-101`），各自 `nohup opencode run … > /tmp/oc-101.log 2>&1 &`。不要多个会话共用一个目录。

## 会话与成本

`opencode session list`、`opencode stats`、`opencode stats --days 7 --models anthropic/claude-sonnet-4`

## 冒烟测试

```bash
opencode run 'Respond with exactly: OPENCODE_SMOKE_OK'
```

输出含 `OPENCODE_SMOKE_OK` 且无 provider/model 报错即可。

## 规则

1. 一次性自动化优先 `opencode run`；需要多轮迭代才开 TUI（tmux）。
2. 每个会话限定一个仓库 / 目录。
3. 长任务定期从日志汇报进度；看起来卡住先看日志再决定是否结束进程。
4. 结束后自己核对 `git diff` 和测试，汇报改动文件、测试结果、遗留风险。
5. push / 开 PR / 评论 PR 属对外操作，先给用户确认内容。
