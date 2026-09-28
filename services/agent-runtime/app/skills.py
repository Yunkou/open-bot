"""Agent Skills loader (agentskills.io SKILL.md).

Loads global skills/ plus optional per-user skills/users/{user_id}/.
"""

from __future__ import annotations

import os
import re
from dataclasses import dataclass
from pathlib import Path

_FRONTMATTER_RE = re.compile(r"\A---\s*\n(.*?)\n---\s*\n?(.*)\Z", re.DOTALL)


@dataclass
class SkillMeta:
    name: str
    description: str
    path: Path
    custom: bool = False


@dataclass
class SkillFull(SkillMeta):
    body: str = ""
    raw: str = ""


def default_skills_root() -> Path:
    env = (os.getenv("SKILLS_DIR") or "").strip()
    if env:
        return Path(env)
    root = (os.getenv("OPEN_BOT_ROOT") or "").strip()
    if root:
        return Path(root) / "skills"
    # services/agent-runtime/app -> repo root
    return Path(__file__).resolve().parents[3] / "skills"


def user_skills_dir(user_id: str, root: Path | None = None) -> Path:
    root = root or default_skills_root()
    safe = "".join(c if c.isalnum() or c in "-_" else "_" for c in (user_id or "").strip()) or "_unknown"
    return root / "users" / safe


def _parse_frontmatter(text: str) -> tuple[dict[str, str], str]:
    m = _FRONTMATTER_RE.match(text)
    if not m:
        return {}, text
    meta: dict[str, str] = {}
    for line in m.group(1).splitlines():
        line = line.strip()
        if not line or line.startswith("#") or ":" not in line:
            continue
        key, _, val = line.partition(":")
        meta[key.strip()] = val.strip().strip("'\"")
    return meta, m.group(2).strip()


def _load_dir(root: Path, *, custom: bool, into: dict[str, SkillMeta]) -> None:
    if not root.is_dir():
        return
    for skill_dir in sorted(root.iterdir()):
        if not skill_dir.is_dir():
            continue
        if not custom and skill_dir.name == "users":
            continue
        skill_md = skill_dir / "SKILL.md"
        if not skill_md.is_file():
            continue
        try:
            text = skill_md.read_text(encoding="utf-8")
        except OSError:
            continue
        meta, _ = _parse_frontmatter(text)
        name = (meta.get("name") or skill_dir.name).strip()
        desc = (meta.get("description") or "").strip()
        if not name or not desc:
            continue
        into[name] = SkillMeta(name=name, description=desc, path=skill_md, custom=custom)


class SkillRegistry:
    def __init__(self, root: Path | None = None, user_id: str | None = None) -> None:
        self.root = root or default_skills_root()
        self.user_id = (user_id or "").strip() or None
        self._by_name: dict[str, SkillMeta] = {}
        self.reload()

    def reload(self) -> None:
        self._by_name.clear()
        _load_dir(self.root, custom=False, into=self._by_name)
        if self.user_id:
            _load_dir(user_skills_dir(self.user_id, self.root), custom=True, into=self._by_name)

    def list_meta(self) -> list[SkillMeta]:
        return list(self._by_name.values())

    def catalog_for_prompt(self, enabled: set[str] | list[str] | None = None) -> str:
        items = self.list_meta()
        if enabled is not None:
            allow = set(enabled)
            items = [s for s in items if s.name in allow]
        if not items:
            return "（当前未发现可用技能）"
        lines = [f"- {s.name}: {s.description}" for s in items]
        return "\n".join(lines)

    def filter_meta(self, enabled: set[str] | list[str] | None) -> list[SkillMeta]:
        items = self.list_meta()
        if enabled is None:
            return items
        allow = set(enabled)
        return [s for s in items if s.name in allow]

    def load(self, name: str) -> SkillFull | None:
        meta = self._by_name.get(name)
        if not meta:
            for m in self._by_name.values():
                if m.path.parent.name == name:
                    meta = m
                    break
        if not meta:
            return None
        try:
            raw = meta.path.read_text(encoding="utf-8")
        except OSError:
            return None
        _, body = _parse_frontmatter(raw)
        return SkillFull(
            name=meta.name,
            description=meta.description,
            path=meta.path,
            custom=meta.custom,
            body=body,
            raw=raw,
        )


def registry_for_user(user_id: str | None = None, root: Path | None = None) -> SkillRegistry:
    return SkillRegistry(root=root, user_id=user_id)
