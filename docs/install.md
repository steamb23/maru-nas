# Installation

한국어: [설치](install.ko.md)

If this is your first installation, following the [quick start](https://github.com/steamb23/maru-nas#quick-start) in the README is enough. This page collects what is worth knowing about each of its steps. The list of options is in [Install options](install-options.md), and the values you change after installing are in [Configuration](configuration.md).

## Requirements

### OMV versions

Maru NAS works with every release of OpenMediaVault 8 (8.0 and later). The following depends on the version.

- OMV two-factor authentication is available only in OMV 8.5 and later.
- OMV 8.0 and 8.1 do not report the automatic logout time. The dashboard therefore does not show how much time is left in the session, and it notices that the OMV session has ended only when it checks periodically.

Check your version with `dpkg-query -W openmediavault`. If you have just installed from the installation image (ISO), you have the image's version as it is, so we recommend updating first, under System → Update Management in the OMV web UI or with `sudo omv-upgrade`.

### Accounts and SSH

Install as a regular user account that can use sudo. The regular user created by the OMV installation image is in the sudo group.

- OMV allows SSH only for root and members of the `_ssh` group. To sign in to that account over SSH, add the `_ssh` group to it in the user editor of the OMV web UI.
- If root is the only account, you can run the script as root (it continues with the install step without sudo). With a repository owned by root, however, [upgrading from the dashboard](upgrade.md#upgrading-from-the-dashboard) cannot be turned on.

### Build tools

Right after installing from the OMV installation image, none of the tools below are present, but all of them are in the apt repositories OMV 8 (Debian 13) uses by default.

| Tool | Required version | apt package (version) |
|---|---|---|
| Go | 1.27.1 or later | `golang-go` (1.27 in trixie-backports, 1.24 in the default repository) |
| Node.js | 20.19 or later, or 22.12 or later | `nodejs` (20.19) |
| npm | 7 or later | `npm` (9.2). On Debian it is installed separately from `nodejs` |
| make | GNU make | `make` (4.4) |
| git | | `git` (2.47) |

```sh
sudo apt update
sudo apt install git make nodejs npm
sudo apt install -t trixie-backports golang-go
```

- The last line installs Go 1.27 from the Debian trixie-backports repository, which OMV enables by default.
- Without `-t trixie-backports` you get Go 1.24. The first build then has Go download the 1.27.1 toolchain from proxy.golang.org automatically, which adds about 330 MB under `~/go/pkg/mod`. If you set `GOTOOLCHAIN=local` or cannot reach proxy.golang.org, install Go from backports or from [go.dev/dl](https://go.dev/dl/).
- With Node.js 20, `npm ci` shows an `EBADENGINE` warning for Vitest. The warning is about a development test tool and does not affect the build.
- The build connects to github.com (icon submodule), registry.npmjs.org (npm dependencies) and proxy.golang.org (Go dependencies). Installing the tools and building needs about 1.1 GB of free space (about 1.4 GB with Go 1.24 from the default repository).

## Getting the source

The `stable` branch is the latest release and `main` is the code in development, so install from `stable`. The install script fetches the icon submodule, so you do not need `--recurse-submodules`.

```sh
git clone -b stable https://github.com/steamb23/maru-nas.git
cd maru-nas
```

To install a specific version, check out its tag after cloning (for example `git checkout v0.10.0`). Releases and their changes are listed under [Releases](https://github.com/steamb23/maru-nas/releases).

## Install script

As a regular user account, run it **without sudo**.

```sh
./install.sh
```

1. **Settings**: it asks for the service user, the listen mode, the OMV address, the SMART refresh time, the OMV Workbench address, the public address for passkeys, whether to install the privileged helper and whether to allow upgrading from the dashboard, then shows a summary and asks you to confirm. All input happens in this step.
2. **Build**: it builds as the account that ran it. In a fresh clone it first fetches the icon submodule and the npm dependencies.
3. **Install**: it asks for your sudo password once and installs as root. As root it continues without asking.

If Maru NAS is already installed, it does not ask for settings and upgrades with the installed settings ([Upgrading](upgrade.md)).

- In a terminal, it asks in a full-screen interface (TUI) where you choose with the arrow keys and type values, and it points out invalid values as you type. With `--plain` or `TERM=dumb` it asks one line at a time.
- If standard input is not a terminal (a pipe, automation and so on), it does not ask. It lists the missing setting options and stops ([Automation](#automation)).
- Messages are shown in English or Korean depending on the locale, and you can also choose with `--lang`.
- At the end it tells you the address to put in your reverse proxy. The install script does not configure the reverse proxy ([Reverse proxy](reverse-proxy.md)).

### What gets installed

| Path | Contents |
|---|---|
| `/usr/local/bin/maru` | The dashboard executable. It runs as the service user (`maru` by default, created as a system user if missing) |
| `/etc/default/maru`, `/etc/systemd/system/maru.service` | The [configuration](configuration.md) file and the systemd unit |
| `/var/lib/maru` | The dashboard's own state: user settings (widget layout, shortcuts, screen settings), the administrators' global settings, uploaded wallpapers, sign-in methods of the security hardening mode |
| `/usr/local/bin/maru-helper`, `/etc/default/maru-helper`, `maru-helper.socket` and `.service` | The [privileged helper](helper.md) (installed by default) |
| `/usr/local/lib/maru/maru-install`, `/etc/default/maru-upgrade`, `maru-upgrade.service` and `.path` | When [upgrading from the dashboard](upgrade.md#upgrading-from-the-dashboard) is allowed |
| `/var/lib/maru-upgrade` | The record of the installed version and commit, and the upgrade status |

If you choose not to install the privileged helper or upgrading from the dashboard, what was installed before is removed.

### Changing settings

```sh
./install.sh --reinstall
```

It asks for every setting again, with the current settings as defaults.

- If the settings change, the existing `/etc/default/maru` is backed up to `/etc/default/maru.bak-<time>`.
- Lines the installer does not ask about are kept. Web terminal lines (`MARU_TERMINAL` and so on) keep their existing values, and other `MARU_*` lines you added by hand (for example `MARU_COOKIE_PATH`) move to the end of the file.

### Automation

Settings given as options are not asked. A first install and `--reinstall` still show a summary and ask whether to proceed, so they stop at this confirmation when standard input is not a terminal. Only an upgrade (`--upgrade`, or no options without a TTY) and `--service-install` as root with every setting given finish without confirmation.

To automate a first install, build with `make build` first and then run the following as root. Add `--dry-run` to the same options to see first what would change.

```sh
./install.sh --service-install --user maru --listen unix:/run/maru/maru.sock \
  --omv-url http://127.0.0.1 --smart-refresh-at 00:00 --workbench-url https://nas.example.lan/ \
  --public-origin '' --no-helper
```

### Running as root

If you run it as root (`sudo ./install.sh`, `sudo -i`, `su` and so on) and the repository is not owned by root, it skips the build. Building as root would leave root-owned files in the repository, and the PATH under sudo may not include the Go and npm you installed.

- It then uses the existing `bin/maru-install` and asks whether to just install the existing `bin/maru`. With `--upgrade`, or for an upgrade whose standard input is not a terminal, it stops instead of asking.
- It also uses the existing `bin/maru-install` when `go` is not in the PATH.

## Uninstalling

```sh
./install.sh --uninstall
```

- It removes the services, executables and configuration. If the privileged helper and the dashboard upgrade service were installed, they are removed too, along with the installation record and the upgrade status (`/var/lib/maru-upgrade`).
- `/var/lib/maru` and the service user are removed only after you confirm.
- `/run/maru` is removed as well, so if you mounted this directory into a proxy container, recreate the proxy container after installing again.
