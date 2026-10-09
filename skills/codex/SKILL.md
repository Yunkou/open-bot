---
name: codex
description: Delegate coding to the OpenAI Codex CLI on a connected computer via host_shell (codex exec, worktrees, review). Use when the user asks to run Codex.
---

# Codex CLI

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/autonomous-ai-agents/codex`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

把编码任务交给用户电脑上的 [Codex](https://github.com/openai/codex) CLI（OpenAI 的自主编码 agent）。全部经 `host_shell` 执行（先 `list_machines`，再 `load_skill host-shell`）。

## 何时用

- 用户明确要用 Codex
- 较大的功能 / 重构 / PR 评审 / 批量修 issue，希望交给外部 agent 独立完成

## 前提

- 已安装：`npm install -g @openai/codex`（`host_shell` 跑 `codex --version` 检查）
- 已登录：`OPENAI_API_KEY` 或 Codex CLI 自己的 OAuth（`~/.codex/auth.json`）。没有 `OPENAI_API_KEY` 不代表没登录。
- **必须在 git 仓库里**运行；临时任务用 `cd $(mktemp -d) && git init && …`

## open-bot 里的执行方式

`host_shell` 是一次性命令：没有 PTY、没有 stdin、stdout 会截断，也没有 `workdir` 参数（用 `cd DIR && …`）。

### 一次性任务（首选）

```bash
cd ~/project && codex exec --sandbox workspace-write 'Add dark mode toggle to settings' 2>&1 | tail -n 60
```

`codex exec` 非交互、做完即退出，不需要 PTY。

### 长任务（后台 + 日志轮询）

```bash
cd ~/project && nohup codex exec --sandbox workspace-write 'Refactor the auth module' > /tmp/codex-auth.log 2>&1 &
echo started
```

之后用 `host_shell` 轮询：`tail -n 40 /tmp/codex-auth.log`；结束判断：`pgrep -fl "codex exec" || echo done`。整个任务很长时，用 `defer_work` 把「启动 + 轮询 + 汇报」放到后台，本轮先简短确认。需要停止：`pkill -f "codex exec.*auth module"`（会走确认卡）。

### 需要来回交互时（tmux）

```bash
tmux new-session -d -s codex -x 160 -y 50 -c ~/project 'codex'
tmux send-keys -t codex 'Implement OAuth refresh flow and add tests' Enter
tmux capture-pane -t codex -p -S -40      # 看进度
tmux kill-session -t codex                # 结束
```

## 常用参数

| 参数 | 作用 |
|------|------|
| `exec "prompt"` | 一次性执行，完成即退出 |
| `--sandbox workspace-write`（`-s`） | 在工作区内自动批准改文件（推荐的自动构建模式） |
| `--sandbox danger-full-access` | 关闭 Codex 沙箱；桌面服务上下文里 bubblewrap 失败时才用 |
| `--dangerously-bypass-approvals-and-sandbox` | 无沙箱无审批，最危险；除非用户明确要求，不要用 |
| `review --base origin/main` | 评审当前分支相对 base 的改动 |

`--full-auto` 已弃用，改用 `--sandbox workspace-write`。

## PR 评审（在临时克隆里做，避免弄脏用户工作区）

```bash
REVIEW=$(mktemp -d) && git clone https://github.com/user/repo.git $REVIEW && cd $REVIEW && gh pr checkout 42 && codex review --base origin/main 2>&1 | tail -n 80
```

## 并行修 issue（worktree 隔离）

```bash
cd ~/project && git worktree add -b fix/issue-78 /tmp/open-bot-scratch/issue-78 main
cd /tmp/open-bot-scratch/issue-78 && nohup codex exec --sandbox workspace-write 'Fix issue #78: <描述>. Commit when done.' > /tmp/codex-78.log 2>&1 &
```

每个任务一个 worktree，不要多个 Codex 共用一个目录。完成后：

- 用 `git -C /tmp/open-bot-scratch/issue-78 log --oneline -5` 和 `git diff main...` 检查结果，跑相关测试。
- **push、`gh pr create`、`gh pr comment` 都是对外可见操作**：先把分支名 / PR 标题 / 正文给用户确认，再执行。
- 清理：`git worktree remove /tmp/open-bot-scratch/issue-78`。

## 规则

1. 一次性任务用 `codex exec`；长任务 `nohup … &` + 日志轮询；需要交互才用 tmux。
2. 必须在 git 仓库；启动前确认 `git status` 干净，方便事后 `git diff` 审查。
3. 任务提示要窄而具体；结束后自己核对改动和测试，不要只转述 Codex 的自述。
4. 不要打断正在跑的任务；看日志判断进度，耐心等。
5. 汇报时给出：改了哪些文件、测试结果、遗留风险。
