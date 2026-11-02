use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

pub fn player(configured: Option<&str>) -> PathBuf {
    if let Some(path) = configured.filter(|path| !path.is_empty()) {
        return path.into();
    }
    #[cfg(windows)]
    let name = "neoviolet.exe";
    #[cfg(not(windows))]
    let name = "neoviolet";
    if let Ok(executable) = std::env::current_exe()
        && let Some(directory) = executable.parent()
    {
        let sibling = directory.join(name);
        if sibling.is_file() {
            return sibling;
        }
    }
    name.into()
}

pub fn arguments(input: impl IntoIterator<Item = String>) -> Vec<String> {
    let mut output = vec!["--xdg-config".into()];
    let mut options = true;
    for argument in input {
        if options && argument == "--" {
            options = false;
        } else if options && (argument == "--xdg-config" || argument.starts_with("--xdg-config=")) {
            continue;
        }
        output.push(argument);
    }
    output
}

pub fn version(path: &Path) -> String {
    let mut command = Command::new(path);
    command
        .arg("--version")
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        command.creation_flags(0x08000000);
    }
    let Ok(mut child) = command.spawn() else {
        return "unavailable".into();
    };
    let deadline = Instant::now() + Duration::from_secs(3);
    loop {
        match child.try_wait() {
            Ok(Some(_)) => {
                return child
                    .wait_with_output()
                    .ok()
                    .map(|output| String::from_utf8_lossy(&output.stdout).trim().to_string())
                    .filter(|text| !text.is_empty())
                    .unwrap_or_else(|| "unavailable".into());
            }
            Ok(None) if Instant::now() < deadline => std::thread::sleep(Duration::from_millis(20)),
            _ => {
                let _ = child.kill();
                let _ = child.wait();
                return "unavailable".into();
            }
        }
    }
}

pub fn environment() -> std::collections::HashMap<String, String> {
    #[cfg(target_os = "macos")]
    {
        let shell = std::env::var("SHELL").unwrap_or_else(|_| "/bin/zsh".into());
        if let Some(output) = capture(&shell, &["-l", "-c", "/usr/bin/env -0"]) {
            return output
                .split(|byte| *byte == 0)
                .filter_map(|entry| {
                    let entry = std::str::from_utf8(entry).ok()?;
                    let (key, value) = entry.split_once('=')?;
                    (matches!(key, "PATH" | "LANG" | "HOME" | "SHELL")
                        || key.starts_with("LC_")
                        || key.starts_with("HOMEBREW_"))
                    .then(|| (key.to_string(), value.to_string()))
                })
                .collect();
        }
    }
    std::collections::HashMap::new()
}

#[cfg(target_os = "macos")]
fn capture(program: &str, args: &[&str]) -> Option<Vec<u8>> {
    let mut child = Command::new(program)
        .args(args)
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .ok()?;
    let deadline = Instant::now() + Duration::from_secs(3);
    loop {
        match child.try_wait() {
            Ok(Some(_)) => {
                return child
                    .wait_with_output()
                    .ok()
                    .filter(|output| output.status.success())
                    .map(|output| output.stdout);
            }
            Ok(None) if Instant::now() < deadline => std::thread::sleep(Duration::from_millis(20)),
            _ => {
                let _ = child.kill();
                let _ = child.wait();
                return None;
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn forwards_flags_and_preserves_flag_shaped_filename_after_separator() {
        let input = ["--xdg-config", "--vol", "35", "--", "--xdg-config"];
        assert_eq!(arguments(input.map(String::from)), input);
    }
}
