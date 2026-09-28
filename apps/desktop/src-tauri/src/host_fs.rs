use std::fs;
use std::path::{Component, Path, PathBuf};
use std::time::UNIX_EPOCH;

use serde_json::{json, Value};

fn home_dir() -> Result<PathBuf, String> {
    for key in ["HOME", "USERPROFILE"] {
        if let Ok(v) = std::env::var(key) {
            let p = PathBuf::from(v);
            if p.is_dir() {
                return Ok(p);
            }
        }
    }
    Err("找不到用户主目录".into())
}

fn allowed_roots() -> Result<Vec<PathBuf>, String> {
    let home = home_dir()?;
    Ok(vec![
        home.join("Downloads"),
        home.join("Desktop"),
        home.join("Documents"),
    ])
}

fn logical_path(raw: &str) -> Result<PathBuf, String> {
    let raw = raw.trim();
    if raw.is_empty() || raw == "~" || raw == "home" {
        return Err("请指定 Downloads、Desktop 或 Documents 下的路径".into());
    }
    if raw.contains('\0') {
        return Err("路径无效".into());
    }
    let home = home_dir()?;
    let path = if let Some(rest) = raw.strip_prefix("~/") {
        home.join(rest)
    } else if raw == "~" {
        return Err("请指定 Downloads、Desktop 或 Documents 下的路径".into());
    } else {
        let p = PathBuf::from(raw);
        if p.is_absolute() {
            p
        } else {
            home.join(raw)
        }
    };
    for c in path.components() {
        if matches!(c, Component::ParentDir) {
            return Err("路径不能包含 ..".into());
        }
    }
    Ok(path)
}

fn under_root(path: &Path, root: &Path) -> bool {
    path == root || path.starts_with(root)
}

fn ensure_allowed(raw: &str) -> Result<PathBuf, String> {
    let path = logical_path(raw)?;
    let roots = allowed_roots()?;
    let ok = roots.iter().any(|root| under_root(&path, root));
    if !ok {
        return Err("只能访问 Downloads、Desktop、Documents".into());
    }
    if path.exists() {
        let canon = fs::canonicalize(&path).map_err(|e| e.to_string())?;
        let rooted = roots.iter().any(|root| {
            fs::canonicalize(root)
                .map(|c| under_root(&canon, &c))
                .unwrap_or(false)
        });
        if !rooted {
            return Err("只能访问 Downloads、Desktop、Documents".into());
        }
        return Ok(canon);
    }
    if let Some(parent) = path.parent() {
        if parent.exists() {
            let canon_parent = fs::canonicalize(parent).map_err(|e| e.to_string())?;
            let rooted = roots.iter().any(|root| {
                fs::canonicalize(root)
                    .map(|c| under_root(&canon_parent, &c))
                    .unwrap_or(false)
            });
            if !rooted {
                return Err("只能访问 Downloads、Desktop、Documents".into());
            }
        }
    }
    Ok(path)
}

fn list_roots() -> Result<Value, String> {
    let roots = allowed_roots()?;
    let mut entries = Vec::new();
    for root in roots {
        let name = root
            .file_name()
            .map(|s| s.to_string_lossy().to_string())
            .unwrap_or_else(|| root.display().to_string());
        entries.push(json!({
            "name": name,
            "path": name,
            "is_dir": true,
        }));
    }
    Ok(json!({ "ok": true, "path": "", "entries": entries }))
}

#[tauri::command]
pub fn host_stat(path: String) -> Result<Value, String> {
    if path.trim().is_empty() || path.trim() == "~" {
        return Ok(json!({ "ok": true, "exists": true, "is_dir": true }));
    }
    let resolved = ensure_allowed(&path)?;
    let exists = resolved.exists();
    Ok(json!({
        "ok": true,
        "exists": exists,
        "is_dir": exists && resolved.is_dir(),
    }))
}

#[tauri::command]
pub fn host_ls(path: String) -> Result<Value, String> {
    if path.trim().is_empty() || path.trim() == "~" || path.trim() == "home" {
        return list_roots();
    }
    let dir = ensure_allowed(&path)?;
    if !dir.is_dir() {
        return Err("不是目录".into());
    }
    let mut entries = Vec::new();
    for item in fs::read_dir(&dir).map_err(|e| e.to_string())? {
        let item = item.map_err(|e| e.to_string())?;
        let meta = item.metadata().map_err(|e| e.to_string())?;
        let modified = meta
            .modified()
            .ok()
            .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
            .map(|d| d.as_secs())
            .unwrap_or(0);
        entries.push(json!({
            "name": item.file_name().to_string_lossy(),
            "is_dir": meta.is_dir(),
            "size": meta.len(),
            "modified": modified,
        }));
    }
    entries.sort_by(|a, b| {
        let mb = b.get("modified").and_then(|v| v.as_u64()).unwrap_or(0);
        let ma = a.get("modified").and_then(|v| v.as_u64()).unwrap_or(0);
        mb.cmp(&ma)
    });
    if entries.len() > 200 {
        entries.truncate(200);
    }
    Ok(json!({ "ok": true, "path": path, "entries": entries }))
}

#[tauri::command]
pub fn host_read(path: String) -> Result<Value, String> {
    let file = ensure_allowed(&path)?;
    if !file.is_file() {
        return Err("不是文件".into());
    }
    let bytes = fs::read(&file).map_err(|e| e.to_string())?;
    if bytes.len() > 200_000 {
        return Err("文件太大，只支持读取 200KB 以内的文本".into());
    }
    let text = String::from_utf8(bytes).map_err(|_| "不是文本文件".to_string())?;
    Ok(json!({ "ok": true, "path": path, "content": text }))
}

#[tauri::command]
pub fn host_write(path: String, content: String) -> Result<Value, String> {
    let file = ensure_allowed(&path)?;
    if let Some(parent) = file.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }
    fs::write(&file, content.as_bytes()).map_err(|e| e.to_string())?;
    Ok(json!({ "ok": true, "path": path }))
}

#[tauri::command]
pub fn host_delete(path: String) -> Result<Value, String> {
    let file = ensure_allowed(&path)?;
    if file.is_dir() {
        return Err("只能删除文件".into());
    }
    fs::remove_file(&file).map_err(|e| e.to_string())?;
    Ok(json!({ "ok": true, "path": path }))
}

#[tauri::command]
pub fn host_move(path: String, dest: String) -> Result<Value, String> {
    let from = ensure_allowed(&path)?;
    let to = ensure_allowed(&dest)?;
    if let Some(parent) = to.parent() {
        fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }
    fs::rename(&from, &to).map_err(|e| e.to_string())?;
    Ok(json!({ "ok": true, "path": path, "dest": dest }))
}
