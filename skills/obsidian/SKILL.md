---
name: obsidian
description: Read, search, create and edit notes in an Obsidian vault on a connected computer (host_read / host_write / host_shell rg). Use for Obsidian / 笔记库 requests.
---

# Obsidian Vault

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/note-taking/obsidian`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

## open-bot 运行方式

- **在哪执行**：用户已连接的电脑。先 `list_machines` 选机，再 `load_skill host-shell`；命令用 `host_shell`（一次一条，stdout 会截断），读写文件用 `host_read` / `host_write`，搜文件内容用 `host_shell` 跑 `rg` / `grep`，查「最大/某类文件」用 `host-file-query`。
- **长进程**：没有后台进程工具。用 `nohup <cmd> > /tmp/<name>.log 2>&1 &` 启动，再用 `host_shell` 跑 `tail -n 50 /tmp/<name>.log` 轮询；整体耗时很长的任务用 `defer_work` 放后台交付。
- **确认**：写入/删除/有风险的命令由系统确认卡处理；结果 waiting = 尚未执行，denied = 用户拒绝，如实说明，不要编造输出。


Use this skill for filesystem-first Obsidian vault work: reading notes, listing notes, searching note files, creating notes, appending content, and adding wikilinks.

## Vault path

Use a known or resolved vault path before calling file tools.

The documented vault-path convention is the `OBSIDIAN_VAULT_PATH` environment variable, for example exported in `~/.zshrc` (or remembered via `memory_write`). If it is unset, use `~/Documents/Obsidian Vault`.

File tools do not expand shell variables. Do not pass paths containing `$OBSIDIAN_VAULT_PATH` to `host_read`, `host_write`, `host_write`（先 `host_read` 再整文件写回，细节见 `coding-edit`）, or `host_shell`（`rg` / `grep` / `find`）; resolve the vault path first and pass a concrete absolute path. Vault paths may contain spaces, which is another reason to prefer file tools over shell commands.

If the vault path is unknown, `host_shell` is acceptable for resolving `OBSIDIAN_VAULT_PATH` or checking whether the fallback path exists. Once the path is known, switch back to file tools.

## Read a note

Use `host_read` with the resolved absolute path to the note. Prefer this over `cat` because it provides line numbers and pagination.

## List notes

Use `host_shell`（`rg` / `grep` / `find`） with `target: "files"` and the resolved vault path. Prefer this over `find` or `ls`.

- To list all markdown notes, use `pattern: "*.md"` under the vault path.
- To list a subfolder, search under that subfolder's absolute path.

## Search

Use `host_shell`（`rg` / `grep` / `find`） for both filename and content searches. Prefer this over `grep`, `find`, or `ls`.

- For filenames, use `host_shell`（`rg` / `grep` / `find`） with `target: "files"` and a filename `pattern`.
- For note contents, use `host_shell`（`rg` / `grep` / `find`） with `target: "content"`, the content regex as `pattern`, and `file_glob: "*.md"` when you want to restrict matches to markdown notes.

## Create a note

Use `host_write` with the resolved absolute path and the full markdown content. Prefer this over shell heredocs or `echo` because it avoids shell quoting issues and returns structured results.

## Append to a note

Prefer a native file-tool workflow when it is not awkward:

- Read the target note with `host_read`.
- Use `host_write`（先 `host_read` 再整文件写回，细节见 `coding-edit`） for an anchored append when there is stable context, such as adding a section after an existing heading or appending before a known trailing block.
- Use `host_write` when rewriting the whole note is clearer than constructing a fragile patch.

For an anchored append with `host_write`（先 `host_read` 再整文件写回，细节见 `coding-edit`）, replace the anchor with the anchor plus the new content.

For a simple append with no stable context, `host_shell` is acceptable if it is the clearest safe option.

## Targeted edits

Use `host_write`（先 `host_read` 再整文件写回，细节见 `coding-edit`） for focused note changes when the current content gives you stable context. Prefer this over shell text rewriting.

## Wikilinks

Obsidian links notes with `[[Note Name]]` syntax. When creating notes, use these to link related content.
