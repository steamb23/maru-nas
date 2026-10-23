# Upgrading

## Upgrading over SSH

Run these in the repository.

```sh
git pull
./install.sh
```

If you cloned `stable`, `git pull` brings in the latest release. If you checked out a tag to stay on a specific version, fetch with `git fetch --tags` instead of `git pull` and check out the new version's tag.

When Maru NAS is already installed, `./install.sh` builds and installs with the installed settings as they are. It shows a summary and lets you choose "Upgrade", "Change settings" or "Cancel". "Change settings" asks for every setting again with the current values as defaults, like `--reinstall`. As a regular user, you only need to enter your sudo password. The dashboard and the privileged helper restart, so every user has to sign in again.

- Only settings added in the new version are asked. For example, coming from a version before the security hardening mode existed, it asks for the public address for passkeys.
- Setting options given along with it (`--listen` and so on) replace only those existing values.
- If an existing value fails the new version's checks, it warns and asks for that setting again (showing the default instead of the existing value).
- In scripts, use `./install.sh --upgrade`. It upgrades without confirmation and fails if nothing is installed.
- If standard input is not a terminal, it upgrades without offering the choice. If a setting must be asked, it lists those options and stops, so give them as options or run again with `--reinstall`.

## Upgrading from the dashboard

Instead of running `git pull` and `./install.sh` over SSH, an administrator upgrades on the "Update" page of the Settings app. It looks for a new version in the local git repository used for the installation, builds it as the repository owner, and installs it as root.

### Turning it on

- When installing the privileged helper, allow it with `--web-upgrade` and turn it off with `--no-web-upgrade`. In a new installation the question defaults to allowing it.
- Upgrading from a version without this setting leaves it off. To turn it on, run `./install.sh --web-upgrade` or `./install.sh --reinstall`.
- The repository must be cloned by a regular user account. It cannot be allowed when root owns the repository.
- git and the build run as the repository owner, so that account must be able to use git, Go, Node.js, npm and make. Every time a regular user runs the install script, it records the directories where it found these tools as the build PATH, so if you moved the tools, run `./install.sh` again as the repository owner.
- The Update page is turned off in installations whose state directory was moved outside `/var/lib/maru` with `MARU_STATE_DIR`.

### Update channel

"Update channel" on the Update page chooses the branch that remote checks follow. The choice is saved in the dashboard settings, and choosing alone changes neither the repository nor the installation. Remote checks follow that branch from the next one on.

| Channel | Branch | Contents |
|---|---|---|
| Stable | `stable` | Released versions only. Right for most installations. |
| Development | `main` | Merged changes right away. These are commits before a release and less tested. |
| Current branch | The branch checked out now | Follows that branch's upstream (the same as before you chose a channel). Shown when the repository is on a branch other than `stable` or `main`. |

- The channel branch is fetched from `origin` by name. A clone with a single branch (`--single-branch` or `--depth`) works too. If you cloned from a fork, the fork must have `stable` and `main` branches.
- Upgrading on a channel other than the current branch moves the repository to that channel branch and makes it track the same branch on `origin`. In a single-branch clone, that branch is also added to the fetch configuration of `origin`. The original branch stays as it is.
- Going back from `main` to `stable` usually means a version older than the one installed. In that case the older version is installed over it, and the confirmation screen tells you how many commits will be dropped. The older version may fail to read dashboard settings that the newer one changed, or may lose some of them. If you go back to a version without the channel choice, its Update page has no channel and follows the current branch (`stable`).
- If the local branch with that channel's name has commits that are not on `origin`, it does not overwrite them and asks you to clean up over SSH.
- You cannot change the channel while a check or an upgrade is in progress. If you change the channel after a check, that result is no longer used to upgrade, so check again.

### Checking and upgrading

1. Checking on the Update page shows the new version, the commits in between, the settings the new version asks for and what to do after upgrading. There are two ways to check.
    - **Fetch and check**: runs `git fetch` as the repository owner and targets the branch of the [update channel](#update-channel) (for example `origin/stable`). If the channel is the branch checked out now (or the channel is "Current branch"), the current checkout must be able to fast-forward to it. It uses that account's git configuration and credentials as they are and never asks for a password or for an SSH host key confirmation, so `git fetch` must work for that account without asking anything.
    - **Check the local repository**: targets the current checkout (HEAD) that you updated over SSH. This also works for installations on a checked-out tag and for local commits.
    - Both need a working tree without uncommitted changes, and the target must descend from the installed commit. Only when moving to another channel is a target that does not descend from it (an older version) installed over the existing one.
2. Answer the settings it asks for and press "Upgrade". After root elevation with your password, it shows the progress.
3. maru and the privileged helper restart in the install step, so the screen changes to "Restarting Maru NAS". When Maru NAS is back, reload the page and sign in again, and the Update page shows the result.

If the build takes long and you do not use the dashboard meanwhile, the OMV session may end and send you back to the sign-in page first. The upgrade keeps going, and after you sign in again the Update page continues showing the progress. The service log is in `journalctl -u maru-upgrade`.

### When you must upgrade over SSH

If a change cannot be handled from the web, the page shows the reason and the commands to run over SSH instead of upgrading. Examples are a missing installation record, local commits that prevent a fast-forward, a new version that adds a setting the web cannot ask for, and a higher required Go or Node.js version.

Sign in over SSH as the repository owner and run `./install.sh` (or `./install.sh --reinstall`) in the repository. After one manual upgrade, you can upgrade from the dashboard again from the next version on.

If the reason only needs the repository to be cleaned up, as when the local branch with a channel's name has commits that are not on `origin` while moving channels, it asks you to clean up over SSH and check again, instead of showing commands. After you clean up and check again, you can upgrade from the dashboard.

### If it fails

- If it fails before or during the build, the installation is unchanged. If it fetched from the remote, though, the working tree may be left at the target commit (or on the other channel's branch, if it was moving channels).
- If it fails in the install step, the dashboard may not come up. Recover by running `./install.sh` again in the repository over SSH. It does not roll back to the previous version automatically.

### Security and caveats

- Turning this on extends what a root-elevated session can do to "upgrade from the installed repository". An upgrade runs code built from the repository as root, so an account that can change the repository (the repository owner) can run code as root through an upgrade, just as when running `./install.sh` over SSH. Treat the repository owner account like an administrator account, and review the commit list on the confirmation screen before upgrading.
- Checking does not need elevation, so a compromised dashboard could keep requesting checks. All a check does is a `git fetch` and a temporary build as the repository owner.
- The development channel (`main`) installs commits right away, before they are released. Review the commit list even more carefully before upgrading.
- If you do not need this feature, turn it off with `--no-web-upgrade`.
- Do not run `./install.sh` over SSH while an upgrade from the dashboard is in progress. They do not block each other.
- A version that changes the icon submodule commit needs that commit to be available from the submodule's remote (GitHub).
