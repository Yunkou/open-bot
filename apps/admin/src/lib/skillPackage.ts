/**
 * Pure helpers for the settings「技能」editor: package tree ops + SKILL.md frontmatter checks.
 * Paths are package-relative, "/"-separated, never leading "/". Folders are tracked
 * explicitly (empty ones persist as `<dir>/.keep`, hidden in the tree).
 */

export const SKILL_ENTRY = "SKILL.md";
export const KEEP_FILE = ".keep";

export type PkgFile = {
  /** Current editor buffer. */
  content: string;
  /** Content at last load/save (null = new file, never saved). */
  saved: string | null;
  /** Path at last load/save (null = new). Used for structural dirty detection. */
  savedPath: string | null;
};

export type PkgState = {
  files: Record<string, PkgFile>;
  /** Explicit folders (including empty ones). Parent folders of files are implied. */
  folders: string[];
  /** Snapshot of path layout at last load/save, for structure-dirty. */
  savedLayout: string;
};

export function dirname(p: string): string {
  const i = p.lastIndexOf("/");
  return i < 0 ? "" : p.slice(0, i);
}

export function basename(p: string): string {
  const i = p.lastIndexOf("/");
  return i < 0 ? p : p.slice(i + 1);
}

export function joinPath(dir: string, name: string): string {
  return dir ? `${dir}/${name}` : name;
}

function layoutOf(files: Record<string, PkgFile>, folders: string[]): string {
  return JSON.stringify([Object.keys(files).sort(), allFolders(files, folders)]);
}

/** Every folder path (explicit + implied by files), sorted. */
export function allFolders(files: Record<string, PkgFile>, folders: string[]): string[] {
  const set = new Set<string>();
  const add = (d: string) => {
    while (d) {
      set.add(d);
      d = dirname(d);
    }
  };
  folders.forEach(add);
  Object.keys(files).forEach((p) => add(dirname(p)));
  return [...set].sort();
}

export function loadPackage(list: { path: string; content: string }[]): PkgState {
  const files: Record<string, PkgFile> = {};
  const folders: string[] = [];
  for (const f of list) {
    const p = f.path.replace(/^\.\//, "");
    if (basename(p) === KEEP_FILE) {
      const d = dirname(p);
      if (d) folders.push(d);
      continue;
    }
    files[p] = { content: f.content ?? "", saved: f.content ?? "", savedPath: p };
  }
  return { files, folders, savedLayout: layoutOf(files, folders) };
}

/** Files to PUT: every file + `.keep` for folders that would otherwise be empty. */
export function serializePackage(st: PkgState): { path: string; content: string }[] {
  const out = Object.entries(st.files)
    .map(([path, f]) => ({ path, content: f.content }))
    .sort((a, b) => a.path.localeCompare(b.path));
  const dirsWithFiles = new Set<string>();
  for (const p of Object.keys(st.files)) {
    let d = dirname(p);
    while (d) {
      dirsWithFiles.add(d);
      d = dirname(d);
    }
  }
  for (const d of allFolders(st.files, st.folders)) {
    if (!dirsWithFiles.has(d)) out.push({ path: joinPath(d, KEEP_FILE), content: "" });
  }
  return out;
}

/** After a successful save: buffers become the new baseline. */
export function markSaved(st: PkgState): PkgState {
  const files: Record<string, PkgFile> = {};
  for (const [p, f] of Object.entries(st.files)) files[p] = { content: f.content, saved: f.content, savedPath: p };
  return { files, folders: st.folders, savedLayout: layoutOf(files, st.folders) };
}

export function isFileDirty(f: PkgFile | undefined): boolean {
  return Boolean(f) && (f!.saved === null || f!.content !== f!.saved);
}

export function isStructureDirty(st: PkgState): boolean {
  return layoutOf(st.files, st.folders) !== st.savedLayout;
}

export function isPackageDirty(st: PkgState): boolean {
  return isStructureDirty(st) || Object.values(st.files).some(isFileDirty);
}

/** Validate one path segment typed in the tree (server: [A-Za-z0-9._-], no dotfiles). */
export function segmentError(name: string): string | null {
  const n = name.trim();
  if (!n) return "名称不能为空";
  if (n.length > 80) return "名称过长";
  if (n === "." || n === "..") return "非法名称";
  if (n.startsWith(".")) return "不支持以 . 开头的隐藏文件";
  if (!/^[A-Za-z0-9._-]+$/.test(n)) return "仅允许字母、数字、- _ .（不含空格、中文、/）";
  return null;
}

function exists(st: PkgState, path: string): boolean {
  return path in st.files || allFolders(st.files, st.folders).includes(path);
}

export type OpResult = { state: PkgState; error?: undefined; renamed?: Record<string, string> } | { error: string; state?: undefined };

export function createFile(st: PkgState, dir: string, name: string, content = ""): OpResult {
  const err = segmentError(name);
  if (err) return { error: err };
  const p = joinPath(dir, name.trim());
  if (exists(st, p)) return { error: `「${p}」已存在` };
  return { state: { ...st, files: { ...st.files, [p]: { content, saved: null, savedPath: null } } } };
}

export function createFolder(st: PkgState, dir: string, name: string): OpResult {
  const err = segmentError(name);
  if (err) return { error: err };
  const p = joinPath(dir, name.trim());
  if (exists(st, p)) return { error: `「${p}」已存在` };
  return { state: { ...st, folders: [...st.folders, p] } };
}

function isUnder(path: string, dir: string): boolean {
  return path === dir || path.startsWith(dir + "/");
}

/** Move/rename `from` (file or folder) to `to`. Returns old→new map for open tabs. */
export function movePath(st: PkgState, from: string, to: string): OpResult {
  if (from === to) return { state: st };
  if (from === SKILL_ENTRY) return { error: "入口文件 SKILL.md 不可移动或改名" };
  if (to === SKILL_ENTRY) return { error: "SKILL.md 为保留的入口文件名" };
  if (isUnder(to, from)) return { error: "不能移动到自身或子目录内" };
  if (exists(st, to)) return { error: `「${to}」已存在` };
  const renamed: Record<string, string> = {};
  const files: Record<string, PkgFile> = {};
  for (const [p, f] of Object.entries(st.files)) {
    if (isUnder(p, from)) {
      const np = to + p.slice(from.length);
      renamed[p] = np;
      files[np] = f;
    } else {
      files[p] = f;
    }
  }
  const isFolder = !(from in st.files);
  const folders = st.folders.map((d) => (isUnder(d, from) ? to + d.slice(from.length) : d));
  if (isFolder && !folders.includes(to)) folders.push(to);
  return { state: { ...st, files, folders }, renamed };
}

export function renamePath(st: PkgState, from: string, newName: string): OpResult {
  if (from === SKILL_ENTRY) return { error: "入口文件 SKILL.md 不可改名" };
  const err = segmentError(newName);
  if (err) return { error: err };
  return movePath(st, from, joinPath(dirname(from), newName.trim()));
}

/** Drag-drop: move `from` into folder `targetDir` ("" = package root). */
export function moveInto(st: PkgState, from: string, targetDir: string): OpResult {
  if (dirname(from) === targetDir) return { state: st };
  return movePath(st, from, joinPath(targetDir, basename(from)));
}

export function deletePath(st: PkgState, path: string): OpResult {
  if (path === SKILL_ENTRY) return { error: "入口文件不可删" };
  const files: Record<string, PkgFile> = {};
  for (const [p, f] of Object.entries(st.files)) if (!isUnder(p, path)) files[p] = f;
  const folders = st.folders.filter((d) => !isUnder(d, path));
  return { state: { ...st, files, folders } };
}

export type TreeNode =
  | { kind: "folder"; path: string; name: string; children: TreeNode[] }
  | { kind: "file"; path: string; name: string };

/** Build tree: folders first (a→z), then files; SKILL.md pinned first at root. */
export function buildTree(st: PkgState): TreeNode[] {
  const folders = allFolders(st.files, st.folders);
  const build = (dir: string): TreeNode[] => {
    const subdirs = folders.filter((d) => dirname(d) === dir).sort((a, b) => a.localeCompare(b));
    const files = Object.keys(st.files)
      .filter((p) => dirname(p) === dir)
      .sort((a, b) => a.localeCompare(b));
    const nodes: TreeNode[] = [];
    if (dir === "" && files.includes(SKILL_ENTRY)) nodes.push({ kind: "file", path: SKILL_ENTRY, name: SKILL_ENTRY });
    for (const d of subdirs) nodes.push({ kind: "folder", path: d, name: basename(d), children: build(d) });
    for (const f of files) if (!(dir === "" && f === SKILL_ENTRY)) nodes.push({ kind: "file", path: f, name: basename(f) });
    return nodes;
  };
  return build("");
}

// ---- SKILL.md frontmatter ----

export type FrontmatterIssue = { line: number; message: string };
export type FrontmatterInfo = { name: string; description: string; issues: FrontmatterIssue[]; bodyStartLine: number };

const BLOCK_SCALARS = new Set([">", ">-", ">+", "|", "|-", "|+"]);
const SKILL_NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;

/**
 * Mirrors services/api parseSkillFrontmatter: `---` … `---`, single-line `key: value`.
 * `expectedName` (custom skills): name must equal the skill id and match [a-z0-9-].
 * Lines are 1-based.
 */
export function checkSkillFrontmatter(text: string, expectedName?: string): FrontmatterInfo {
  const lines = text.replace(/\r\n/g, "\n").split("\n");
  const issues: FrontmatterIssue[] = [];
  let i = 0;
  while (i < lines.length && lines[i].trim() === "") i++;
  if (i >= lines.length || lines[i].trim() !== "---") {
    return {
      name: "",
      description: "",
      bodyStartLine: 1,
      issues: [{ line: Math.min(i, lines.length - 1) + 1, message: "SKILL.md 必须以 --- 开头的 frontmatter（name / description）" }],
    };
  }
  const open = i;
  let close = -1;
  for (let j = open + 1; j < lines.length; j++) {
    if (lines[j].trim() === "---") {
      close = j;
      break;
    }
  }
  if (close < 0) {
    return { name: "", description: "", bodyStartLine: 1, issues: [{ line: open + 1, message: "frontmatter 缺少结束的 ---" }] };
  }
  let name = "";
  let description = "";
  let nameLine = -1;
  let descLine = -1;
  for (let j = open + 1; j < close; j++) {
    const raw = lines[j];
    const t = raw.trim();
    if (!t || t.startsWith("#")) continue;
    const k = t.indexOf(":");
    if (k < 0) {
      if (/^\s/.test(raw) && descLine >= 0 && j === descLine + 1) {
        issues.push({ line: j + 1, message: "description 必须写在一行内（不支持多行 / 续行）" });
      }
      continue;
    }
    const key = t.slice(0, k).trim();
    const val = t.slice(k + 1).trim().replace(/^["']|["']$/g, "");
    if (key === "name") {
      name = val;
      nameLine = j;
      if (BLOCK_SCALARS.has(val)) issues.push({ line: j + 1, message: "name 必须是单行值" });
    } else if (key === "description") {
      description = val;
      descLine = j;
      if (BLOCK_SCALARS.has(val)) issues.push({ line: j + 1, message: "description 必须单行（不支持 YAML >- / | 多行写法）" });
    } else if (/^\s/.test(raw) && descLine >= 0 && j === descLine + 1) {
      issues.push({ line: j + 1, message: "description 必须写在一行内（不支持多行 / 续行）" });
    }
  }
  if (!name) issues.push({ line: (nameLine >= 0 ? nameLine : open) + 1, message: "name 必填" });
  else if (expectedName !== undefined) {
    if (!SKILL_NAME_RE.test(name)) issues.push({ line: nameLine + 1, message: "name 仅允许小写字母、数字与 -（如 my-helper）" });
    else if (name !== expectedName)
      issues.push({ line: nameLine + 1, message: `name 必须与技能名「${expectedName}」一致（改名请新建技能）` });
  }
  if (!description) issues.push({ line: (descLine >= 0 ? descLine : open) + 1, message: "description 必填（单行）" });
  return { name, description, issues, bodyStartLine: close + 2 };
}

/** Body without frontmatter, for preview. */
export function stripFrontmatter(text: string): string {
  const m = /^\s*---\r?\n[\s\S]*?\r?\n---[ \t]*(\r?\n|$)/.exec(text);
  return m ? text.slice(m[0].length) : text;
}

/** Rewrite the frontmatter `name:` line (copy built-in → custom). */
export function replaceFrontmatterName(text: string, newName: string): string {
  const m = /^(\s*---\r?\n)([\s\S]*?)(\r?\n---)/.exec(text);
  if (!m) return `---\nname: ${newName}\ndescription: 自定义技能：${newName}\n---\n\n${text}`;
  const fm = /^name\s*:.*$/m.test(m[2]) ? m[2].replace(/^name\s*:.*$/m, `name: ${newName}`) : `name: ${newName}\n${m[2]}`;
  return text.slice(0, m.index) + m[1] + fm + m[3] + text.slice(m.index + m[0].length);
}

export function fileKind(path: string): "md" | "py" | "sh" | "code" | "other" {
  const lower = path.toLowerCase();
  if (lower.endsWith(".md") || lower.endsWith(".markdown")) return "md";
  if (lower.endsWith(".py")) return "py";
  if (lower.endsWith(".sh") || lower.endsWith(".bash") || lower.endsWith(".zsh")) return "sh";
  if (/\.(js|mjs|cjs|ts|tsx|jsx|json|ya?ml|toml|html|css|go|rb|rs|java|sql|txt)$/.test(lower)) return "code";
  return "other";
}
