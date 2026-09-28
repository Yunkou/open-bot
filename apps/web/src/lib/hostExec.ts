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

function isWriteOp(op: string): boolean {
  return op === "write" || op === "delete" || op === "move";
}

async function runOp(req: HostExecRequest): Promise<Record<string, unknown>> {
  const invoke = tauriInvoke();
  if (!invoke) {
    return { ok: false, error: "当前窗口不能操作本机文件" };
  }
  const path = req.path || "";
  if (req.op === "ls") return (await invoke("host_ls", { path })) as Record<string, unknown>;
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
  return { ok: false, error: "不支持的操作" };
}

async function needsConfirm(req: HostExecRequest): Promise<boolean> {
  if (req.op === "shell" || req.op === "delete" || req.op === "move") return true;
  if (req.op !== "write") return false;
  const invoke = tauriInvoke();
  if (!invoke) return true;
  try {
    const stat = (await invoke("host_stat", { path: req.path || "" })) as { exists?: boolean; is_dir?: boolean };
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
            result = { ok: false, error: "本机写入已关闭" };
          } else if (await needsConfirm(req)) {
            const allowed = await opts.confirm(req);
            if (!allowed) {
              result = { ok: false, denied: true, error: "用户拒绝了这次操作" };
            } else {
              result = await runOp(req);
              result.confirmed = true;
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
