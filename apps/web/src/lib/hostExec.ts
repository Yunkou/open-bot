import { hostExecWebSocketUrl } from "../api";

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

/** Default on: allowlisted read-only host_shell commands skip the chat confirm card. */
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

const READONLY_SHELL_ALLOW = new Set([
  "ls", "find", "du", "stat",
  "md5", "md5sum", "shasum", "sha1sum", "sha256sum",
  "wc", "cat", "head", "tail", "file",
  "pwd", "which", "type", "dirname", "basename",
  "realpath", "readlink", "uname", "date", "whoami",
  "id", "df", "hostname", "echo", "printf",
  "true", "false", "test", "[",
  "grep", "egrep", "fgrep",
  "sort", "uniq", "cut", "tr", "awk",
  "tree", "arch", "sw_vers", "printenv", "locale",
]);

const READONLY_SHELL_DENY_SUBSTR = [
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
];

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

/** Mirror of API isReadonlyShellCommand — keep deny-by-default in sync with readonly_shell.go. */
export function isReadonlyShellCommand(command: string): boolean {
  const cmd = command.trim();
  if (!cmd || cmd.length > 2000) return false;
  const lower = cmd.toLowerCase();
  for (const bad of READONLY_SHELL_DENY_SUBSTR) {
    if (lower.includes(bad)) return false;
  }
  if (hasNonNullWriteRedirect(cmd)) return false;
  const segments = splitShellSegments(cmd);
  if (segments.length === 0) return false;
  for (const seg of segments) {
    const s = seg.trim();
    if (!s || s.endsWith("&")) return false;
    const tok = firstShellToken(s);
    if (!tok) return false;
    if (!READONLY_SHELL_ALLOW.has(baseName(tok).toLowerCase())) return false;
  }
  return true;
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

async function needsConfirm(req: HostExecRequest): Promise<boolean> {
  if (req.op === "shell") {
    if (req.dest === "terminal") return true;
    if (hostShellReadonlyAutoEnabled() && isReadonlyShellCommand(req.path || "")) {
      return false;
    }
    return true;
  }
  if (
    req.op === "delete" ||
    req.op === "move" ||
    req.op === "ssh_write" ||
    req.op === "ssh_delete" ||
    req.op === "ssh_exec"
  ) {
    return true;
  }
  if (req.op !== "write") return false;
  const invoke = tauriInvoke();
  if (!invoke) return true;
  try {
    const stat = (await invoke("host_stat", { path: req.path || "" })) as {
      exists?: boolean;
      is_dir?: boolean;
      under_home?: boolean;
    };
    if (stat.under_home === false) return true;
    return Boolean(stat.exists && !stat.is_dir);
  } catch {
    return true;
  }
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
          if (isWriteOp(req.op) && !hostWritesEnabled()) {
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
