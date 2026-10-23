# Privileged helper

The privileged helper (`maru-helper`) is a separate service that runs as root. The dashboard itself runs as a dedicated unprivileged user and cannot raise its privileges, so the following work goes through the helper.

- **User operations**: read and write files as the signed-in user's own Linux account (uid, primary group, supplementary groups). The kernel decides from ownership and ACLs what can actually be opened, so the same permissions apply as over SSH. Only that user's home directory and the data disks mounted under `/srv` are in scope. The [file explorer](files.md) uses these, and the one-time SSH key of the [web terminal](terminal.md) is also added and removed with these rights.
- **Root operations**: when an administrator enters their password again, that dashboard session can use a fixed set of root operations for 5 minutes (similar to sudo). There is no way to run arbitrary commands, and each feature gets only the operations it needs. Currently these are checking whom operations run as, the upgrade request of [upgrading from the dashboard](upgrade.md#upgrading-from-the-dashboard), and creating a disk trash for the file explorer.

OMV shared folder privileges are settings for services such as SMB and FTP, so the helper does not follow them. To have the file explorer follow them, turn on [following OMV shared folder privileges](files.md#following-omv-shared-folder-privileges), and to block access at the helper level, change the shared folder's ACL in OMV.

The install script installs it by default. If you choose not to install it when asked, or pass `--no-helper`, it is not installed. The features that need the helper are then unavailable, but everything else works as usual. Administrators can check the installation status, whom the current session runs as, and root elevation on the "Server" page of the Settings app.

## Security design

The helper does not simply trust what the dashboard says, so that a compromised dashboard does not immediately get root or every user's rights.

- Only the dashboard service user can connect to the socket (`/run/maru-helper/helper.sock`, mode `0600`).
- At sign-in, the helper checks the password the dashboard received by itself, and only then issues a ticket for that session. Tickets live only in the helper's memory. They are discarded on sign-out or session expiry, and all of them vanish when the helper or the dashboard restarts. For accounts that use two-factor authentication, the ticket can be used only after that verification is complete.
- Tickets are issued by the same rule as OMV sign-in. Root does not get one, and the account must be in the `openmediavault-admin` group or in the regular user uid range (`UID_MIN` to `UID_MAX` in `/etc/login.defs`).
- Root elevation is available only to users in both `openmediavault-admin` and the elevation group (`sudo` by default, changed with `--elevate-group` when installing), and only after they enter their password again. Elevation is tied to that session and ends after 5 minutes (operations do not extend it), and it can be ended right away on the Server page. If the elevation group is `openmediavault-admin`, every OMV administrator can elevate.
- After 3 failed password checks within 5 minutes, the user is locked out, starting at 30 seconds and doubling up to 15 minutes. This lockout never locks an account permanently.
- Each operation runs in a child process, and user operations drop to that user's rights before they start. The root process does not handle file contents.
- The systemd unit leaves the helper, even as root, only the capabilities and writable paths it needs. You can review it with `systemd-analyze security maru-helper`.
- Passwords and tickets are never logged. The helper records the results of password checks, every root operation, and file write operations (with their paths) in `journalctl -u maru-helper`. File reads are recorded without paths, as the operation and its result only.

## Remaining risks

- If the dashboard is compromised, an attacker can use the tickets of the users signed in at that time, and can intercept passwords entered after that.
- OMV administrators can already do the equivalent of root through the OMV web UI, so the helper's root elevation gives administrators no new power.
- Turning on upgrading from the dashboard widens what an elevated session can do ([Security and caveats](upgrade.md#security-and-caveats)).

## Limitations

- Accounts are checked only against the local `/etc/passwd`, `/etc/group` and `/etc/shadow`. Accounts that come from the LDAP plugin and the like can sign in to the dashboard but do not get a helper ticket.
- If a home directory is not under `/home` or `/srv`, it can only be read.

## Checking the installation

```sh
systemctl status maru-helper.socket
```

After signing in, check on the "Server" page of the Settings app that "Runs as" shows your account's uid, and that after elevation "Runs as (root)" shows uid 0. To see the requests the helper received and their results, run the following.

```sh
journalctl -u maru-helper -e
```
