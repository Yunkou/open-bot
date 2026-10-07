#!/usr/bin/env node
// Evaluate a JS expression in a live Chromium / Electron page over CDP.
// Adapted for open-bot from Hermes Agent's desktop CDP helper idea (MIT).
//
// Usage:
//   node cdp_eval.mjs [--port 9222] [--match <substring of url/title>] "<js expression>"
//   node cdp_eval.mjs --list            # list page targets
// Requires Node >= 22 (global WebSocket). Prints the evaluated value (JSON if object).
const args = process.argv.slice(2);
let port = 9222, match = '', list = false;
const rest = [];
for (let i = 0; i < args.length; i++) {
  if (args[i] === '--port') port = Number(args[++i]);
  else if (args[i] === '--match') match = args[++i];
  else if (args[i] === '--list') list = true;
  else rest.push(args[i]);
}
const expr = rest.join(' ');
if (typeof WebSocket === 'undefined') {
  console.error('Node >= 22 required (global WebSocket missing).');
  process.exit(2);
}
let targets;
try {
  targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
} catch (e) {
  console.error(`No CDP endpoint on 127.0.0.1:${port} (${e.message}).`);
  process.exit(3);
}
const pages = targets.filter((t) => t.type === 'page');
if (list) {
  for (const t of pages) console.log(`${t.id}  ${t.title}  ${t.url}`);
  process.exit(0);
}
const target = pages.find((t) => !match || t.url.includes(match) || t.title.includes(match));
if (!target) {
  console.error(`No page target matching "${match}". Use --list.`);
  process.exit(4);
}
if (!expr) {
  console.error('Missing expression.');
  process.exit(1);
}
const ws = new WebSocket(target.webSocketDebuggerUrl);
const timer = setTimeout(() => { console.error('timeout'); process.exit(5); }, 15000);
ws.onopen = () => ws.send(JSON.stringify({
  id: 1,
  method: 'Runtime.evaluate',
  params: { expression: expr, returnByValue: true, awaitPromise: true },
}));
ws.onmessage = (ev) => {
  const msg = JSON.parse(typeof ev.data === 'string' ? ev.data : ev.data.toString());
  if (msg.id !== 1) return;
  clearTimeout(timer);
  const r = msg.result || {};
  if (r.exceptionDetails) {
    console.error('Exception:', r.exceptionDetails.exception?.description || r.exceptionDetails.text);
    process.exit(6);
  }
  const v = r.result?.value;
  console.log(typeof v === 'string' ? v : JSON.stringify(v, null, 1));
  ws.close();
  process.exit(0);
};
ws.onerror = (e) => { console.error('ws error', e.message || e); process.exit(7); };
