import { hostExecWebSocketUrl } from "../api";
import {
  readCachedUserSettings,
  writeCachedUserSettings,
  type UserSettings,
  type AutoReviewRule,
  normalizeMachineExecPolicy,
  type MachineExecPolicy,
} from "./userSettings";

export const HOST_WRITES_KEY = "openbot_host_writes";

export type HostExecRequest = {
  type: "exec";
  req_id: string;
  op: "ls" | "read" | "write" | "delete" | "move" | string;
  path?: string;
  dest?: string;
  content?: string;
  conversation_id?: string;
  ssh_host?: string;
  ssh_user?: string;
  ssh_port?: number;
  ssh_fingerprint?: string;
  /** Optional shallow-list filters for host_ls. */
  limit?: number;
  sort?: string;
  glob?: string;
  /** API already got allow/deny in chat; execute without asking again. */
  preconfirmed?: boolean;
};

type TauriInvoke = (cmd: string, args?: Record<string, unknown>) => Promise<unknown>;

function tauriInvoke(): TauriInvoke | null {
  const internals = (window as Window & { __TAURI_INTERNALS__?: { invoke?: TauriInvoke } })
    .__TAURI_INTERNALS__;
  return internals?.invoke ?? null;
}

export function hostWritesEnabled(): boolean {
  try {
    return localStorage.getItem(HOST_WRITES_KEY) !== "0";
  } catch {
    return true;
  }
}

export function setHostWritesEnabled(on: boolean): void {
  try {
    localStorage.setItem(HOST_WRITES_KEY, on ? "1" : "0");
  } catch {
    /* ignore */
  }
}


export const HOST_SHELL_READONLY_AUTO_KEY = "openbot_host_shell_readonly_auto";

/** Legacy kill-switch. Off forces host exec to confirm (same as Auto-review off). */
export function hostShellReadonlyAutoEnabled(): boolean {
  try {
    return localStorage.getItem(HOST_SHELL_READONLY_AUTO_KEY) !== "0";
  } catch {
    return true;
  }
}

export function setHostShellReadonlyAutoEnabled(on: boolean): void {
  try {
    localStorage.setItem(HOST_SHELL_READONLY_AUTO_KEY, on ? "1" : "0");
  } catch {
    /* ignore */
  }
}

function baseName(tok: string): string {
  const norm = tok.replace(/\\/g, "/");
  const i = norm.lastIndexOf("/");
  return i >= 0 ? norm.slice(i + 1) : norm;
}

function takeToken(s: string): [string, string] {
  const trimmed = s.replace(/^\s+/, "");
  if (!trimmed) return ["", ""];
  if (trimmed[0] === "'" || trimmed[0] === '"') {
    const q = trimmed[0];
    let end = 1;
    while (end < trimmed.length && trimmed[end] !== q) end++;
    if (end < trimmed.length) return [trimmed.slice(1, end), trimmed.slice(end + 1)];
    return [trimmed.slice(1), ""];
  }
  let end = 0;
  while (end < trimmed.length && !/\s/.test(trimmed[end]!)) end++;
  return [trimmed.slice(0, end), trimmed.slice(end)];
}

function firstShellToken(s: string): string {
  let rest = s;
  for (;;) {
    const [tok, next] = takeToken(rest);
    if (!tok) return "";
    if (tok.includes("=") && !tok.startsWith("-") && !tok.includes("/")) {
      rest = next;
      continue;
    }
    return tok;
  }
}

function splitShellSegments(cmd: string): string[] {
  const out: string[] = [];
  let cur = "";
  let inSingle = false;
  let inDouble = false;
  for (let i = 0; i < cmd.length; i++) {
    const c = cmd[i]!;
    if (c === "'" && !inDouble) {
      inSingle = !inSingle;
      cur += c;
      continue;
    }
    if (c === '"' && !inSingle) {
      inDouble = !inDouble;
      cur += c;
      continue;
    }
    if (!inSingle && !inDouble) {
      if (c === "|" || c === ";") {
        if (c === "|" && cmd[i + 1] === "|") {
          const seg = cur.trim();
          if (seg) out.push(seg);
          cur = "";
          i++;
          continue;
        }
        const seg = cur.trim();
        if (seg) out.push(seg);
        cur = "";
        continue;
      }
      if (c === "&" && cmd[i + 1] === "&") {
        const seg = cur.trim();
        if (seg) out.push(seg);
        cur = "";
        i++;
        continue;
      }
    }
    cur += c;
  }
  const seg = cur.trim();
  if (seg) out.push(seg);
  return out;
}

function hasNonNullWriteRedirect(cmd: string): boolean {
  let inSingle = false;
  let inDouble = false;
  for (let i = 0; i < cmd.length; i++) {
    const c = cmd[i]!;
    if (c === "'" && !inDouble) {
      inSingle = !inSingle;
      continue;
    }
    if (c === '"' && !inSingle) {
      inDouble = !inDouble;
      continue;
    }
    if (inSingle || inDouble || c !== ">") continue;
    let j = i;
    while (j < cmd.length && cmd[j] === ">") j++;
    const target = firstShellToken(cmd.slice(j));
    if (target !== "/dev/null" && target !== "nul") return true;
    i = j - 1;
  }
  return false;
}


const SHELL_CONFIRM_SUBSTR = [
  "$(", "`", "$((", "<<",
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
  "git push",
  "npm install", "npm ci", "pnpm install", "pnpm add",
  "yarn add", "yarn install",
  "pip install", "pip3 install", "pipx install",
  "brew install", "brew uninstall",
  "apt install", "apt-get install", "apt remove", "apt-get remove",
  "yum install", "dnf install",
  "cargo install", "go install", "gem install",
  "snap install", "flatpak install",
  "choco install", "winget install", "conda install",
  "bun install", "bun add",
];

const SHELL_RISKY_BINS = new Set([
  "rm", "mv", "cp",
  "chmod", "chown", "chgrp", "unlink",
  "mkdir", "rmdir", "touch", "ln", "dd",
  "tee", "truncate", "shred",
  "sudo", "doas", "pkexec",
  "ssh", "scp", "sftp",
  "curl", "wget",
  "kill", "pkill", "killall",
  "nc", "ncat", "netcat",
  "reboot", "shutdown", "halt",
  "eval", "source", "exec",
]);

function flagTakesValue(flag: string): boolean {
  switch (flag) {
    case "-C":
    case "-c":
    case "--cwd":
    case "--prefix":
    case "--directory":
    case "-t":
    case "--target":
    case "--git-dir":
    case "--work-tree":
    case "--namespace":
    case "--config":
    case "--registry":
    case "--cache":
    case "--file":
    case "-f":
    case "--requirement":
      return true;
    default:
      return false;
  }
}

function shellArgs(seg: string): string[] {
  const args: string[] = [];
  let rest = seg.trim();
  for (;;) {
    const [tok, next] = takeToken(rest);
    if (!tok) break;
    rest = next;
    if (args.length === 0 && tok.includes("=") && !tok.startsWith("-") && !tok.includes("/")) continue;
    args.push(tok);
  }
  return args;
}

function positionals(args: string[]): string[] {
  const out: string[] = [];
  for (let i = 0; i < args.length; i++) {
    const a = args[i]!;
    if (a === "--") {
      out.push(...args.slice(i + 1));
      break;
    }
    if (flagTakesValue(a)) {
      i++;
      continue;
    }
    if (a.startsWith("-")) continue;
    out.push(a);
  }
  return out;
}

function segmentGitPush(args: string[]): boolean {
  if (!args.length || baseName(args[0]!).toLowerCase() !== "git") return false;
  for (let i = 1; i < args.length; i++) {
    const a = args[i]!;
    if (flagTakesValue(a)) {
      i++;
      continue;
    }
    if (a.startsWith("-")) continue;
    return a.toLowerCase() === "push";
  }
  return false;
}

function firstPositionalIn(args: string[], verbs: string[]): boolean {
  const pos = positionals(args);
  if (!pos.length) return false;
  const head = pos[0]!.toLowerCase();
  return verbs.some((v) => v === head);
}

function npmStyleInstall(args: string[]): boolean {
  const pos = positionals(args);
  if (!pos.length) return false;
  switch (pos[0]!.toLowerCase()) {
    case "install":
    case "i":
    case "add":
    case "ci":
    case "uninstall":
    case "un":
    case "remove":
    case "rm":
    case "update":
    case "upgrade":
      return true;
    default:
      return false;
  }
}

function pacmanMutating(args: string[]): boolean {
  for (const a of args) {
    switch (a.toLowerCase()) {
      case "-s":
      case "-sy":
      case "-syu":
      case "-syy":
      case "-syyu":
      case "-su":
      case "-u":
      case "-r":
      case "-rn":
      case "-rns":
      case "-runs":
      case "--sync":
      case "--remove":
      case "--upgrade":
        return true;
      default:
        break;
    }
  }
  return false;
}

function segmentPackageInstall(args: string[]): boolean {
  if (!args.length) return false;
  const base = baseName(args[0]!).toLowerCase();
  const rest = args.slice(1);
  switch (base) {
    case "go":
      return firstPositionalIn(rest, ["install"]);
    case "npm":
    case "pnpm":
    case "bun":
      return npmStyleInstall(rest);
    case "yarn":
      if (positionals(rest).length === 0) return true;
      return npmStyleInstall(rest);
    case "pip":
    case "pip3":
    case "pipx":
    case "brew":
    case "cargo":
    case "gem":
    case "conda":
    case "choco":
    case "winget":
    case "snap":
    case "flatpak":
    case "composer":
      return firstPositionalIn(rest, ["install", "uninstall", "remove", "upgrade", "add", "require"]);
    case "apt":
    case "apt-get":
    case "yum":
    case "dnf":
    case "zypper":
      return firstPositionalIn(rest, ["install", "remove", "purge", "autoremove", "upgrade", "dist-upgrade"]);
    case "pacman":
      return pacmanMutating(rest);
    default:
      return false;
  }
}

function shellSegmentAutoOK(seg: string): boolean {
  const s = seg.trim();
  if (!s || s.endsWith("&")) return false;
  const args = shellArgs(s);
  if (!args.length) return false;
  const base = baseName(args[0]!).toLowerCase();
  if (SHELL_RISKY_BINS.has(base)) return false;
  if (segmentGitPush(args) || segmentPackageInstall(args)) return false;
  return true;
}

/** Mirror of API hostShellAutoEligible — no command allowlist; risky patterns confirm. */
export function hostShellAutoEligible(command: string): boolean {
  const cmd = command.trim();
  if (!cmd || cmd.length > 2000) return false;
  const lower = cmd.toLowerCase();
  for (const bad of SHELL_CONFIRM_SUBSTR) {
    if (lower.includes(bad.toLowerCase())) return false;
  }
  if (hasNonNullWriteRedirect(cmd)) return false;
  const segments = splitShellSegments(cmd);
  if (segments.length === 0) return false;
  for (const seg of segments) {
    if (!shellSegmentAutoOK(seg)) return false;
  }
  return true;
}

/** Grok-aligned Auto-review tiers: deterministic, no LLM judgment. */
export type HostExecReviewTier = "auto" | "confirm" | "deny";

export type HostExecReview = {
  tier: HostExecReviewTier;
  reason: string;
  code: string;
};

const HARD_DENY_PIPE_SHELL = [
  "|sh", "| sh", "|bash", "| bash", "|zsh", "| zsh", "|dash", "| dash", "|fish", "| fish",
];

function isWipeRootCommand(lower: string): boolean {
  for (const prefix of ["rm -rf ", "rm -fr "]) {
    let idx = lower.indexOf(prefix);
    while (idx >= 0) {
      const rest = lower.slice(idx + prefix.length).replace(/^[ \t]+/, "");
      if (rest === "/" || rest === "/*") return true;
      if (rest.startsWith("/")) {
        const after = rest.slice(1);
        if (!after || after === "*") return true;
        const c = after[0];
        if (c === " " || c === "\t" || c === ";" || c === "&" || c === "|" || c === "\n") return true;
      }
      const next = lower.indexOf(prefix, idx + prefix.length);
      if (next < 0) break;
      idx = next;
    }
  }
  return false;
}

function matchHardDenyShell(command: string): HostExecReview | null {
  const lower = command.trim().toLowerCase();
  if (!lower) return null;
  if (isWipeRootCommand(lower)) {
    return { tier: "deny", code: "wipe_root", reason: "Auto-review 硬拒绝：疑似清空根目录" };
  }
  const pipedShell = HARD_DENY_PIPE_SHELL.some((s) => lower.includes(s));
  if (pipedShell && (lower.includes("curl ") || lower.includes("wget "))) {
    return { tier: "deny", code: "pipe_download_shell", reason: "Auto-review 硬拒绝：下载管道进 shell（如 curl|sh）" };
  }
  if (pipedShell) {
    return { tier: "deny", code: "pipe_to_shell", reason: "Auto-review 硬拒绝：管道进 shell 解释器" };
  }
  if (lower.includes(":(){")) {
    return { tier: "deny", code: "fork_bomb", reason: "Auto-review 硬拒绝：疑似 fork bomb" };
  }
  if (lower.includes("mkfs")) {
    return { tier: "deny", code: "format_disk", reason: "Auto-review 硬拒绝：格式化磁盘" };
  }
  if (lower.includes("of=/dev/") || lower.includes(">/dev/sd") || lower.includes("> /dev/sd") ||
      lower.includes(">/dev/disk") || lower.includes("> /dev/disk")) {
    return { tier: "deny", code: "raw_disk_write", reason: "Auto-review 硬拒绝：写入块设备" };
  }
  if (lower.includes("dd if=")) {
    return { tier: "deny", code: "dd_image", reason: "Auto-review 硬拒绝：dd 磁盘镜像类命令" };
  }
  return null;
}


let cachedUserSettings: UserSettings = readCachedUserSettings();
let cachedExecPolicy: MachineExecPolicy = "allow";

/** Sync Auto-review prefs from server/settings UI into the host-exec gate. */
export function setHostExecUserSettings(settings: UserSettings): void {
  cachedUserSettings = settings;
  writeCachedUserSettings(settings);
}

export function getHostExecUserSettings(): UserSettings {
  return cachedUserSettings;
}

/** Per-machine exec policy for the currently connected desktop host. */
export function setHostExecMachinePolicy(policy: MachineExecPolicy | string): void {
  cachedExecPolicy = normalizeMachineExecPolicy(policy);
}

export function getHostExecMachinePolicy(): MachineExecPolicy {
  return cachedExecPolicy;
}

function buildAutoReviewHaystack(op: string, path: string, dest: string, reason: string): string {
  const labels: Record<string, string> = {
    ls: "本机只读 打开文件 列表",
    read: "本机只读 打开文件 列表",
    open: "本机只读 打开文件 列表",
    write: "写入本机文件",
    delete: "删除本机文件",
    move: "移动重命名",
    shell: "本机命令 shell 运行命令",
    ssh_ls: "远程只读",
    ssh_read: "远程只读",
    ssh_write: "远程写入 远程删除 远程命令",
    ssh_delete: "远程写入 远程删除 远程命令",
    ssh_exec: "远程写入 远程删除 远程命令",
  };
  const parts = [op, path, dest, reason, labels[op] || op];
  if (dest === "terminal") parts.push("终端", "terminal");
  return parts.join(" ").toLowerCase();
}

function ruleMatchesHaystack(when: string, haystack: string): boolean {
  let w = when.trim().toLowerCase();
  if (!w) return false;
  if (haystack.includes(w)) return true;
  for (const sep of [" ", "\t", "，", ",", "、", "/", "|", "；", ";", "：", ":", "。"]) {
    w = w.split(sep).join(" ");
  }
  for (const tok of w.split(/\s+/)) {
    const t = tok.trim();
    if ([...t].length < 2) continue;
    if (haystack.includes(t)) return true;
  }
  return false;
}

function matchUserRules(rules: AutoReviewRule[], haystack: string): { ask: boolean; allow: boolean } {
  let ask = false;
  let allow = false;
  for (const r of rules) {
    if (!ruleMatchesHaystack(r.when || "", haystack)) continue;
    if (r.action === "ask_first") ask = true;
    if (r.action === "auto_allow") allow = true;
  }
  return { ask, allow };
}

/** Apply per-user Auto-review prefs on top of built-in tiers. Hard deny always wins. */
export function applyUserAutoReview(
  base: HostExecReview,
  settings: UserSettings,
  op: string,
  path = "",
  dest = "",
): HostExecReview {
  if (base.tier === "deny") return base;
  if (!settings.auto_review_enabled) {
    if (base.tier === "auto") {
      return { tier: "confirm", reason: "自动审核已关闭，需你确认", code: "auto_review_off" };
    }
    return base;
  }
  const haystack = buildAutoReviewHaystack(op, path, dest, base.reason);
  const { ask, allow } = matchUserRules(settings.auto_review_rules || [], haystack);
  if (ask) {
    if (base.tier === "auto") {
      return { tier: "confirm", reason: "用户规则：先询问", code: "user_rule_ask_first" };
    }
    return base;
  }
  if (allow && base.tier === "confirm") {
    return { tier: "auto", reason: "用户规则：自动允许", code: "user_rule_auto_allow" };
  }
  return base;
}

export function classifyHostExecReviewForUser(
  op: string,
  path = "",
  dest = "",
  settings: UserSettings = cachedUserSettings,
): HostExecReview {
  return applyUserAutoReview(classifyHostExecReview(op, path, dest), settings, op, path, dest);
}

/** Mirror of API classifyHostExecReview — keep in sync with host_exec_review.go. */
export function classifyHostExecReview(op: string, path = "", dest = ""): HostExecReview {
  const o = op.trim();
  const p = path.trim();
  const d = dest.trim();
  switch (o) {
    case "ls":
    case "read":
    case "ssh_ls":
    case "ssh_read":
    case "open":
      return { tier: "auto", reason: "只读/可逆操作，Auto-review 自动放行", code: "readonly_op" };
    case "write":
      return { tier: "confirm", reason: "写入本机文件，需你确认", code: "write" };
    case "delete":
      return { tier: "confirm", reason: "删除本机文件，需你确认", code: "delete" };
    case "move":
      return { tier: "confirm", reason: "移动或重命名，需你确认", code: "move" };
    case "ssh_write":
      return { tier: "confirm", reason: "写入远程文件，需你确认", code: "ssh_write" };
    case "ssh_delete":
      return { tier: "confirm", reason: "删除远程文件，需你确认", code: "ssh_delete" };
    case "ssh_exec":
      return { tier: "confirm", reason: "远程执行命令，需你确认", code: "ssh_exec" };
    case "shell":
      if (d === "terminal") {
        return { tier: "confirm", reason: "将打开终端窗口，需你确认", code: "terminal" };
      }
      {
        const hard = matchHardDenyShell(p);
        if (hard) return hard;
      }
      if (hostShellAutoEligible(p)) {
        return { tier: "auto", reason: "未发现写入或危险模式，Auto-review 自动放行", code: "shell_auto" };
      }
      return { tier: "confirm", reason: "命令看起来会改动系统或有风险，需你确认", code: "shell_confirm" };
    default:
      return { tier: "confirm", reason: "未知操作，需你确认", code: "unknown_op" };
  }
}


function isWriteOp(op: string): boolean {
  return op === "write" || op === "delete" || op === "move" || op === "ssh_write" || op === "ssh_delete" || op === "ssh_exec";
}

async function runOp(req: HostExecRequest): Promise<Record<string, unknown>> {
  const invoke = tauriInvoke();
  if (!invoke) {
    return { ok: false, error: "当前窗口不能操作本机文件" };
  }
  const path = req.path || "";
  if (req.op === "ls") {
    return (await invoke("host_ls", {
      path,
      limit: req.limit,
      sort: req.sort,
      glob: req.glob,
    })) as Record<string, unknown>;
  }
  if (req.op === "read") return (await invoke("host_read", { path })) as Record<string, unknown>;
  if (req.op === "write") {
    return (await invoke("host_write", { path, content: req.content || "" })) as Record<string, unknown>;
  }
  if (req.op === "delete") return (await invoke("host_delete", { path })) as Record<string, unknown>;
  if (req.op === "move") {
    return (await invoke("host_move", { path, dest: req.dest || "" })) as Record<string, unknown>;
  }
  if (req.op === "open") return (await invoke("host_open", { name: path })) as Record<string, unknown>;
  if (req.op === "shell") {
    return (await invoke("host_shell", {
      command: path,
      terminal: req.dest === "terminal",
    })) as Record<string, unknown>;
  }
  if (req.op.startsWith("ssh_")) {
    const args = {
      host: req.ssh_host || "",
      user: req.ssh_user || "",
      port: req.ssh_port || 0,
      path,
    };
    if (req.op === "ssh_ls") return (await invoke("host_ssh_ls", args)) as Record<string, unknown>;
    if (req.op === "ssh_read") return (await invoke("host_ssh_read", args)) as Record<string, unknown>;
    if (req.op === "ssh_write") {
      return (await invoke("host_ssh_write", { ...args, content: req.content || "" })) as Record<string, unknown>;
    }
    if (req.op === "ssh_delete") return (await invoke("host_ssh_delete", args)) as Record<string, unknown>;
    if (req.op === "ssh_exec") {
      return (await invoke("host_ssh_exec", {
        host: args.host,
        user: args.user,
        port: args.port,
        command: path,
      })) as Record<string, unknown>;
    }
  }
  return { ok: false, error: "不支持的操作" };
}

async function probeSsh(req: HostExecRequest): Promise<Record<string, unknown>> {
  const invoke = tauriInvoke();
  if (!invoke) return { ok: false, error: "当前窗口不能连接远程主机" };
  try {
    return (await invoke("host_ssh_probe", {
      host: req.ssh_host || "",
      user: req.ssh_user || "",
      port: req.ssh_port || 0,
    })) as Record<string, unknown>;
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) };
  }
}

/** Client Auto-review gate: machine exec_policy + user settings + built-in tiers. */
function reviewForRequest(req: HostExecRequest): HostExecReview {
  const policy = cachedExecPolicy;
  if (policy === "deny") {
    return {
      tier: "deny",
      reason: "这台电脑已设置为不允许执行",
      code: "exec_policy_deny",
    };
  }
  let settings = cachedUserSettings;
  if (policy === "ask") {
    settings = { ...settings, auto_review_enabled: false };
  }
  // Legacy kill-switch: if localStorage readonly auto is off, treat as auto_review off for shell.
  if (!hostShellReadonlyAutoEnabled()) {
    settings = { ...settings, auto_review_enabled: false };
  }
  const rev = classifyHostExecReviewForUser(req.op, req.path || "", req.dest || "", settings);
  return rev;
}

async function refineWriteReview(req: HostExecRequest, base: HostExecReview): Promise<HostExecReview> {
  if (req.op !== "write") return base;
  const invoke = tauriInvoke();
  if (!invoke) return base;
  try {
    const stat = (await invoke("host_stat", { path: req.path || "" })) as {
      exists?: boolean;
      is_dir?: boolean;
      under_home?: boolean;
    };
    if (stat.under_home === false) {
      return { tier: "confirm", reason: "主目录外写入，需你确认", code: "write_outside_home" };
    }
    if (stat.exists && !stat.is_dir) {
      return { tier: "confirm", reason: "覆盖已有文件，需你确认", code: "write_overwrite" };
    }
    // New file under home: still confirm via API hostOpNeedsConfirm; keep confirm.
    return base;
  } catch {
    return base;
  }
}

async function needsConfirm(req: HostExecRequest): Promise<boolean> {
  let rev = reviewForRequest(req);
  if (rev.code === "write") rev = await refineWriteReview(req, rev);
  return rev.tier === "confirm";
}

function hardDenyReview(req: HostExecRequest): HostExecReview | null {
  const rev = reviewForRequest(req);
  return rev.tier === "deny" ? rev : null;
}

export function startHostExecSession(opts: {
  token: string;
  machineId: string;
  confirm: (req: HostExecRequest) => Promise<boolean>;
}): () => void {
  let stopped = false;
  let socket: WebSocket | null = null;
  let retry: ReturnType<typeof setTimeout> | null = null;

  const connect = () => {
    if (stopped) return;
    const ws = new WebSocket(hostExecWebSocketUrl(opts.machineId, opts.token));
    socket = ws;
    ws.onmessage = (ev) => {
      void (async () => {
        let req: HostExecRequest;
        try {
          req = JSON.parse(String(ev.data)) as HostExecRequest;
        } catch {
          return;
        }
        if (req.type !== "exec" || !req.req_id) return;
        let result: Record<string, unknown>;
        try {
          const hard = hardDenyReview(req);
          if (hard) {
            result = {
              ok: false,
              denied: true,
              auto_review: hard.code === "exec_policy_deny" ? undefined : "deny",
              exec_policy: hard.code === "exec_policy_deny" ? "deny" : undefined,
              review_code: hard.code,
              error: hard.reason,
            };
          } else if (isWriteOp(req.op) && !hostWritesEnabled()) {
            result = {
              ok: false,
              error: req.op.startsWith("ssh_") ? "写入已关闭，远程写入、删除和命令也停着" : "本机写入已关闭",
            };
          } else if (req.preconfirmed) {
            result = await runOp(req);
            result.confirmed = true;
          } else if (await needsConfirm(req)) {
            let confirmReq = req;
            let blocked: Record<string, unknown> | null = null;
            if (req.op.startsWith("ssh_")) {
              const probe = await probeSsh(req);
              if (probe.ok !== true) {
                blocked = probe;
              } else if (probe.host_key_status === "changed") {
                blocked = {
                  ok: false,
                  error: `主机密钥和 known_hosts 不一致（${String(probe.fingerprint || "")}）。连接已断开`,
                };
              } else if (probe.host_key_status === "unknown" && probe.fingerprint) {
                confirmReq = { ...req, ssh_fingerprint: String(probe.fingerprint) };
              }
            }
            if (blocked) {
              result = blocked;
            } else {
              const allowed = await opts.confirm(confirmReq);
              if (!allowed) {
                result = { ok: false, denied: true, error: "用户拒绝了这次操作" };
              } else {
                result = await runOp(req);
                result.confirmed = true;
              }
            }
          } else {
            result = await runOp(req);
          }
        } catch (err) {
          result = { ok: false, error: err instanceof Error ? err.message : String(err) };
        }
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: "exec_result", req_id: req.req_id, ...result }));
        }
      })();
    };
    ws.onclose = () => {
      if (stopped) return;
      retry = setTimeout(connect, 2000);
    };
  };
  connect();
  return () => {
    stopped = true;
    if (retry) clearTimeout(retry);
    socket?.close();
  };
}
