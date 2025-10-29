# 🐘 phpenv – PHP Version Manager for Windows

**phpenv** is a simple, no-nonsense PHP version manager for Windows — inspired by tools like **nvm** and **pyenv**, but built in **Go** and designed specifically for Windows users.
It lets you install, switch, and manage multiple PHP builds, or use a custom runtime — all through an intuitive **interactive terminal UI**.

---

## 🚀 Quick Start

```bash
# Build from source
go build ./cmd/phpenv

# Add the shims directory to your PATH once
setx PATH "%LOCALAPPDATA%\phpenv\shims;%PATH%"

# Launch the UI
phpenv
```

Use the arrow keys (`↑`/`↓`) to move, **Enter** to select, **Esc** to go back, and `:` to open the command palette.

---

## ⚙️ What You Get

* 🔢 Manage and switch between PHP versions easily
* 💡 Interactive, keyboard-driven interface (Bubble Tea)
* 🧱 Works with both `x64` and `x86`, `nts` or `ts` builds
* 🗂 Per-project overrides with `.phpenv.json`
* ⚡ Instant environment updates (registry + PATH refresh)
* 🧹 No system pollution — everything stays under one folder
* 🧰 Full JSON configuration for power users

---

## 🧭 Commands

| Command                                | Description                           |                      |                                   |
| -------------------------------------- | ------------------------------------- | -------------------- | --------------------------------- |
| `phpenv`                               | Start the interactive UI              |                      |                                   |
| `phpenv help [command]`                | Show usage info                       |                      |                                   |
| `phpenv list [--remote] [--arch <x64   | x86>] [--nts                          | --ts]`               | List installed or remote versions |
| `phpenv install <version> [--arch <x64 | x86>] [--nts                          | --ts] [--force]`     | Install a PHP version             |
| `phpenv remove <version> [...]`        | Remove a version                      |                      |                                   |
| `phpenv use <version> [--arch <x64     | x86>] [--nts                          | --ts] [--desc text]` | Make a version active             |
| `phpenv use --path <php.exe>`          | Use a custom PHP outside phpenv       |                      |                                   |
| `phpenv which [dir]`                   | Show which PHP is used for a folder   |                      |                                   |
| `phpenv local [options] [version]`     | Manage local `.phpenv.json` overrides |                      |                                   |
| `phpenv exec php [args...]`            | Run PHP via phpenv’s resolver         |                      |                                   |
| `phpenv config`                        | Show the config file location         |                      |                                   |
| `phpenv menu`                          | Force-open the UI                     |                      |                                   |

---

## 💻 Interactive Mode

The **Bubble Tea** UI makes version management actually fun:

* Smooth scrolling lists and download progress bars
* Always-visible status bar with active PHP info
* A command line (`:`) for quick actions, e.g.:

  ```
  :install 8.3.0 --nts
  :use --path C:\php\php.exe
  ```
* Keyboard shortcuts:

  * `↑` / `↓` → Navigate
  * `Enter` → Select
  * `Esc` → Go back
  * `q` → Quit immediately

---

## 🧩 Configuration

Your config lives here by default:
📄 `%LOCALAPPDATA%\phpenv\config.json`

```json
{
  "root": "C:\\Users\\you\\AppData\\Local\\phpenv",
  "versions_dir": "C:\\Users\\you\\AppData\\Local\\phpenv\\versions",
  "cache_dir": "C:\\Users\\you\\AppData\\Local\\phpenv\\cache",
  "shims_dir": "C:\\Users\\you\\AppData\\Local\\phpenv\\shims",
  "env": {
    "PHPRC": "C:\\custom\\php.ini"
  },
  "path_additions": [
    "C:\\php-support\\bin"
  ],
  "default_arch": "x64",
  "default_thread_safety": "nts"
}
```

When you activate a version, the shim (`php.cmd`) sets all environment variables under `env`, prepends your `path_additions`, and ensures the right PHP directory comes first in `PATH`.
That means your global environment stays clean, but PHP runs exactly as configured.

---

## 🧱 Per-Project Overrides

If you work across projects using different PHP versions, drop a `.phpenv.json` file in your project root:

```json
{
  "version": "8.2.12-nts-x64",
  "env": {
    "PHPRC": "C:\\myapp\\php.ini"
  },
  "path_additions": ["C:\\myapp\\bin"]
}
```

phpenv will pick it up automatically when you’re inside that folder (and it’ll search upward too).

---

## 🪟 Windows Integration

On Windows, activating a version does the following:

* Updates `PHP_ROOT` and `CURRENT_PHP` in your user environment
* Patches your user-level `Path` in the registry
* Broadcasts an update so new shells instantly see the change — no reboot needed

### 🐧 macOS / Linux

For Unix systems, phpenv generates:

```
$PHPENV_ROOT/profile/phpenv.sh
```

Then just add this to your shell:

```bash
source "$PHPENV_ROOT/profile/phpenv.sh"
```

---

## 🌐 Remote Versions

phpenv pulls remote listings from:

* [https://windows.php.net/downloads/releases/](https://windows.php.net/downloads/releases/)
* [https://windows.php.net/downloads/releases/archives/](https://windows.php.net/downloads/releases/archives/)

If the structure of those pages changes, the scraper may need updating.

---

## 🧰 Examples

```bash
# Show all available remote builds
phpenv list --remote --nts --arch x64

# Install PHP 8.2.12 (non-thread-safe, x64)
phpenv install 8.2.12 --nts --arch x64

# Make it the active version
phpenv use 8.2.12 --nts --arch x64

# Confirm
php -v
```

---

## 🧼 Uninstalling

To remove everything:

1. Delete the root folder (`%LOCALAPPDATA%\phpenv`)
2. Remove the `shims` path from your system PATH

That’s it. Your system goes back to normal.

---

## 💡 Notes & Tips

* Default arch: **x64**
* Default build: **NTS** (recommended for CLI)
* Change paths or defaults anytime via `config.json`
* Set `PHPENV_ROOT` before first use if you want a custom location

---

## 🤝 Contributing

Ideas, PRs, and bug reports are always welcome.
If you build something cool on top of `phpenv`, share it — we’d love to see it.

---

## 🪪 License

MIT © 2025