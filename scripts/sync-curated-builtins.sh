#!/usr/bin/env bash
# Sync skills/curated-builtins.json → skills/<slug>/ + skills/CURATED.lock
# Fetches from GitHub (codeload). Falls back to scaffolding empty dirs + lock notes
# when network blocks. Does not overwrite preserve_local packages (tdd, diagnosing-bugs).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST="$ROOT/skills/curated-builtins.json"
SKILLS_DIR="$ROOT/skills"
LOCK="$SKILLS_DIR/CURATED.lock"
TMP="${TMPDIR:-/tmp}/open-bot-curated-sync-$$"

if [[ ! -f "$MANIFEST" ]]; then
  echo "missing $MANIFEST" >&2
  exit 1
fi

command -v python3 >/dev/null || { echo "python3 required" >&2; exit 1; }

mkdir -p "$TMP"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

python3 - "$MANIFEST" "$SKILLS_DIR" "$LOCK" "$TMP" <<'PY'
import hashlib, json, os, shutil, sys, tarfile, urllib.error, urllib.request
from datetime import datetime, timezone

manifest_path, skills_dir, lock_path, tmp = sys.argv[1:5]
with open(manifest_path, encoding="utf-8") as f:
    manifest = json.load(f)

lock = {
    "synced_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "manifest_version": manifest.get("version"),
    "skills": {},
    "errors": [],
    "refresh": (
        "Re-run: scripts/sync-curated-builtins.sh\n"
        "Or per skill: npx skills add <repo> --skill <skill>  then copy into skills/<slug>/\n"
        "Update refs in skills/curated-builtins.json after upstream moves."
    ),
}

def download_tarball(repo_url: str, ref: str, dest_dir: str) -> str:
    """Return extracted root dir path."""
    owner_repo = repo_url.rstrip("/").replace("https://github.com/", "")
    # Prefer codeload full sha or branch
    urls = [
        f"https://codeload.github.com/{owner_repo}/tar.gz/{ref}",
        f"https://codeload.github.com/{owner_repo}/tar.gz/main",
    ]
    tgz = os.path.join(dest_dir, "src.tgz")
    last_err = None
    for url in urls:
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "open-bot-curated-sync"})
            with urllib.request.urlopen(req, timeout=120) as r, open(tgz, "wb") as out:
                shutil.copyfileobj(r, out)
            last_err = None
            break
        except Exception as e:
            last_err = e
    if last_err:
        raise last_err
    extract_to = os.path.join(dest_dir, "extract")
    os.makedirs(extract_to, exist_ok=True)
    with tarfile.open(tgz, "r:gz") as tar:
        tar.extractall(extract_to)
    roots = [os.path.join(extract_to, n) for n in os.listdir(extract_to)]
    roots = [p for p in roots if os.path.isdir(p)]
    if not roots:
        raise RuntimeError("empty tarball")
    return roots[0]

def copy_tree(src: str, dst: str) -> None:
    if os.path.exists(dst):
        shutil.rmtree(dst)
    shutil.copytree(src, dst)

def sha256_dir(path: str) -> str:
    h = hashlib.sha256()
    for dirpath, _, filenames in os.walk(path):
        for name in sorted(filenames):
            fp = os.path.join(dirpath, name)
            rel = os.path.relpath(fp, path).replace(os.sep, "/")
            h.update(rel.encode())
            with open(fp, "rb") as f:
                while True:
                    chunk = f.read(65536)
                    if not chunk:
                        break
                    h.update(chunk)
    return h.hexdigest()[:16]

# Cache one extract per repo@ref
extracts = {}

for sk in manifest.get("skills", []):
    slug = sk["slug"]
    dest = os.path.join(skills_dir, slug)
    entry = {
        "slug": slug,
        "package": sk.get("package"),
        "skill": sk.get("skill"),
        "repo": sk.get("repo"),
        "path": sk.get("path"),
        "ref": sk.get("ref"),
        "preserve_local": bool(sk.get("preserve_local")),
        "status": "pending",
    }
    if sk.get("preserve_local"):
        if os.path.isfile(os.path.join(dest, "SKILL.md")):
            entry["status"] = "preserved_local"
            entry["content_hash"] = sha256_dir(dest)
            lock["skills"][slug] = entry
            print(f"preserve {slug}")
            continue
        else:
            print(f"warn: preserve_local {slug} missing SKILL.md; will fetch")

    key = (sk["repo"], sk.get("ref") or "main")
    try:
        if key not in extracts:
            repo_tmp = os.path.join(tmp, hashlib.sha1(f"{key[0]}@{key[1]}".encode()).hexdigest()[:12])
            os.makedirs(repo_tmp, exist_ok=True)
            extracts[key] = download_tarball(sk["repo"], sk.get("ref") or "main", repo_tmp)
            print(f"fetched {sk['repo']}@{sk.get('ref', 'main')[:12]}")
        root = extracts[key]
        src = os.path.join(root, sk["path"])
        if not os.path.isdir(src):
            raise FileNotFoundError(f"path not in tarball: {sk['path']}")
        if not os.path.isfile(os.path.join(src, "SKILL.md")):
            raise FileNotFoundError(f"no SKILL.md under {sk['path']}")
        copy_tree(src, dest)
        entry["status"] = "synced"
        entry["content_hash"] = sha256_dir(dest)
        print(f"synced {slug} ← {sk['path']}")
    except Exception as e:
        entry["status"] = "error"
        entry["error"] = str(e)
        lock["errors"].append(f"{slug}: {e}")
        print(f"ERROR {slug}: {e}", file=sys.stderr)
        # Scaffold placeholder if missing so repo layout is documented
        if not os.path.isfile(os.path.join(dest, "SKILL.md")):
            os.makedirs(dest, exist_ok=True)
            stub = (
                f"---\nname: {slug}\ndescription: "
                f"Placeholder — run scripts/sync-curated-builtins.sh to fetch from "
                f"{sk.get('package')} ({sk.get('install', '')}).\n---\n\n"
                f"# {slug}\n\nNetwork fetch failed during sync. Refresh with:\n\n"
                f"```bash\n{sk.get('install', 'scripts/sync-curated-builtins.sh')}\n```\n"
            )
            with open(os.path.join(dest, "SKILL.md"), "w", encoding="utf-8") as f:
                f.write(stub)
            entry["status"] = "scaffolded"
            print(f"scaffolded {slug}")

    lock["skills"][slug] = entry

with open(lock_path, "w", encoding="utf-8") as f:
    json.dump(lock, f, indent=2, ensure_ascii=False)
    f.write("\n")

errs = lock["errors"]
synced = sum(1 for s in lock["skills"].values() if s["status"] in ("synced", "preserved_local"))
print(f"done: {synced}/{len(lock['skills'])} ok; lock → {lock_path}")
if errs and synced == 0:
    sys.exit(2)
PY
