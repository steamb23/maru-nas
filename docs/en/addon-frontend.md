# Building screens

A package's windows and widgets are HTML pages under `ui/`. The dashboard shows each page in a sandboxed iframe and puts the SDK runtime and the shared styles in `_maru/` next to it.

## Entry HTML

```html
<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <link rel="stylesheet" href="./_maru/maru.css" />
    <script src="./_maru/sdk.js"></script>
  </head>
  <body>
    <main class="addon-window">
      <h1 id="title"></h1>
    </main>
    <script type="module" src="./window.js"></script>
  </body>
</html>
```

- Load `./_maru/sdk.js` first, in `<head>`. It applies the theme as soon as it loads, so dark mode does not flash white.
- Keep scripts in files. Inline scripts and `eval` are blocked.
- Use relative paths (`./`) everywhere. The dashboard serves the files under an address that differs per session.
- `ui/_maru` is a name the dashboard uses and cannot be part of a package.

## Connecting and context

```js
const app = await maru.connect()
document.getElementById('title').textContent = app.t('title')
app.on('context', (ctx) => render(ctx))
```

`maru.connect()` connects to the dashboard and returns the app object. Calling it again returns the same object. Opened outside the dashboard, it fails with `bridge_closed`.

`app.context` holds the values below, and a `'context'` event arrives whenever they change.

| Value | Contents |
|---|---|
| `view` | `'window'` or `'widget'` |
| `active` | Whether the view should draw. `false` while the window is minimized or the browser tab is hidden; stop periodic requests then |
| `size` | The frame size (`width`, `height`, and `cols`, `rows` for widgets) |
| `theme`, `animations` | `'light'` or `'dark'`, and whether animations are on |
| `language`, `messages` | The screen language (for example `ko`, `zh-Hans`) and the package's translation table |
| `admin` | Whether the signed-in user is an OMV administrator |
| `addonId`, `sdkLevel` | The package id and the dashboard's SDK level |

## API

| Function | Contents |
|---|---|
| `app.t(key, params)` | A text from the package's translations. Replaces `{{name}}` with `params`. Returns the key for missing keys |
| `app.rpc(service, method, params)` | Calls an OMV method declared in the manifest's `rpc.read` and returns the result |
| `app.storage.get()`, `app.storage.set(data)` | This user's saved value for the package (JSON, up to 64 KiB), `null` if none. `set` at most once per second |
| `app.confirm({ message, title, confirmLabel, danger })` | The dashboard's confirmation dialog, `true` if confirmed. Its title is "Request from the <package name> addon", and `title` goes in front of the message |
| `app.notify(level, key, params)` | A dashboard notification (toast and notification center). `level` is `info`, `success`, `warning` or `error`, and the text is a translation key. Up to 3 per minute. Clicking it opens the package window |
| `app.openWorkbench(route)` | Opens an OMV workbench route in the named tab (no leading slash, for example `storage/smart/devices`) |
| `app.data(query)` | Data from the package backend ([Addon backends](addon-backend.md)). `query` is an object of strings (up to 32 keys, 512 characters each) that the backend receives as the query string. Resolves with the backend's JSON (`null` if there is none) |
| `app.action(name, { target, input })` | Runs an action declared in the manifest's `actions` in the backend. Accepted only right after the user pressed the frame (below), and the dashboard asks first if the action has `confirm`. `target` is a string (up to 1024 characters) and `input` a JSON object (up to 8 KiB). Resolves with nothing |
| `app.query(name, { target, input })` | Runs an action like `app.action` and resolves with the backend's result. It does not need a user action, so a window can use it to read when it opens |

- Each frame can have 16 requests waiting at once, and a message can be up to 256 KiB. Other requests are accepted up to 50 per second.
- `action`, `query`, `confirm` and `notify` work only in window views.
- Data requests wait up to 15 seconds and actions up to their `timeoutSeconds`. On installations without addon backends turned on, a package with a backend shows as unavailable.

## Windows and widgets

- A widget is a small view attached to desktop cells. It cannot be clicked or reached with Tab; clicking it opens the package window (if there is a window view). The dashboard handles dragging and resizing.
- Widgets can use only `rpc`, `storage.get` and `data`. A widget's background is transparent so the widget frame shows through.
- When the window is minimized or the tab is hidden, `context.active` becomes `false`. Stop periodic requests in both windows and widgets.

## Requests that need a user action

`action` and `openWorkbench` are accepted only right after the user pressed inside that frame. Call them directly in a button's `click` handler. `query` and `data` do not need a user action.

- It must be within 5 seconds of a press inside the frame or an Enter or Space key.
- The frame must have focus, its window must be in front and not blocked by a dialog.
- At most once per second.
- Calls on page load or from a timer are rejected with `bridge_gesture_required`.

## Translations

- Put screen texts in `i18n/<language>.json`: flat JSON with keys and string values. `en.json` is required.
- Keys are up to 100 characters of letters, digits, `_`, `.` and `-`; values up to 2000 bytes; up to 2000 keys per file.
- The dashboard gives the texts of the screen language on top of `en` as `context.messages`. When the user changes the language a `'context'` event arrives, so draw the texts again.
- Placeholders are `{{name}}` (`app.t('host', { name })`). There are no plural forms.

## Look

- `./_maru/maru.css` holds the dashboard's design tokens and shared classes, so packages look like the built-in apps.
- Tokens: `--text`, `--text-muted`, `--accent`, `--window-bg`, `--window-border`, `--hover-bg`. The theme is `<html data-theme="light|dark">`, `data-animations="off"` when animations are off, and the view is `data-view="window|widget"`.
- Classes: `.addon-window` (the window's centered column), `.addon-card`, `.addon-card-head`, `.addon-card-title`, `.addon-card-value`, `.addon-card-sub`, `.addon-section-title`, `.addon-hint`, `.addon-list`, `.addon-banner`, `.addon-empty`, `.addon-button` (`.primary`, `.danger`), `.badge`, `.meter` and `.meter-fill`, `.status-dot`, `.addon-widget`, `.widget-line`, `.widget-name`, `.widget-value`
- Inline `style` attributes and `<style>` elements are allowed.

## Frame restrictions

Package screens run in a sandbox without an origin (an opaque origin), and every response carries the dashboard's Content-Security-Policy.

- They cannot send network requests (`fetch`, XHR, WebSocket, EventSource). Call OMV with `app.rpc`.
- Scripts, images and fonts come only from the package's files. Images and fonts can also be `data:` and `blob:`. Files from other sites (CDNs and so on) are blocked.
- `document.cookie`, `localStorage`, `sessionStorage` and IndexedDB throw `SecurityError`. Keep per-user values in `app.storage`.
- Forms are not submitted, and no `submit` event fires. Instead of a `<form>`, handle the button's `click` and the input's Enter key yourself.
- `alert`, `confirm`, `prompt`, popups (`window.open`), sending the dashboard page itself to another address, and browser features that need permissions such as the clipboard are not available. Confirm with `app.confirm` and open the workbench with `app.openWorkbench`.
- If the frame navigates to another address, the dashboard drops the connection and loads the package screen again from the start.

## Errors

Failed requests reject with a `MaruError` (`status`, `code`, `addonCode`, `message`).

| `code` | Meaning |
|---|---|
| `bridge_not_allowed` | The request is not available in this view (`confirm` in a widget, for example) |
| `bridge_gesture_required` | A request that needs a user action was made without one |
| `bridge_rate_limited`, `bridge_busy` | Too frequent or too many requests |
| `bridge_invalid` | A malformed request |
| `bridge_cancelled` | The user cancelled the dashboard's confirmation dialog |
| `bridge_timeout`, `bridge_closed` | The dashboard did not answer, or the connection was lost |
| `rpc_forbidden` | The method is not declared in the manifest or is not allowed |
| `omv_error`, `omv_unavailable` | OMV returned an error, or OMV could not be reached |
| `addon_error` | The package backend answered with an error (below) |
| `addon_no_backend` | `data` in a package without a backend |
| `addon_backend_unavailable` | The backend is not running, did not answer a data request within 15 seconds, has not been turned on by the backend sync yet after an install or update, or the dashboard is handling too many backend requests at once |
| `addon_backend_disabled`, `backend_unavailable` | Addon backends are turned off on this installation, or this package's backend could not be turned on |
| `action_timeout` | The action did not finish within its `timeoutSeconds` |
| `action_busy` | Another run of the same action with the same target is in progress |
| `admin_required` | The action is administrator-only and the user is not an administrator |
| `security_required` | The user did not complete the dashboard's identity check for an action of a package that declares OMV writes |

OMV returns an error when a regular user calls an administrator-only method. Check `context.admin` before calling one.

For `addon_error`, `message` is the backend's own message, with control and bidirectional control characters removed and cut to 300 characters, and `addonCode` is the backend's error code (such as `config_dirty`). Show the message or pick your own text by `addonCode`. When the backend's answer was not valid (a crash, an unexpected status), there is no `addonCode` and the message is a short English description from the dashboard. For every other code, `message` stays a generic description of the failed request, so decide what to show by `code`.

If an action needs the identity check, the dashboard shows its own dialog and runs the action again, so `security_required` arrives only when the user cancels it.

## TypeScript and Vite

`maru-addon init <directory> --ts` creates a TypeScript + Vite project. `addon/` is the package directory, and `npm run build` writes `addon/ui/`.

- The types are in `@maru-nas/addon-sdk` (`"types": ["@maru-nas/addon-sdk"]` in `tsconfig.json`), including the global `maru` and types such as `MaruApp`.
- `vite.config.ts`: `base: './'` (relative paths), `build.outDir: 'addon/ui'`, `build.assetsInlineLimit: 0` (keep assets as files), one HTML page per view (`rollupOptions.input`)
- Add the `vite-ignore` attribute to the `_maru/` references in the HTML so Vite does not bundle them.
- During development, rebuild with `npm run watch` (`vite build --watch`) and pass the `addon/` directory to `MARU_DEV_ADDONS`. The Vite dev server (HMR) cannot be used.
