# Manifest

`manifest.json` at the package root declares the package's id, name, views and permissions. A package with an unknown field is not installed. For editor completion, point `$schema` at the SDK's JSON Schema ([`sdk/js/schema/manifest.schema.json`](https://github.com/steamb23/maru-nas/blob/main/sdk/js/schema/manifest.schema.json)). The schema covers only the structure; conditions between fields are checked by the dashboard when it installs the package.

```json
{
  "$schema": "https://raw.githubusercontent.com/steamb23/maru-nas/main/sdk/js/schema/manifest.schema.json",
  "format": 1,
  "id": "hello-widget",
  "version": "0.1.0",
  "sdk": 1,
  "name": { "en": "Hello widget", "ko": "안녕 위젯" },
  "description": { "en": "Shows the host name." },
  "author": { "name": "Example Author", "url": "https://example.com" },
  "icon": "icon.svg",
  "views": {
    "window": { "entry": "window.html", "defaultSize": { "width": 460, "height": 400 } },
    "widget": { "entry": "widget.html", "defaultSize": { "cols": 2, "rows": 1 } }
  },
  "rpc": { "read": ["System.getInformation"] }
}
```

## Fields

| Field | Required | Contents |
|---|---|---|
| `$schema` | | The schema address for editors. The dashboard ignores it |
| `format` | Yes | `1` |
| `id` | Yes | Up to 24 characters: a lowercase letter, then lowercase letters and digits joined by `-` (for example `hello-widget`). It must not be the id of a built-in addon, another package or an id the dashboard reserves (`files`, `settings`, `maru` and others). Do not change it after release (it becomes a different package) |
| `version` | Yes | SemVer (`1.2.3`, `1.2.3-beta.1`) without build metadata (`+`), up to 32 characters |
| `sdk` | Yes | The SDK level the package needs, `1` for now. If it is higher than the dashboard supports, the package is incompatible |
| `name` | Yes | Names per language (up to 40 characters). `en` is required |
| `description` | | Descriptions per language (up to 200 characters). If present, `en` is required |
| `author` | Yes | `name` (up to 60 characters) and an optional `url` (`http` or `https`). If the name changes in an update with the same id, the review screen warns |
| `icon` | Yes | `"icon.svg"`, an SVG file at the package root |
| `views` | Yes | At least one of a window (`window`) and a widget (`widget`). See "Views" below |
| `requires` | | `admin` (`true` shows the package to administrators only) and `plugins` (OMV plugins it needs) |
| `rpc` | | `read` (OMV read methods to call) and `write` (write methods its actions call), up to 64 in total |
| `actions` | | Action declarations (up to 32). They need a backend |
| `backend` | | The backend declaration (`network`). This version does not install packages with a backend |

## Texts

- `name`, `description` and an action's `label` and `confirmText` are objects keyed by language code. Use only the languages the dashboard supports (`en`, `ko`, `de`, `fr`, `es`, `it`, `pt-PT`, `nl`, `pl`, `cs`, `ru`, `uk`, `ja`, `zh-Hans`, `zh-Hant`). When there is no text for the screen language, `en` is shown.
- Texts cannot be blank or contain control or format characters (including direction overrides), `{{`, `}}` or `$t(` (so they do not mix with the dashboard's translation syntax).
- Texts inside your screens go in translation files (`i18n/<language>.json`), not in the manifest ([Building screens](addon-frontend.md#translations)).

## Views

| Field | Contents |
|---|---|
| `window.entry` | The window's HTML file, an `.html` file name directly under `ui/` (for example `window.html`) |
| `window.defaultSize` | The size the window opens with, `{width, height}` (pixels, 1 to 4096) |
| `window.minSize` | The smallest size it can be resized to. Must not be larger than the default size |
| `widget.entry` | The widget's HTML file |
| `widget.defaultSize` | The widget size `{cols, rows}` (desktop cells, 1 to 12) |
| `widget.minSize`, `widget.maxSize` | The resize range. Without them the size is fixed. The minimum must not exceed the default and the maximum must not be below it |

## Plugins

`requires.plugins` lists OMV plugins like `[{ "package": "openmediavault-nut", "rpcFile": "nut.inc" }]`: the plugin's Debian package name and the RPC file it installs (`/usr/share/openmediavault/engined/rpc/<file>`). A plugin counts as installed only when both are present. For plugins the dashboard already knows, `rpcFile` must match the dashboard's value. Without the plugin, the app and widget show as unavailable.

## OMV methods

- Screens can call (`app.rpc`) only the methods in `rpc.read`. The format is `"Service.method"`, and duplicates are rejected.
- Check OMV service and method names and what they return in the OMV sources (`/usr/share/openmediavault/engined/rpc/*.inc`). Prefer methods that exist in every OMV 8 version (8.0 and later).
- Some methods are not allowed for addon packages, such as running commands or scripts, installing packages and reading credentials ([Addon security](addon-security.md#methods-that-are-not-allowed)). A package that declares one is not installed.
- Declaring as a write a method another addon declares as a read (or the other way around) makes the package "Not allowed".
- `rpc.write` can only be used with actions, and actions need a backend. This version does not install backends, so write methods cannot be used yet either.

## Actions and backends

These fields are for a later version. For now only their format is checked, and packages with a backend are not installed.

| Field | Contents |
|---|---|
| `actions[].name` | The action name (same format as `id`, up to 32 characters) |
| `actions[].admin` | Whether only administrators can run it |
| `actions[].confirm` | Whether the dashboard asks before running it. If `true`, `confirmText` is required |
| `actions[].timeoutSeconds` | The time for the whole action (1 to 600 seconds, 60 if omitted) |
| `actions[].label`, `confirmText` | Texts per language (up to 40 and 300 characters) |
| `backend.network` | Where the backend can connect: an empty array (no network), `"lan"`, `"loopback"` |
