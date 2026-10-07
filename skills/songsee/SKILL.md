---
name: songsee
description: Audio spectrograms and features (mel, chroma, MFCC) with the songsee CLI. Use to visualize or analyze an audio file.
---

# songsee

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/media/songsee`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

## open-bot 运行方式

- **在哪执行**：用户已连接的电脑。先 `list_machines` 选机，再 `load_skill host-shell`；命令用 `host_shell`（一次一条，stdout 会截断），读写文件用 `host_read` / `host_write`，搜文件内容用 `host_shell` 跑 `rg` / `grep`，查「最大/某类文件」用 `host-file-query`。
- **长进程**：没有后台进程工具。用 `nohup <cmd> > /tmp/<name>.log 2>&1 &` 启动，再用 `host_shell` 跑 `tail -n 50 /tmp/<name>.log` 轮询；整体耗时很长的任务用 `defer_work` 放后台交付。
- **确认**：写入/删除/有风险的命令由系统确认卡处理；结果 waiting = 尚未执行，denied = 用户拒绝，如实说明，不要编造输出。


Generate spectrograms and multi-panel audio feature visualizations from audio files.

## Prerequisites

Requires [Go](https://go.dev/doc/install):
```bash
go install github.com/steipete/songsee/cmd/songsee@latest
```

Optional: `ffmpeg` for formats beyond WAV/MP3.

## Quick Start

```bash
# Basic spectrogram
songsee track.mp3

# Save to specific file
songsee track.mp3 -o spectrogram.png

# Multi-panel visualization grid
songsee track.mp3 --viz spectrogram,mel,chroma,hpss,selfsim,loudness,tempogram,mfcc,flux

# Time slice (start at 12.5s, 8s duration)
songsee track.mp3 --start 12.5 --duration 8 -o slice.jpg

# From stdin
cat track.mp3 | songsee - --format png -o out.png
```

## Visualization Types

Use `--viz` with comma-separated values:

| Type | Description |
|------|-------------|
| `spectrogram` | Standard frequency spectrogram |
| `mel` | Mel-scaled spectrogram |
| `chroma` | Pitch class distribution |
| `hpss` | Harmonic/percussive separation |
| `selfsim` | Self-similarity matrix |
| `loudness` | Loudness over time |
| `tempogram` | Tempo estimation |
| `mfcc` | Mel-frequency cepstral coefficients |
| `flux` | Spectral flux (onset detection) |

Multiple `--viz` types render as a grid in a single image.

## Common Flags

| Flag | Description |
|------|-------------|
| `--viz` | Visualization types (comma-separated) |
| `--style` | Color palette: `classic`, `magma`, `inferno`, `viridis`, `gray` |
| `--width` / `--height` | Output image dimensions |
| `--window` / `--hop` | FFT window and hop size |
| `--min-freq` / `--max-freq` | Frequency range filter |
| `--start` / `--duration` | Time slice of the audio |
| `--format` | Output format: `jpg` or `png` |
| `-o` | Output file path |

## Notes

- WAV and MP3 are decoded natively; other formats require `ffmpeg`
- Output images can be inspected with 看图检查（open-bot 无看图工具：改用 OCR / 文本化检查，或把图片路径给用户确认） for automated audio analysis
- Useful for comparing audio outputs, debugging synthesis, or documenting audio processing pipelines
