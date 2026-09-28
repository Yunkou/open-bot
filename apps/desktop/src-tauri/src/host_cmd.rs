use std::fs;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::thread;
use std::time::Duration;

use serde_json::{json, Value};

fn home_dir() -> Option<PathBuf> {
    for key in ["HOME", "USERPROFILE"] {
        if let Ok(v) = std::env::var(key) {
            let p = PathBuf::from(v);
            if p.is_dir() {
                return Some(p);
            }
        }
    }
    None
}

fn validate_label(raw: &str, limit: usize) -> Result<String, String> {
    let text = raw.trim();
    if text.is_empty() {
        return Err("内容是空的".into());
    }
    if text.chars().any(|c| c.is_control()) || text.len() > limit {
        return Err("内容无效或太长".into());
    }
    Ok(text.to_string())
}

fn command_ok(cmd: &mut Command) -> Result<(), String> {
    let status = cmd.status().map_err(|e| e.to_string())?;
    if status.success() {
        Ok(())
    } else {
        Err(format!("启动失败（{status}）"))
    }
}

fn consider_app(path: PathBuf, query: &str, exact: &mut Option<PathBuf>, fuzzy: &mut Option<PathBuf>) {
    let Some(name) = path.file_name().and_then(|s| s.to_str()) else {
        return;
    };
    let Some(stem) = name.strip_suffix(".app") else {
        return;
    };
    if stem.eq_ignore_ascii_case(query) {
        *exact = Some(path);
        return;
    }
    if fuzzy.is_none() && stem.to_lowercase().contains(&query.to_lowercase()) {
        *fuzzy = Some(path);
    }
}

fn scan_apps(dir: &Path, query: &str, depth: u8, exact: &mut Option<PathBuf>, fuzzy: &mut Option<PathBuf>) {
    if exact.is_some() || depth > 1 {
        return;
    }
    let Ok(rd) = fs::read_dir(dir) else {
        return;
    };
    for ent in rd.flatten() {
        if exact.is_some() {
            return;
        }
        let path = ent.path();
        let name = ent.file_name().to_string_lossy().to_string();
        if name.ends_with(".app") {
            consider_app(path, query, exact, fuzzy);
        } else if path.is_dir() && !name.starts_with('.') {
            scan_apps(&path, query, depth + 1, exact, fuzzy);
        }
    }
}

fn find_macos_app(query: &str) -> Option<PathBuf> {
    let query = query.trim().trim_end_matches(".app");
    let mut dirs = vec![
        PathBuf::from("/Applications"),
        PathBuf::from("/System/Applications"),
    ];
    if let Some(home) = home_dir() {
        dirs.insert(0, home.join("Applications"));
    }
    let mut exact = None;
    let mut fuzzy = None;
    for dir in dirs {
        scan_apps(&dir, query, 0, &mut exact, &mut fuzzy);
        if exact.is_some() {
            break;
        }
    }
    exact.or(fuzzy)
}

#[tauri::command]
pub fn host_open(name: String) -> Result<Value, String> {
    let name = validate_label(&name, 200)?;
    #[cfg(target_os = "macos")]
    {
        let mut cmd = Command::new("open");
        cmd.arg("-a");
        if let Some(app) = find_macos_app(&name) {
            cmd.arg(&app);
            command_ok(&mut cmd)?;
            return Ok(json!({ "ok": true, "name": name, "app": app.display().to_string() }));
        }
        cmd.arg(&name);
        command_ok(&mut cmd)?;
        return Ok(json!({ "ok": true, "name": name }));
    }
    #[cfg(target_os = "windows")]
    {
        let mut cmd = Command::new("cmd");
        cmd.args(["/C", "start", "", &name]);
        command_ok(&mut cmd)?;
        return Ok(json!({ "ok": true, "name": name }));
    }
    #[cfg(all(unix, not(target_os = "macos")))]
    {
        if Command::new("gtk-launch")
            .arg(&name)
            .status()
            .map(|s| s.success())
            .unwrap_or(false)
        {
            return Ok(json!({ "ok": true, "name": name }));
        }
        let mut cmd = Command::new("xdg-open");
        cmd.arg(&name);
        command_ok(&mut cmd)?;
        return Ok(json!({ "ok": true, "name": name }));
    }
    #[cfg(not(any(target_os = "macos", target_os = "windows", unix)))]
    {
        let _ = name;
        Err("当前系统不能打开软件".into())
    }
}

fn applescript_quote(s: &str) -> String {
    format!("\"{}\"", s.replace('\\', "\\\\").replace('"', "\\\""))
}

fn open_terminal(command: &str) -> Result<(), String> {
    #[cfg(target_os = "macos")]
    {
        let script = format!(
            "tell application \"Terminal\" to activate\ntell application \"Terminal\" to do script {}",
            applescript_quote(command)
        );
        let mut cmd = Command::new("osascript");
        cmd.args(["-e", &script]);
        return command_ok(&mut cmd);
    }
    #[cfg(target_os = "windows")]
    {
        let mut cmd = Command::new("cmd");
        cmd.args(["/C", "start", "cmd", "/K", command]);
        return command_ok(&mut cmd);
    }
    #[cfg(all(unix, not(target_os = "macos")))]
    {
        if Command::new("x-terminal-emulator")
            .args(["-e", "bash", "-lc", command])
            .status()
            .map(|s| s.success())
            .unwrap_or(false)
        {
            return Ok(());
        }
        let mut cmd = Command::new("gnome-terminal");
        cmd.args(["--", "bash", "-lc", command]);
        return command_ok(&mut cmd);
    }
    #[cfg(not(any(target_os = "macos", target_os = "windows", unix)))]
    {
        let _ = command;
        Err("当前系统不能打开终端".into())
    }
}

fn run_captured(command: &str) -> Result<Value, String> {
    let mut cmd = if cfg!(windows) {
        let mut c = Command::new("cmd");
        c.args(["/C", command]);
        c
    } else {
        let mut c = Command::new("sh");
        c.args(["-c", command]);
        c
    };
    cmd.stdin(Stdio::null()).stdout(Stdio::piped()).stderr(Stdio::piped());
    if let Some(home) = home_dir() {
        cmd.current_dir(home);
    }
    #[cfg(unix)]
    {
        use std::os::unix::process::CommandExt;
        cmd.process_group(0);
    }
    let child = cmd.spawn().map_err(|e| e.to_string())?;
    let pid = child.id();
    let (tx, rx) = std::sync::mpsc::channel();
    thread::spawn(move || {
        let _ = tx.send(child.wait_with_output());
    });
    let output = match rx.recv_timeout(Duration::from_secs(30)) {
        Ok(Ok(output)) => output,
        Ok(Err(e)) => return Err(e.to_string()),
        Err(_) => {
            #[cfg(unix)]
            {
                let _ = Command::new("kill").args(["-9", &format!("-{pid}")]).status();
            }
            #[cfg(windows)]
            {
                let _ = Command::new("taskkill")
                    .args(["/F", "/T", "/PID", &pid.to_string()])
                    .status();
            }
            return Err("命令超过 30 秒还没结束".into());
        }
    };
    let mut text = String::from_utf8_lossy(&output.stdout).to_string();
    let err = String::from_utf8_lossy(&output.stderr);
    if !err.trim().is_empty() {
        if !text.is_empty() && !text.ends_with('\n') {
            text.push('\n');
        }
        text.push_str(&err);
    }
    let chars: Vec<char> = text.chars().collect();
    if chars.len() > 4000 {
        text = chars[..4000].iter().collect();
        text.push_str("\n…");
    }
    Ok(json!({
        "ok": output.status.success(),
        "exit_code": output.status.code(),
        "output": text,
    }))
}

#[tauri::command]
pub fn host_shell(command: String, terminal: bool) -> Result<Value, String> {
    let command = validate_label(&command, 2000)?;
    if terminal {
        open_terminal(&command)?;
        return Ok(json!({ "ok": true, "terminal": true, "command": command }));
    }
    let mut result = run_captured(&command)?;
    if let Some(obj) = result.as_object_mut() {
        obj.insert("command".into(), json!(command));
        obj.insert("terminal".into(), json!(false));
    }
    Ok(result)
}
