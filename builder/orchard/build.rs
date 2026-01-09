use std::env;
use std::fs;
use std::path::PathBuf;
use std::process::Command;

fn main() {
    #[cfg(target_os = "macos")]
    {
        let out_dir = match env::var("OUT_DIR") {
            Ok(value) => PathBuf::from(value),
            Err(_) => return,
        };

        let target_dir = match out_dir.ancestors().nth(3) {
            Some(path) => path.to_path_buf(),
            None => return,
        };

        let deps_dir = target_dir.join("deps");
        let _ = fs::create_dir_all(&deps_dir);

        let rustc = env::var("RUSTC").unwrap_or_else(|_| "rustc".to_string());
        let sysroot = Command::new(rustc)
            .args(["--print", "sysroot"])
            .output()
            .ok()
            .and_then(|output| String::from_utf8(output.stdout).ok())
            .map(|s| s.trim().to_string());

        let target = env::var("TARGET").unwrap_or_default();
        let sysroot = match sysroot {
            Some(path) if !path.is_empty() => PathBuf::from(path),
            _ => return,
        };

        let lib_dir = sysroot
            .join("lib")
            .join("rustlib")
            .join(target)
            .join("lib");

        if let Ok(entries) = fs::read_dir(&lib_dir) {
            for entry in entries.flatten() {
                let path = entry.path();
                let name = match path.file_name().and_then(|s| s.to_str()) {
                    Some(name) => name,
                    None => continue,
                };
                if name.starts_with("libstd-") && name.ends_with(".dylib") {
                    let dest = deps_dir.join(name);
                    let _ = fs::copy(&path, &dest);
                    break;
                }
            }
        }
    }
}
