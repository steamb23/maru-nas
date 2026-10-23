# Configuration

한국어: [설정](configuration.ko.md)

After installation, Maru NAS is configured with environment variables in `/etc/default/maru` or with command-line flags. After changing a value, restart it with `sudo systemctl restart maru`. Values the installer asks for are safer to change with `./install.sh --reinstall` ([Changing settings](install.md#changing-settings)).

Administrators can see the values in use and where each one comes from (default, environment variable, command-line flag) on the "Server" page of the [Settings app](settings-app.md).

## Settings you can change on the web

The workbench address (`-workbench-url`), the SMART check time (`-smart-refresh-at`) and whether the web terminal is allowed (`-terminal`) can also be changed on the "Server" page of the Settings app.

- Values changed there take effect right away without a restart, and take precedence over flags and environment variables (they are saved in `site.json` in the state directory).
- They stay when you reinstall or upgrade with `install.sh`. To go back to the flag value, press "Use server value" on that page.

## Settings

| Flag | Environment variable | Default | Description |
|---|---|---|---|
| `-listen` | `MARU_LISTEN` | `127.0.0.1:8090` | The listen address: a loopback `host:port` or `unix:/absolute/path.sock` (Unix socket, mode `0660`) ([Choosing a listen mode](reverse-proxy.md#choosing-a-listen-mode)). |
| `-allow-non-loopback` | `MARU_ALLOW_NON_LOOPBACK` | `false` | Also allows a TCP address that is not loopback (`0.0.0.0:8090` and so on). This is dangerous, so read the warning in [Choosing a listen mode](reverse-proxy.md#choosing-a-listen-mode) first. |
| `-omv-url` | `MARU_OMV_URL` | `http://127.0.0.1` | The address of the OMV web UI (nginx). Only loopback addresses are accepted. If you changed the OMV web UI port, include the port (the install script finds it in the nginx configuration and suggests it). Maru NAS connects over plain HTTP only, so "Force SSL/TLS" in OMV must be off. |
| `-cookie-path` | `MARU_COOKIE_PATH` | `/` | The dashboard path as the browser sees it (the Path of the session cookie). You can narrow it when the dashboard is under a path prefix. |
| `-state-dir` | `MARU_STATE_DIR`, `STATE_DIRECTORY` | (none) | The directory where the dashboard keeps its state. If empty, the state is kept only in memory and lost on restart. In an installation, systemd passes `/var/lib/maru` in `STATE_DIRECTORY`. |
| `-smart-refresh-at` | `MARU_SMART_REFRESH_AT` | `00:00` | The time of day after which SMART information is read again, once a day (`HH:MM`, server local time). If the last read was before this time, it is read again on the next visit. Reading SMART wakes disks in standby, so there is no automatic periodic read. |
| `-workbench-url` | `MARU_WORKBENCH_URL` | (none) | The address the browser uses to open the OMV web UI ([Opening the OMV Workbench](#opening-the-omv-workbench)). If empty, the "OMV Workbench" app and the profile menu item are hidden. |
| `-helper-socket` | `MARU_HELPER_SOCKET` | (none) | The absolute path of the [privileged helper](helper.md) socket (the install script uses `/run/maru-helper/helper.sock`). If empty, the features that use the helper are off. |
| `-terminal` | `MARU_TERMINAL` | `false` | Allows the [web terminal](terminal.md). It can also be turned on and off on the "Server" page of the Settings app. |
| `-ssh-addr` | `MARU_SSH_ADDR` | `auto` | The SSH server address the web terminal connects to. `auto` connects to `127.0.0.1` on the `Port` in `/etc/ssh/sshd_config`, so it follows a port changed in OMV. A value you set yourself must be a loopback `IP:port`. |
| `-ssh-host-keys` | `MARU_SSH_HOST_KEYS` | `/etc/ssh/ssh_host_*_key.pub` | A glob pattern for the SSH server host public key files the web terminal accepts. Usually left as it is. |
| `-public-origin` | `MARU_PUBLIC_ORIGIN` | (none) | The public HTTPS address browsers use to reach the dashboard (`https://host[:port]`, without a path). Used for passkeys in the [security hardening mode](security.md). It must be a domain name with a certificate the browser trusts, and IP addresses are not accepted. If empty, passkeys are unavailable and only authenticator apps (TOTP) and recovery codes can be used. |

## State directory and backups

- Sessions live only in the backend's memory, so every user must sign in again after the backend restarts.
- `/var/lib/maru` holds only the dashboard's own state and has nothing to do with the OMV configuration. Copying this directory along with your backups keeps user settings, global settings, uploaded wallpapers and sign-in methods of the security hardening mode.
- The sign-in method records contain authenticator app secrets, so protect the backups too ([Design and limitations](security.md#design-and-limitations)).

## Opening the OMV Workbench

With `-workbench-url` set, "OMV Workbench" in "All apps" and in desktop shortcuts, and "Open OMV Workbench" in the profile menu, open the OMV web UI in a new browser tab.

- The value is the address the browser uses to reach OMV (for example `https://nas.example.lan/`). It accepts an address starting with `http://` or `https://`, or a path relative to the dashboard (`/`, `./`, `../`). If the dashboard is under a path prefix (for example `/desk/`) on the same host as OMV, `/` works too. Addresses with a query or a fragment, and addresses without `http(s)://` such as `nas.example.lan/`, are not accepted.
- You sign in to OMV separately in that tab. OMV has no way to hand over a sign-in without the password, so the dashboard's sign-in cannot be passed on to the OMV tab. An OMV tab you already opened is reused, so you stay signed in as long as you keep that tab open. When a browser signs in for the first time, OMV may send a "new browser login" notification email.
- The OMV web UI can be shown in a window (iframe) only when it has the same origin (host and port) as the dashboard, so it opens in a browser tab rather than in a window inside the dashboard.
