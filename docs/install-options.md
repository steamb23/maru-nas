# Install options

These are the options of `./install.sh`. Without options, it asks for the settings and installs the first time, and upgrades with the installed settings if Maru NAS is already installed. The flow is in [Installation](install.md), and what happens during an upgrade is in [Upgrading](upgrade.md).

## Install modes

| Option | Description |
|---|---|
| (none) | Asks for the settings, builds and installs. If already installed, upgrades with the installed settings. |
| `--reinstall` | Asks for every setting again, with the installed values as defaults, and reinstalls. If nothing is installed, it is the same as a first install. |
| `--upgrade` | For scripts. Upgrades with the installed settings without confirmation, and fails if nothing is installed. Asks only for settings added in the new version. |
| `--uninstall` | Removes the services, executables and configuration ([Uninstalling](install.md#uninstalling)). |

## Running

| Option | Description |
|---|---|
| `--no-build` | Skips the build and installs the existing `bin/maru` (and `bin/maru-helper` if the privileged helper is installed). |
| `--service-install` | Runs only the install step, without building (root only). Installs without asking when every setting option is given ([Automation](install.md#automation)). |
| `--dry-run` | Does not run the install step. Shows only the changes to the files it would write and the commands it would run. It does not use sudo and does not change the system. Without `--no-build`, the build does run (it writes only inside the repository). |
| `--plain` | Asks one line at a time instead of using the TUI. |
| `--lang LANG` | Chooses the language of the messages, `en` or `ko`. |
| `-h`, `--help` | Shows the usage. |

## Settings

Settings given as options are not asked. Given with an upgrade, only those settings replace the existing values. For questions where you enter `-`, giving an empty value (`''`) does the same.

| Option | Description |
|---|---|
| `--user NAME` | The service user (default `maru`). |
| `--listen ADDR` | The listen address: `127.0.0.1:8090`, `unix:/run/maru/maru.sock` or `host:port` ([Choosing a listen mode](reverse-proxy.md#choosing-a-listen-mode)). |
| `--allow-non-loopback` | Allows a `--listen` address that is not loopback (dangerous). |
| `--omv-url URL` | The OMV web UI address (loopback). |
| `--smart-refresh-at HH:MM` | The time of day to read SMART information again. |
| `--workbench-url URL` | The address the browser uses to open the OMV web UI. Empty hides the "OMV Workbench" app ([Opening the OMV Workbench](configuration.md#opening-the-omv-workbench)). |
| `--public-origin URL` | The public HTTPS address browsers use to reach the dashboard (for passkeys). Empty turns passkeys off. |
| `--helper`, `--no-helper` | Installs or does not install the [privileged helper](helper.md). The question defaults to installing it. `--no-helper` removes an installed helper. |
| `--elevate-group GROUP` | The group required, together with `openmediavault-admin`, for root elevation (default `sudo`). Used only with `--helper`. |
| `--web-upgrade`, `--no-web-upgrade` | Allows or disallows upgrading from the dashboard. Used only with `--helper` ([Turning it on](upgrade.md#turning-it-on)). |
| `--build-path PATH` | The PATH for builds when upgrading from the dashboard. It is not asked: every time a regular user runs `./install.sh`, it is set to the directories where git, go, node, npm and make were found. |

## Internal

These options are used by upgrading from the dashboard. You never need to run them yourself.

| Option | Description |
|---|---|
| `--upgrade-plan` | Only reads the existing installation and prints, as JSON, the settings an upgrade to this version must ask for and the reasons a manual reinstall is needed. It does not change the system. |
| `--upgrade-service` | Handles one check or apply request of upgrading from the dashboard (root only). Run by the upgrade service (`maru-upgrade.service`). |
