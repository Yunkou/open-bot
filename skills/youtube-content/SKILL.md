---
name: youtube-content
description: Fetch YouTube transcripts and turn them into summaries, chapters, threads or blog posts. Use for YouTube 视频总结 / 字幕.
---

# YouTube Content Tool

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/media/youtube-content`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

## open-bot 运行方式

- **在哪执行**：默认在内部执行环境：先 `sandbox_ensure`，命令用 `sandbox_shell`，文件用 `sandbox_write` / `sandbox_read`。产物写到 `/workspace/out/` 下，回复里写出完整路径即可作为附件卡片给用户下载/预览；对用户只谈结果，不提内部环境或路径细节。
- **包内脚本**：`load_skill` 不会执行脚本。需要时 `load_skill(name="youtube-content", path="scripts/<file>")` 取内容 → `sandbox_write` 到 `/workspace/skills/youtube-content/scripts/` → `sandbox_shell` 运行（缺依赖先 `pip install` / `apt-get` 等）。
- **用户本机文件**：文本可用 `host_read` 读入；需要在用户电脑上直接处理时，改用 `host_shell`（先 `load_skill host-shell`），并确认该机已装依赖。


## When to use

Use when the user shares a YouTube URL or video link, asks to summarize a video, requests a transcript, or wants to extract and reformat content from any YouTube video. Transforms transcripts into structured content (chapters, summaries, threads, blog posts).

Extract transcripts from YouTube videos and convert them into useful formats.

## Setup

在内部执行环境里准备一个独立 venv（`sandbox_ensure` 后用 `sandbox_shell`）：

```bash
python3 -m venv /workspace/.venv-yt && /workspace/.venv-yt/bin/pip install -q youtube-transcript-api
/workspace/.venv-yt/bin/python -c "import youtube_transcript_api; print(youtube_transcript_api.__file__)"
```

再把脚本放进去：`load_skill(name="youtube-content", path="scripts/fetch_transcript.py")` → `sandbox_write` 到 `/workspace/skills/youtube-content/scripts/fetch_transcript.py`。下文 `SKILL_DIR` 即 `/workspace/skills/youtube-content`，`python` 即 `/workspace/.venv-yt/bin/python`。

YouTube 可能拦截数据中心 IP（报 `RequestBlocked` / `IpBlocked`）。这时改在用户电脑上跑：`list_machines` → 同样的 venv + 脚本用 `host_write` / `host_shell` 放到 `~/.open-bot/skills/youtube-content/`。

## Helper Script

`SKILL_DIR` is the directory containing this SKILL.md file. The script accepts any standard YouTube URL format, short links (youtu.be), shorts, embeds, live links, or a raw 11-character video ID.

```bash
# JSON output with metadata
python SKILL_DIR/scripts/fetch_transcript.py "https://youtube.com/watch?v=VIDEO_ID"

# Plain text (good for piping into further processing)
python SKILL_DIR/scripts/fetch_transcript.py "URL" --text-only

# With timestamps
python SKILL_DIR/scripts/fetch_transcript.py "URL" --timestamps

# Specific language with fallback chain
python SKILL_DIR/scripts/fetch_transcript.py "URL" --language tr,en
```

## Output Formats

After fetching the transcript, format it based on what the user asks for:

- **Chapters**: Group by topic shifts, output timestamped chapter list
- **Summary**: Concise 5-10 sentence overview of the entire video
- **Chapter summaries**: Chapters with a short paragraph summary for each
- **Thread**: Twitter/X thread format — numbered posts, each under 280 chars
- **Blog post**: Full article with title, sections, and key takeaways
- **Quotes**: Notable quotes with timestamps

### Example — Chapters Output

```
00:00 Introduction — host opens with the problem statement
03:45 Background — prior work and why existing solutions fall short
12:20 Core method — walkthrough of the proposed approach
24:10 Results — benchmark comparisons and key takeaways
31:55 Q&A — audience questions on scalability and next steps
```

## Workflow

1. **Fetch** the transcript using `sandbox_shell` and the venv Python with `--text-only --timestamps`.
2. **Validate**: confirm the output is non-empty and in the expected language. If empty, retry without `--language` to get any available transcript. If still empty, tell the user the video likely has transcripts disabled.
3. **Chunk if needed**: if the transcript exceeds ~50K characters, split into overlapping chunks (~40K with 2K overlap) and summarize each chunk before merging.
4. **Transform** into the requested output format. If the user did not specify a format, default to a summary.
5. **Verify**: re-read the transformed output to check for coherence, correct timestamps, and completeness before presenting.

## Error Handling

- **Transcript disabled**: tell the user; suggest they check if subtitles are available on the video page.
- **Private/unavailable video**: relay the error and ask the user to verify the URL.
- **No matching language**: retry without `--language` to fetch any available transcript, then note the actual language to the user.
- **Dependency missing**: re-run the venv setup above and make sure the command uses `/workspace/.venv-yt/bin/python`.
