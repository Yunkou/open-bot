package httpserver

import (
	"path/filepath"
	"strings"
	"unicode"
)

// Read-only host_shell soft-allow (Phase D).
// Deny-by-default: only simple pipelines of allowlisted commands skip chat confirm.
// Skill scripts (bash/sh script.sh), terminal=true, ssh_exec, and anything with
// redirects / substitutions / dangerous tokens still require confirmation.

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

// Substrings that always force confirm (checked case-insensitively on the raw command).
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

func hostExecNeedsChatConfirm(op, path, dest string) bool {
	if !hostOpNeedsConfirm(op) {
		return false
	}
	if op == "shell" {
		if strings.TrimSpace(dest) == "terminal" {
			return true
		}
		if isReadonlyShellCommand(path) {
			return false
		}
	}
	return true
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
