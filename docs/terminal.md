# Web terminal

"Terminal" in "All apps" opens, in a window, a shell on this NAS's SSH server (sshd), signed in as the account you used for the dashboard. The dashboard connects over SSH to the SSH server on the same host (`127.0.0.1`) and relays the screen, so who can get in and how is decided by OMV's SSH settings as they are.

## Turning it on

- It is off by default. An administrator turns on "Web terminal" on the "Server" page of the Settings app, or sets `MARU_TERMINAL=true`. Turning it off also closes open terminals right away.
- The SSH service must be enabled in OMV, and your account must be in a group allowed to sign in over SSH (`_ssh` by default in OMV). You can add the group in the user editor of the OMV web UI.
- The OMV default administrator account (`admin`) has no shell and cannot use it. There is no way to connect as another account or as root. Do root work with `sudo` inside the shell.
- Administrators can require a password re-check or two-step verification before the web terminal with the [security hardening mode](security.md).

## Authentication

- **With the privileged helper**, you are not asked for your password again. For each connection the dashboard creates a one-time key, and the [privileged helper](helper.md) adds it to `~/.ssh/authorized_keys` as a line (comment `maru-ephemeral`) that is valid for 2 minutes and only for connections from this host, then removes it as soon as authentication finishes. This works even with SSH settings that accept only public keys ("Password authentication" off in OMV).
- **Without the helper, or when the key cannot be used**, you type the password the SSH server asks for (and the two-factor code) in the terminal. The password is not stored and goes straight to the SSH server.
- If your home directory is writable by its group or by other users, the SSH server ignores the keys in it, so the one-time key cannot be used. Remove that permission (`chmod go-w <home directory>`) or connect with your password.

If you use an SSH server that accepts only public keys without the helper, you can allow passwords only for connections from this host. Add the following to "Extra options" in the SSH settings of the OMV web UI. Extra options are inserted right before `Include /etc/ssh/sshd_config.d/*.conf`, so without the final `Match all`, the `Include` after it also falls under this condition.

```
Match Address 127.0.0.1,::1
    PasswordAuthentication yes
Match all
```

## Windows and sessions

- You can open several terminal windows. Each click on the terminal icon in "All apps" or in a shortcut opens a new window, and you can also right-click the terminal button on the taskbar (long-press on touch) and choose "New window". Each window opens its own shell, and one dashboard session can have up to 4 open at once, counting windows in other tabs.
- One dashboard session (a signed-in browser) shares one SSH connection, so opening a terminal in a new window or another tab does not ask again.
- Closing the window, signing out or the session expiring ends the shell. Typing in the terminal extends the session, but output alone does not, so run long tasks inside `tmux` or `screen`.
- Copy with Ctrl+Shift+C and paste with Ctrl+Shift+V.

## Security

- The dashboard is only a client of the SSH server. The SSH server is open to the LAN anyway, so a compromised dashboard gains no new rights, and SSH policies such as login records, PAM sessions and `AllowGroups` still apply. The host key is compared with `/etc/ssh/ssh_host_*_key.pub`, so the dashboard connects only to the same server.
- The one-time key stays within what the helper can already do (writing to the signed-in user's own home), so it adds no new rights. A line that could not be removed is rejected by the SSH server after 2 minutes anyway, and is removed on the next connection.
- The SSH server and fail2ban see every connection as coming from `127.0.0.1`. The dashboard therefore counts failed web terminal passwords per browser IP and user name itself and locks them out (by the same rule as sign-in). The dashboard log (`journalctl -u maru`) records only the user, the browser IP and the result.
