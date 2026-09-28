mod host_cmd;
mod host_fs;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            host_fs::host_stat,
            host_fs::host_ls,
            host_fs::host_read,
            host_fs::host_write,
            host_fs::host_delete,
            host_fs::host_move,
            host_cmd::host_open,
            host_cmd::host_shell,
        ])
        .run(tauri::generate_context!())
        .expect("error while running Open Bot");
}
