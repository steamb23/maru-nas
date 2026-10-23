# Maru NAS documentation

Maru NAS is a desktop-like web dashboard that runs on OpenMediaVault 8. If you are installing it for the first time, follow the [quick start](https://github.com/steamb23/maru-nas#quick-start) in the repository README. These pages cover what you look up while running it after installation.

## Installation and operation

| Page | Contents |
|---|---|
| [Installation](install.md) | Requirements and differences between OMV versions, build tools, the install script's flow and what it installs, reinstalling, automation, uninstalling |
| [Install options](install-options.md) | Every option of `install.sh` |
| [Upgrading](upgrade.md) | Upgrading over SSH, upgrading from the dashboard |
| [Reverse proxy](reverse-proxy.md) | Listen modes, proxy requirements, installing Caddy and examples |
| [Configuration](configuration.md) | Every setting in `/etc/default/maru`, values changed on the web, backups, opening the OMV Workbench |

## Features

| Page | Contents |
|---|---|
| [Settings app](settings-app.md) | Pages of the Settings app and who can use them |
| [File explorer](files.md) | Browsing your home folder and shared folders, uploading and downloading, the trash, moving and copying |
| [Web terminal](terminal.md) | An SSH shell in a dashboard window |

## Security

| Page | Contents |
|---|---|
| [Privileged helper](helper.md) | The service that does the work needing user or root rights, and its security design |
| [Security hardening mode](security.md) | Password re-check and two-step verification (passkeys, authenticator apps, recovery codes) before sensitive features |
| [Passkey sign-in](passkey-login.md) | Signing in with a passkey instead of a password, and its limits |

You can read these pages on GitHub as they are in the repository's `docs/en/`, or on the [documentation site](https://steamb23.github.io/maru-nas/en/) built with [Zensical](https://zensical.org/). The site normally follows the latest release (`stable`), and work in progress shows up first in [docs/en/ on `main`](https://github.com/steamb23/maru-nas/tree/main/docs/en). Between releases, the site may already show the documentation of `main`. How to edit them is under "Documentation" in [CONTRIBUTING.md](https://github.com/steamb23/maru-nas/blob/main/CONTRIBUTING.md).
