import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const pidFile = path.join(__dirname, "..", ".mock-llm.pid");

export default async function globalTeardown() {
  if (!fs.existsSync(pidFile)) return;
  const pid = Number(fs.readFileSync(pidFile, "utf8").trim());
  fs.unlinkSync(pidFile);
  if (!pid || Number.isNaN(pid)) return;
  try {
    process.kill(pid, "SIGTERM");
    console.log(`[e2e] stopped mock LLM pid=${pid}`);
  } catch {
    /* already gone */
  }
}
