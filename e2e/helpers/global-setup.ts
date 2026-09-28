import { spawn, type ChildProcess } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { apiHealth } from "./api";
import { mockLLMURL } from "./env";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const pidFile = path.join(__dirname, "..", ".mock-llm.pid");

async function waitHealthy(url: string, attempts = 40): Promise<void> {
  const health = url.replace(/\/v1\/?$/, "") + "/healthz";
  for (let i = 0; i < attempts; i++) {
    try {
      const res = await fetch(health);
      if (res.ok) return;
    } catch {
      /* retry */
    }
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`mock LLM not healthy at ${health}`);
}

export default async function globalSetup() {
  await apiHealth();

  const base = mockLLMURL();
  try {
    const health = base.replace(/\/v1\/?$/, "") + "/healthz";
    const res = await fetch(health);
    if (res.ok) {
      console.log("[e2e] mock LLM already running");
      return;
    }
  } catch {
    /* start */
  }

  const script = path.join(__dirname, "..", "mock-llm-server.mjs");
  const child: ChildProcess = spawn(process.execPath, [script], {
    cwd: path.join(__dirname, ".."),
    env: { ...process.env },
    stdio: ["ignore", "pipe", "pipe"],
    detached: true,
  });
  if (child.pid) {
    fs.writeFileSync(pidFile, String(child.pid), "utf8");
  }
  child.unref();
  await waitHealthy(base);
  console.log(`[e2e] started mock LLM pid=${child.pid} url=${base}`);
}
