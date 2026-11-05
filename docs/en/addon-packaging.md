# Packing and distribution

A package file (`.maru-addon`) is a tar of the package directory compressed with gzip. The SDK's `maru-addon` CLI checks and packs it.

## Files

```
my-addon/
├── manifest.json        required
├── icon.svg             required (SVG)
├── i18n/
│   ├── en.json          required
│   └── ko.json
├── ui/                  the views' HTML and assets (subdirectories allowed)
│   ├── window.html
│   ├── window.js
│   └── assets/…
├── backend/             the backend executables, if the manifest declares a backend
│   ├── linux-amd64
│   └── linux-arm64
├── LICENSE              optional
└── README.md            optional
```

- Only the names above can be at the top level. A `dev/` directory in a development directory is for development files and is not packed.
- Each path segment starts with a letter or digit and has up to 100 letters, digits, `.`, `_` and `-`. Hidden files (starting with `.`), spaces and non-Latin names are not accepted. Paths can be up to 16 levels deep.
- Only regular files and directories are accepted (no symbolic links and so on). File permissions are set at install time regardless of the archive.
- `i18n/` holds files only for languages the dashboard supports (`<language>.json`). `i18n/` and `backend/` cannot have subdirectories.
- `backend/` holds the backend executables, `linux-amd64` and `linux-arm64` (64-bit Linux ELF executables, built statically). A package that declares a backend is installed only if it has the executable for the server's architecture, so include both. Backend files without a `backend` declaration in the manifest are rejected ([Addon backends](addon-backend.md#constraints)).

## Limits

| Item | Limit |
|---|---|
| Package file | 64 MiB |
| Unpacked total | 160 MiB |
| Entries | 2000 |
| One file under `ui/` | 8 MiB |
| One backend executable | 64 MiB |
| `manifest.json` | 64 KiB |
| One translation file, `icon.svg` | 128 KiB |
| `LICENSE`, `README.md` | 256 KiB |

## maru-addon

Until the SDK is published to npm, run it from a checkout of the Maru NAS repository as `node sdk/js/cli/maru-addon.mjs`. It needs Node.js 20.19 or later and nothing else.

| Command | Contents |
|---|---|
| `init <directory> [--id <id>] [--ts] [--backend]` | Creates a new package. The id comes from the directory name; `--ts` makes a TypeScript + Vite project, and `--backend` adds a Go backend module next to the package directory (`addon/`) |
| `validate <directory or file>` | Checks a package directory or a `.maru-addon` file. In a directory, a missing backend executable is only a note |
| `pack <directory> [-o <file>]` | Checks and packs. The default name is `<id>-<version>.maru-addon`, and it prints the SHA-256 and the revision name (rev). It refuses a package that declares a backend but has no executable |

- The CLI checks the manifest's structure and the file layout with the same rules as the dashboard. Conditions between fields, control characters in texts, methods that are not allowed and conflicts with other addons are checked by the dashboard when it installs the package. Load the package in a development server once before packing (the "Addons" page of the Settings app shows the reasons).
- `pack` makes the same tar for the same files (sorted paths, zero timestamps and owners, fixed permissions). The gzip output can differ with the machine's zlib version, so the SHA-256 can differ between machines. Make the file you distribute once and publish its SHA-256 with it.

## Versions and updates

- Raise `version` for each release. When an administrator uploads a file with the same id, it becomes an update.
- Do not change the id. A different id installs as a different package, and users' shortcuts, widgets and saved values do not carry over.
- Changing `author.name` shows an "author is different" warning on the review screen. Older versions and different files with the same version number are warned about too.
- Widening the permissions (OMV methods, administrator-only and so on) is marked "New" on the review screen. Mention it in your change log too.
- Narrowing a widget's size range fits users' placed widgets into the new range.
- Raising the `sdk` level keeps the package from installing on dashboards with a lower level. Raise it only when you need a new feature.

## Distribution

- Publish the `.maru-addon` file and its SHA-256 together somewhere it can be downloaded (GitHub releases and so on). The dashboard has no package catalog, signatures or automatic updates.
- In the README, list the OMV methods the package reads and why, the plugins it needs and the OMV versions it supports. Administrators compare them with the review screen.
- For a package with a backend, also explain what the backend does, which OMV settings its actions change, why it needs `lan` or `loopback` if it asks for them, and what it keeps in its data directory. Mention that installed dashboards run backends only from a later version on.
- Any license is fine. Keep the notice of code you copied from the SDK (MIT).
