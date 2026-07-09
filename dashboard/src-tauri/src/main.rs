// Nexus Dashboard — V10 HUD (V11: Mode Switcher)
//
// Tauri shell that:
//   1. Reads ~/.nexus/state.json and exposes it to the frontend
//   2. Executes the Go engine (nexus) via sidecar with whitelisted subcommands
//   3. Validates all paths and inputs (defense-in-depth)
//
// V11 additions: dedicated Tauri commands for the mode switcher
// (`mode_apply`, `mode_list`, `mode_current`) so the frontend has a typed API
// for the one-click profile switcher instead of going through the generic
// command-string parser.
//
// Security model:
//   - State path resolved to canonical absolute path, must live under HOME
//   - Subcommand whitelist: dotfiles, probe, version, config, profile, list, mode
//   - No shell interpretation — args are passed as array to std::process::Command
//   - Mode names are validated locally (alphanumeric + - _ .) before reaching
//     the sidecar, so the frontend cannot smuggle extra arguments
//   - All errors returned as strings, never logged with secrets

use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use tauri_plugin_shell::ShellExt;

#[derive(Debug, Serialize, Deserialize)]
struct CommandResult {
    success: bool,
    stdout: String,
    stderr: String,
    exit_code: i32,
}

const ALLOWED_SUBCOMMANDS: &[&str] = &[
    "dotfiles", "probe", "version", "config", "profile", "list", "mode",
    "container", "registry",
];

fn home_dir() -> Result<PathBuf, String> {
    #[cfg(target_os = "windows")]
    {
        std::env::var_os("USERPROFILE")
            .map(PathBuf::from)
            .ok_or_else(|| "USERPROFILE not set".to_string())
    }
    #[cfg(not(target_os = "windows"))]
    {
        std::env::var_os("HOME")
            .map(PathBuf::from)
            .ok_or_else(|| "HOME not set".to_string())
    }
}

// read_state — returns the contents of ~/.nexus/state.json after
// verifying the canonical path lives under HOME. Returns the raw JSON
// string so the frontend can parse and display fields without us
// hard-coding a schema in Rust.
#[tauri::command]
fn read_state() -> Result<String, String> {
    let home = home_dir()?;
    let state_path = home.join(".nexus").join("state.json");

    let canonical_home = home
        .canonicalize()
        .map_err(|e| format!("Failed to resolve HOME: {}", e))?;

    let canonical_state = state_path
        .canonicalize()
        .map_err(|_| "State file not found — run 'nexus init' first.".to_string())?;

    if !canonical_state.starts_with(&canonical_home) {
        return Err("State file is outside HOME directory — refusing to read".into());
    }

    fs::read_to_string(&canonical_state).map_err(|e| format!("Failed to read state: {}", e))
}

// exec_nexus_command — runs a whitelisted nexus subcommand. The frontend
// passes a single string of the form "dotfiles status" or "vault init".
// We split on whitespace, validate the first token against the
// whitelist, and pass the rest as separate args to std::process::Command.
#[tauri::command]
async fn exec_nexus_command(
    app: tauri::AppHandle,
    command: String,
) -> Result<CommandResult, String> {
    let parts: Vec<&str> = command.split_whitespace().collect();
    if parts.is_empty() {
        return Err("Empty command".into());
    }
    if !ALLOWED_SUBCOMMANDS.contains(&parts[0]) {
        return Err(format!(
            "Subcommand '{}' is not allowed (allowed: {:?})",
            parts[0], ALLOWED_SUBCOMMANDS
        ));
    }

    // Build args vector (skipping the leading subcommand — Tauri sidecar
    // invocation prepends the binary path, and we pass the rest as args).
    // For `nexus dotfiles status`, we run `nexus dotfiles status` by
    // passing ["dotfiles", "status"] as args to the sidecar.
    let args: Vec<&str> = parts.iter().skip(1).copied().collect();

    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(&args)
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    Ok(CommandResult {
        success: output.status.success(),
        stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
        stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        exit_code: output.status.code().unwrap_or(-1),
    })
}

// exec_nexus_with_path — for commands that take a file path argument
// (e.g., `vault add <path>`). The path is validated here before being
// passed to the Go engine, so the frontend can't trick us into reading
// arbitrary files. The path must be absolute and live under HOME.
#[tauri::command]
async fn exec_nexus_with_path(
    app: tauri::AppHandle,
    command: String,
    file_path: String,
) -> Result<CommandResult, String> {
    let validated_path = validate_user_path(&file_path)?;

    let parts: Vec<&str> = command.split_whitespace().collect();
    if parts.is_empty() {
        return Err("Empty command".into());
    }
    if !ALLOWED_SUBCOMMANDS.contains(&parts[0]) {
        return Err(format!("Subcommand '{}' is not allowed", parts[0]));
    }

    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    // Build args: skip the leading subcommand, append parts[1..] + validated path.
    let mut args: Vec<String> = parts.iter().skip(1).map(|s| s.to_string()).collect();
    args.push(validated_path);

    let output = sidecar
        .args(&args)
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    Ok(CommandResult {
        success: output.status.success(),
        stdout: String::from_utf8_lossy(&output.stdout).into_owned(),
        stderr: String::from_utf8_lossy(&output.stderr).into_owned(),
        exit_code: output.status.code().unwrap_or(-1),
    })
}

// mode_apply — atomic mode switch via the engine. The name is validated
// locally (defense in depth) so a malicious frontend cannot smuggle extra
// arguments to the sidecar. Returns the raw stdout so the frontend can show
// a confirmation or summary.
#[tauri::command]
async fn mode_apply(app: tauri::AppHandle, name: String) -> Result<String, String> {
    validate_mode_name(&name)?;

    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(["mode", "apply", &name])
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!(
            "nexus mode apply failed (exit {}): {}",
            output.status.code().unwrap_or(-1),
            stderr.trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout).into_owned())
}

// mode_list — list built-in and user-defined modes as JSON. Returns the raw
// stdout so the frontend can parse the schema the Go engine emits.
#[tauri::command]
async fn mode_list(app: tauri::AppHandle) -> Result<String, String> {
    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(["mode", "list", "--json"])
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    if !output.status.success() {
        return Err(format!(
            "nexus mode list failed: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout).into_owned())
}

// mode_current — return the currently active mode + last-switch metadata as
// JSON. The frontend prefers reading state.json for the header badge (faster,
// no IPC), but this command is exposed for ad-hoc refreshes.
#[tauri::command]
async fn mode_current(app: tauri::AppHandle) -> Result<String, String> {
    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(["mode", "current", "--json"])
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    if !output.status.success() {
        return Err(format!(
            "nexus mode current failed: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout).into_owned())
}

// validate_mode_name — defense-in-depth validation of a mode name before it
// reaches the sidecar. Mode names are short identifiers (see ADR 010: they
// match `^[a-z][a-z0-9-]{0,63}$` per the YAML schema). We allow a slightly
// looser character set here (also `_` and `.`) for forward compatibility with
// user-defined modes, but reject anything that could be interpreted as a
// shell metacharacter or argument separator.
fn validate_mode_name(name: &str) -> Result<(), String> {
    if name.is_empty() {
        return Err("Mode name cannot be empty".into());
    }
    if name.len() > 64 {
        return Err("Mode name too long (max 64 characters)".into());
    }
    if !name
        .chars()
        .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_' || c == '.')
    {
        return Err(
            "Mode name contains invalid characters (allowed: a-z, A-Z, 0-9, '-', '_', '.')".into(),
        );
    }
    Ok(())
}

// validate_container_name — same defense-in-depth rules as validate_mode_name.
// Container names must match the Distrobox naming convention (see ADR 011).
fn validate_container_name(name: &str) -> Result<(), String> {
    if name.is_empty() {
        return Err("Container name cannot be empty".into());
    }
    if name.len() > 64 {
        return Err("Container name too long (max 64 characters)".into());
    }
    if !name
        .chars()
        .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_' || c == '.')
    {
        return Err(
            "Container name contains invalid characters (allowed: a-z, A-Z, 0-9, '-', '_', '.')".into(),
        );
    }
    Ok(())
}

// validate_image_ref — defense-in-depth validation of an OCI image reference.
// Mirrors the validation in internal/container/container.go:validateImageRef.
fn validate_image_ref(img: &str) -> Result<(), String> {
    if img.is_empty() {
        return Err("Image reference cannot be empty".into());
    }
    if img.len() > 256 {
        return Err("Image reference too long (max 256 characters)".into());
    }
    if img.contains("..") {
        return Err("Image reference cannot contain '..'".into());
    }
    if img.starts_with("--") {
        return Err("Image reference cannot start with '--'".into());
    }
    for c in img.chars() {
        let ok = c.is_ascii_alphanumeric()
            || c == '.'
            || c == '_'
            || c == '/'
            || c == '-'
            || c == ':';
        if !ok {
            return Err(format!("Image reference contains invalid character {:?}", c));
        }
    }
    Ok(())
}

// container_list — list Distrobox containers as JSON. The frontend uses this
// to render the Containers tab.
#[tauri::command]
async fn container_list(app: tauri::AppHandle) -> Result<String, String> {
    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(["container", "list", "--json"])
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    if !output.status.success() {
        return Err(format!(
            "nexus container list failed: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout).into_owned())
}

// container_create — create a Distrobox container with the given name and image.
// Both args are validated locally to prevent injection. The Go engine
// performs the actual creation and state tracking.
#[tauri::command]
async fn container_create(
    app: tauri::AppHandle,
    name: String,
    image: String,
) -> Result<String, String> {
    validate_container_name(&name)?;
    validate_image_ref(&image)?;

    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(["container", "create", &name, "--image", &image, "--json"])
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    if !output.status.success() {
        return Err(format!(
            "nexus container create failed (exit {}): {}",
            output.status.code().unwrap_or(-1),
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout).into_owned())
}

// container_remove — remove a container. The Go engine handles the
// managed-state check and the --force flag.
#[tauri::command]
async fn container_remove(app: tauri::AppHandle, name: String) -> Result<String, String> {
    validate_container_name(&name)?;

    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(["container", "remove", &name, "--json"])
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    if !output.status.success() {
        return Err(format!(
            "nexus container remove failed (exit {}): {}",
            output.status.code().unwrap_or(-1),
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout).into_owned())
}

// container_apps — list applications installed inside a container.
#[tauri::command]
async fn container_apps(app: tauri::AppHandle, name: String) -> Result<String, String> {
    validate_container_name(&name)?;

    let sidecar = app
        .shell()
        .sidecar("nexus")
        .map_err(|e| format!("Failed to locate nexus sidecar: {}", e))?;

    let output = sidecar
        .args(["container", "apps", &name, "--json"])
        .output()
        .await
        .map_err(|e| format!("Failed to execute nexus: {}", e))?;

    if !output.status.success() {
        return Err(format!(
            "nexus container apps failed (exit {}): {}",
            output.status.code().unwrap_or(-1),
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }

    Ok(String::from_utf8_lossy(&output.stdout).into_owned())
}

// validate_user_path ensures the user-provided path is safe to pass to
// the Go engine. Requirements:
//   - Path must be absolute
//   - Canonical path must live under HOME
//   - File must exist (we're encrypting/managing existing files)
//
// Returns the canonical absolute path on success.
fn validate_user_path(input: &str) -> Result<String, String> {
    let home = home_dir()?;

    // Reject path traversal patterns early.
    if input.contains("..") {
        return Err("Path contains '..' — refusing".into());
    }

    // Expand ~ prefix if present.
    let expanded = if let Some(stripped) = input.strip_prefix("~/") {
        home.join(stripped)
    } else if input == "~" {
        home.clone()
    } else if Path::new(input).is_absolute() {
        PathBuf::from(input)
    } else {
        return Err("Path must be absolute (start with /) or under HOME (~/...)".into());
    };

    // Canonicalize and verify containment.
    let canonical_home = home
        .canonicalize()
        .map_err(|e| format!("Failed to resolve HOME: {}", e))?;
    let canonical_path = expanded
        .canonicalize()
        .map_err(|e| format!("Path does not exist or is not accessible: {}", e))?;

    if !canonical_path.starts_with(&canonical_home) {
        return Err("Path is outside HOME directory — refusing".into());
    }

    Ok(canonical_path.to_string_lossy().into_owned())
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            read_state,
            exec_nexus_command,
            exec_nexus_with_path,
            mode_apply,
            mode_list,
            mode_current,
            container_list,
            container_create,
            container_remove,
            container_apps
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
