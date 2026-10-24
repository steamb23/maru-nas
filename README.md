# <img src="frontend/src/assets/logo/maru-lockup.svg" alt="마루 NAS (Maru NAS)" height="80">

> A suspiciously normal desktop-like dashboard for openmediavault.

[![CI](https://github.com/steamb23/maru-nas/actions/workflows/ci.yml/badge.svg)](https://github.com/steamb23/maru-nas/actions/workflows/ci.yml)
[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue)](LICENSE)

한국어: [README.ko.md](README.ko.md)

Maru NAS is a desktop-like web dashboard that runs on OpenMediaVault 8. Windows, a taskbar and desktop widgets show how your NAS is doing, and you can start and stop containers and core services in a few clicks. Deep configuration stays in the OMV web UI.

- You sign in with your OMV account. Changes are made only through the signed-in user's own OMV session.
- A single Go executable (frontend included) runs as a systemd service, and a reverse proxy in front of it handles HTTPS.

> [!WARNING]
> For local networks only. It is not designed to be exposed to the internet, and it has not been tested enough to know what security weaknesses would show if it were.

## Features

- **Status windows and widgets**: see system information, storage, network, temperatures and disk health (SMART) in windows or as desktop widgets.
- **Simple operations**: start, stop and restart containers (compose plugin), and turn core services on and off. Administrators can also shut down, restart or put the NAS to sleep from the profile menu.
- **[File explorer](docs/en/files.md)**: browse your home folder and shared folders with your own permissions, upload, download, and delete to the trash.
- **[Web terminal](docs/en/terminal.md)**: open an SSH shell on this NAS in a window, as the account you signed in with.
- **[System updates](docs/en/system-updates.md)**: check for OMV and Debian package updates and install them all. The installation runs to the end on the server even if you close the window, and you get a notification with the result.
- **[Security hardening mode](docs/en/security.md)**: ask for the password again, or for a passkey or an authenticator app, before sensitive features. You can also [sign in with a passkey](docs/en/passkey-login.md).
- **[Upgrading from the dashboard](docs/en/upgrade.md#upgrading-from-the-dashboard)**: upgrade to a new version from the Settings app, without SSH.
- **[OMV Workbench link](docs/en/configuration.md#opening-the-omv-workbench)**: open the OMV web UI in a new tab when you need deep configuration.

## Quick start

Start from an SSH session on a freshly installed OMV 8. Even with no build tools and no reverse proxy, following the steps below in order gets you to the sign-in page in your browser.

You need:

- OpenMediaVault 8.0 or later ([differences between versions](docs/en/install.md#omv-versions))
- A regular user in the sudo group, signed in over SSH. The user created by the OMV installation image is one ([accounts and SSH](docs/en/install.md#accounts-and-ssh))
- Internet access while building (github.com, registry.npmjs.org, proxy.golang.org) and about 1.1 GB of free space

### 1. Install the build tools

```sh
sudo apt update
sudo apt install git make nodejs npm
sudo apt install -t trixie-backports golang-go
```

The last line installs Go 1.27 from Debian backports, which OMV enables by default. You need `go version` 1.27 or later and `node --version` 20.19 or later. If you use Go 1.24 from the default repository, see [Build tools](docs/en/install.md#build-tools).

### 2. Download and install

```sh
git clone -b stable https://github.com/steamb23/maru-nas.git
cd maru-nas
./install.sh
```

Run it **without sudo**. It asks for the settings, shows a summary for you to confirm, then builds, and asks for your sudo password once in the install step. Leave most answers at their defaults, and answer these two to match the proxy example in step 3.

| Question | Answer |
|---|---|
| OMV Workbench address for browsers | `/` |
| Public dashboard address for passkeys | `-` (turns passkeys off. Once you have an HTTPS domain your browser trusts, change it with `./install.sh --reinstall`) |

### 3. Install a reverse proxy (Caddy)

The dashboard listens only inside this NAS (`127.0.0.1:8090`), so a reverse proxy provides the HTTPS address your browser connects to. Install Caddy from the Debian repository.

```sh
sudo apt install caddy
```

Because the OMV web UI uses port 80, the installation may report that it could not start `caddy.service`. Changing the configuration as below fixes that.

Replace the contents of `/etc/caddy/Caddyfile` with the following (for example with `sudo nano /etc/caddy/Caddyfile`). Replace `nas.example.lan` with the address you use to reach OMV in your browser now (a host name or an IP address).

```caddyfile
{
	auto_https disable_redirects
}

nas.example.lan {
	tls internal
	redir /desk /desk/ 308
	handle_path /desk/* {
		reverse_proxy 127.0.0.1:8090
	}
	handle {
		reverse_proxy 127.0.0.1:80
	}
}
```

```sh
sudo systemctl restart caddy
```

Caddy uses only port 443 and does not change the OMV settings. `https://<address>/desk/` goes to the dashboard and any other `https://<address>/` to the OMV web UI, and the OMV web UI you use now at `http://<address>/` stays as it is. SSL/TLS in the OMV web UI settings must stay off (the default).

### 4. Sign in

Open `https://<address>/desk/` in your browser and sign in with your OMV account.

- `tls internal` uses a certificate Caddy creates itself, so the browser warns at first. To remove the warning or to use passkeys, add Caddy's root certificate to your devices ([Reverse proxy](docs/en/reverse-proxy.md#caddy-example-keeping-the-omv-default-port)).
- Subdomains, Caddy in Docker and other proxies are covered in [Reverse proxy](docs/en/reverse-proxy.md).

### Upgrading and uninstalling

Running the script from the repository again upgrades with the installed settings. Administrators can also upgrade from the dashboard, on the "Update" page of the Settings app ([Upgrading](docs/en/upgrade.md)).

```sh
git pull
./install.sh
```

To uninstall, run `./install.sh --uninstall`.

## Documentation

What you look up while running Maru NAS after installation is on the [documentation site](https://steamb23.github.io/maru-nas/en/). The site normally follows the latest release (`stable`), but between releases it may already show the documentation of `main`, the code in development. You can read the same pages on GitHub in [docs/en/](docs/en/README.md).

| Page | Contents |
|---|---|
| [Installation](docs/en/install.md) | Requirements and differences between OMV versions, build tools, the install script, reinstalling, automation, uninstalling |
| [Install options](docs/en/install-options.md) | Every option of `install.sh` |
| [Upgrading](docs/en/upgrade.md) | Upgrading over SSH, upgrading from the dashboard |
| [Reverse proxy](docs/en/reverse-proxy.md) | Listen modes, proxy requirements, installing Caddy and examples |
| [Configuration](docs/en/configuration.md) | Every setting in `/etc/default/maru`, values changed on the web, backups, opening the OMV Workbench |
| [Settings app](docs/en/settings-app.md) | Pages of the Settings app and who can use them |
| [File explorer](docs/en/files.md) | Visible folders and permissions, uploading and downloading, the trash, moving and copying |
| [Web terminal](docs/en/terminal.md) | Turning it on, authentication, SSH settings and security |
| [System updates](docs/en/system-updates.md) | Checking for and installing all updates, task progress and result notifications, limitations |
| [Privileged helper](docs/en/helper.md) | The service that does the work needing user or root rights, and its security design |
| [Security hardening mode](docs/en/security.md) | Password re-check and two-step verification (passkeys, authenticator apps, recovery codes), recovery |
| [Passkey sign-in](docs/en/passkey-login.md) | Signing in with a passkey instead of a password, and its limits |

## Contributing

How to contribute (development setup, branches and pull requests, translations, documentation) is in [CONTRIBUTING.md](CONTRIBUTING.md) (Korean: [CONTRIBUTING.ko.md](CONTRIBUTING.ko.md)). Before starting a new feature, open an issue so we can agree on the design first.

You can run the development server against a fake OMV, without a real one. You need the same tools as in step 1 of the quick start. On a PC rather than a NAS, use the releases from [go.dev/dl](https://go.dev/dl/) and [nodejs.org](https://nodejs.org/). Vitest 5, used by the tests (`make check`), officially supports Node.js 22.12 and later.

```sh
git clone https://github.com/steamb23/maru-nas.git
cd maru-nas
git submodule update --init --depth 1
npm --prefix frontend ci
make dev-fake
```

Open `http://<host>:5173/` and sign in with the fake account `admin` (password `admin`). Other fake accounts and common commands are in CONTRIBUTING.

- Open pull requests against `main`. `main` is the code in development, and `stable` points at the latest release.
- Documentation lives in `docs/`, and you can preview it with [Zensical](https://zensical.org/) ("Documentation" in CONTRIBUTING).

## License

Distributed under the GNU General Public License v3.0 ([LICENSE](LICENSE)). The sources and licenses of bundled third-party assets such as icons are in [vendor/icons/README.md](vendor/icons/README.md).
