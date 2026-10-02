package httpserver

import (
	"path/filepath"
	"strings"
	"unicode"
)

// Host-exec Auto-review (Grok Bot–aligned): deterministic tiers, no LLM judgment.
//
//  1. hard deny  — clearly dangerous patterns rejected without a confirm card
//  2. auto       — read-only / reversible ops skip confirm (Phase D allowlist + ls/read)
//  3. confirm    — everything else needs the user (chat card), with a structured reason
//
// Settings kill-switch on the desktop client can force shell auto → confirm locally;
// the API always applies the same deny/auto/confirm table.

type hostExecReviewTier string

const (
	hostExecReviewAuto    hostExecReviewTier = "auto"
	hostExecReviewConfirm hostExecReviewTier = "confirm"
	hostExecReviewDeny    hostExecReviewTier = "deny"
)

type hostExecReview struct {
	Tier   hostExecReviewTier
	Reason string // short, user-visible / card copy
	Code   string // stable machine code for tests / logs
}

// Read-only host_shell soft-allow (Phase D).
// Deny-by-default: only simple pipelines of allowlisted commands skip chat confirm.
// Skill scripts (bash/sh script.sh), terminal=true, ssh_exec, and anything with
// redirects / substitutions / dangerous tokens still require confirmation (or hard deny).

var readonlyShellAllow = map[string]struct{}{
	"ls": {}, "find": {}, "du": {}, "stat": {},
	"md5": {}, "md5sum": {}, "shasum": {}, "sha1sum": {}, "sha256sum": {},
	"wc": {}, "cat": {}, "head": {}, "tail": {}, "file": {},
	"pwd": {}, "which": {}, "type": {}, "dirname": {}, "basename": {},
	"realpath": {}, "readlink": {}, "uname": {}, "date": {}, "whoami": {},
	"id": {}, "df": {}, "hostname": {}, "echo": {}, "printf": {},
	"true": {}, "false": {}, "test": {}, "[": {},
	"grep": {}, "egrep": {}, "fgrep": {},
	"sort": {}, "uniq": {}, "cut": {}, "tr": {}, "awk": {},
	"tree": {}, "arch": {}, "sw_vers": {}, "printenv": {}, "locale": {},
}

// Substrings that exclude a command from the read-only allowlist (forces confirm
// unless a harder deny rule matches first). Checked case-insensitively on the raw command.
var readonlyShellDenySubstrings = []string{
	"$(", "`", // command substitution
	"$((",     // arithmetic (rarely needed; avoid cleverness)
	"<<",      // heredoc
	" -exec", "-exec ", "-ok ", " -ok", "-delete",
	"sudo", "doas", " pkexec",
	"|sh", "| sh", "|bash", "| bash", "|zsh", "| zsh", "|dash", "| dash",
	"|fish", "| fish",
	"curl ", "wget ", " nc ", "ncat ", "netcat ",
	"ssh ", "scp ", "sftp ",
	"rm ", "rm\t", "mv ", "mv\t", "cp ", "cp\t",
	"chmod ", "chown ", "chgrp ", "unlink ",
	"mkdir ", "rmdir ", "touch ", "ln ", "dd ",
	"tee ", "truncate ", "shred ",
	"kill ", "pkill ", "killall ",
	"reboot", "shutdown", "halt ",
	"eval ", "source ", " exec ",
	"sed -i", "perl -i", "ruby -i",
}

// Hard-deny patterns: never execute, no confirm card (Auto-review hard block).
// Keep this list narrow — ordinary deletes/writes stay in the confirm tier.
type hardDenyRule struct {
	substr string
	code   string
	reason string
}

var hostExecHardDenyRules = []hardDenyRule{
	{substr: "curl ", code: "pipe_download_shell", reason: "Auto-review 硬拒绝：下载管道进 shell（如 curl|sh）"},
	{substr: "wget ", code: "pipe_download_shell", reason: "Auto-review 硬拒绝：下载管道进 shell（如 wget|sh）"},
	{substr: "|sh", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "| sh", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "|bash", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "| bash", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "|zsh", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "| zsh", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "|dash", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "| dash", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "|fish", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: "| fish", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器"},
	{substr: ":(){", code: "fork_bomb", reason: "Auto-review 硬拒绝：疑似 fork bomb"},
	{substr: "mkfs", code: "format_disk", reason: "Auto-review 硬拒绝：格式化磁盘"},
	{substr: "of=/dev/", code: "raw_disk_write", reason: "Auto-review 硬拒绝：写入块设备"},
	{substr: "> /dev/sd", code: "raw_disk_write", reason: "Auto-review 硬拒绝：写入块设备"},
	{substr: ">/dev/sd", code: "raw_disk_write", reason: "Auto-review 硬拒绝：写入块设备"},
	{substr: "> /dev/disk", code: "raw_disk_write", reason: "Auto-review 硬拒绝：写入块设备"},
	{substr: ">/dev/disk", code: "raw_disk_write", reason: "Auto-review 硬拒绝：写入块设备"},
	{substr: "dd if=", code: "dd_image", reason: "Auto-review 硬拒绝：dd 磁盘镜像类命令"},
}

func classifyHostExecReview(op, path, dest string) hostExecReview {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)
	dest = strings.TrimSpace(dest)

	switch op {
	case "ls", "read", "ssh_ls", "ssh_read", "open":
		return hostExecReview{Tier: hostExecReviewAuto, Reason: "只读/可逆操作，Auto-review 自动放行", Code: "readonly_op"}
	case "write":
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "写入本机文件，需你确认", Code: "write"}
	case "delete":
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "删除本机文件，需你确认", Code: "delete"}
	case "move":
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "移动或重命名，需你确认", Code: "move"}
	case "ssh_write":
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "写入远程文件，需你确认", Code: "ssh_write"}
	case "ssh_delete":
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "删除远程文件，需你确认", Code: "ssh_delete"}
	case "ssh_exec":
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "远程执行命令，需你确认", Code: "ssh_exec"}
	case "shell":
		if dest == "terminal" {
			return hostExecReview{Tier: hostExecReviewConfirm, Reason: "将打开终端窗口，需你确认", Code: "terminal"}
		}
		if hit, rule := matchHardDenyShell(path); hit {
			return hostExecReview{Tier: hostExecReviewDeny, Reason: rule.reason, Code: rule.code}
		}
		if isReadonlyShellCommand(path) {
			return hostExecReview{Tier: hostExecReviewAuto, Reason: "只读 allowlist 命令，Auto-review 自动放行", Code: "readonly_shell"}
		}
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "非只读本机命令（不在 Auto-review allowlist），需你确认", Code: "shell_confirm"}
	default:
		return hostExecReview{Tier: hostExecReviewConfirm, Reason: "未知操作，需你确认", Code: "unknown_op"}
	}
}

func matchHardDenyShell(command string) (bool, hardDenyRule) {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return false, hardDenyRule{}
	}
	if isWipeRootCommand(lower) {
		return true, hardDenyRule{
			code:   "wipe_root",
			reason: "Auto-review 硬拒绝：疑似清空根目录",
		}
	}
	pipedShell := strings.Contains(lower, "|sh") || strings.Contains(lower, "| sh") ||
		strings.Contains(lower, "|bash") || strings.Contains(lower, "| bash") ||
		strings.Contains(lower, "|zsh") || strings.Contains(lower, "| zsh") ||
		strings.Contains(lower, "|dash") || strings.Contains(lower, "| dash") ||
		strings.Contains(lower, "|fish") || strings.Contains(lower, "| fish")
	// Prefer a clearer code when download is piped into a shell.
	if pipedShell && (strings.Contains(lower, "curl ") || strings.Contains(lower, "wget ")) {
		return true, hardDenyRule{
			code:   "pipe_download_shell",
			reason: "Auto-review 硬拒绝：下载管道进 shell（如 curl|sh）",
		}
	}
	for _, rule := range hostExecHardDenyRules {
		if rule.code == "pipe_download_shell" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(rule.substr)) {
			return true, rule
		}
	}
	return false, hardDenyRule{}
}

// isWipeRootCommand matches rm -rf / or rm -rf /* (and -fr), but not rm -rf /tmp.
func isWipeRootCommand(lower string) bool {
	for _, prefix := range []string{"rm -rf ", "rm -fr "} {
		idx := strings.Index(lower, prefix)
		for idx >= 0 {
			rest := strings.TrimLeft(lower[idx+len(prefix):], " \t")
			if rest == "/" || rest == "/*" {
				return true
			}
			if strings.HasPrefix(rest, "/") {
				after := rest[1:]
				if after == "" || after == "*" {
					return true
				}
				// "/" followed by path chars → not wipe root (e.g. /tmp)
				if after[0] == ' ' || after[0] == '\t' || after[0] == ';' || after[0] == '&' || after[0] == '|' || after[0] == '\n' {
					return true
				}
			}
			next := strings.Index(lower[idx+len(prefix):], prefix)
			if next < 0 {
				break
			}
			idx = idx + len(prefix) + next
		}
	}
	return false
}

// hostExecNeedsChatConfirm reports whether the chat confirm card is required.
// Hard-deny ops return false here (they are rejected before confirm).
func hostExecNeedsChatConfirm(op, path, dest string) bool {
	return classifyHostExecReview(op, path, dest).Tier == hostExecReviewConfirm
}

func hostExecHardDenied(op, path, dest string) (bool, hostExecReview) {
	rev := classifyHostExecReview(op, path, dest)
	if rev.Tier == hostExecReviewDeny {
		return true, rev
	}
	return false, rev
}

func hostOpNeedsConfirm(op string) bool {
	switch op {
	case "write", "delete", "move", "shell", "ssh_write", "ssh_delete", "ssh_exec":
		return true
	default:
		return false
	}
}

func isReadonlyShellCommand(command string) bool {
	cmd := strings.TrimSpace(command)
	if cmd == "" || len(cmd) > 2000 {
		return false
	}
	lower := strings.ToLower(cmd)
	for _, bad := range readonlyShellDenySubstrings {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	// Write redirects: allow only >/dev/null and 2>/dev/null style discards.
	if hasNonNullWriteRedirect(cmd) {
		return false
	}
	segments := splitShellSegments(cmd)
	if len(segments) == 0 {
		return false
	}
	for _, seg := range segments {
		if !readonlyShellSegmentOK(seg) {
			return false
		}
	}
	return true
}

func hasNonNullWriteRedirect(cmd string) bool {
	// Scan for > or >> not part of 2> / &> discards to /dev/null.
	// Ignore > inside single/double quotes.
	i := 0
	inSingle, inDouble := false, false
	for i < len(cmd) {
		c := cmd[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			i++
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			i++
			continue
		}
		if inSingle || inDouble || c != '>' {
			i++
			continue
		}
		j := i
		for j < len(cmd) && cmd[j] == '>' {
			j++
		}
		rest := strings.TrimSpace(cmd[j:])
		target := firstShellToken(rest)
		if target != "/dev/null" && target != "nul" {
			return true
		}
		i = j
	}
	return false
}

func splitShellSegments(cmd string) []string {
	// Split on |, ;, &&, || while ignoring those inside simple quotes.
	var out []string
	var b strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			b.WriteByte(c)
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			b.WriteByte(c)
			continue
		}
		if !inSingle && !inDouble {
			if c == '|' || c == ';' {
				if c == '|' && i+1 < len(cmd) && cmd[i+1] == '|' {
					seg := strings.TrimSpace(b.String())
					if seg != "" {
						out = append(out, seg)
					}
					b.Reset()
					i++
					continue
				}
				if c == '|' {
					seg := strings.TrimSpace(b.String())
					if seg != "" {
						out = append(out, seg)
					}
					b.Reset()
					continue
				}
				// ;
				seg := strings.TrimSpace(b.String())
				if seg != "" {
					out = append(out, seg)
				}
				b.Reset()
				continue
			}
			if c == '&' && i+1 < len(cmd) && cmd[i+1] == '&' {
				seg := strings.TrimSpace(b.String())
				if seg != "" {
					out = append(out, seg)
				}
				b.Reset()
				i++
				continue
			}
		}
		b.WriteByte(c)
	}
	seg := strings.TrimSpace(b.String())
	if seg != "" {
		out = append(out, seg)
	}
	return out
}

func readonlyShellSegmentOK(seg string) bool {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return false
	}
	// Reject backgrounding and bare redirects as command.
	if strings.HasSuffix(seg, "&") {
		return false
	}
	tok := firstShellToken(seg)
	if tok == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(tok))
	// Windows-ish path base still works via filepath.Base.
	if _, ok := readonlyShellAllow[base]; !ok {
		return false
	}
	return true
}

func firstShellToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Skip leading VAR=value assignments (rare for read-only; still take next token).
	for {
		tok, rest := takeToken(s)
		if tok == "" {
			return ""
		}
		if strings.Contains(tok, "=") && !strings.HasPrefix(tok, "-") && !strings.Contains(tok, "/") {
			// Likely ENV=val — skip and continue.
			s = rest
			continue
		}
		return tok
	}
}

func takeToken(s string) (tok, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s == "" {
		return "", ""
	}
	if s[0] == '\'' || s[0] == '"' {
		q := s[0]
		end := 1
		for end < len(s) && s[end] != q {
			end++
		}
		if end < len(s) {
			return s[1:end], s[end+1:]
		}
		return s[1:], ""
	}
	end := 0
	for end < len(s) && !unicode.IsSpace(rune(s[end])) {
		end++
	}
	return s[:end], s[end:]
}
