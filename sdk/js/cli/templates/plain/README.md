# @@NAME@@

A Maru NAS addon package.

- `manifest.json`: id, name, views and the permissions the addon asks for
- `ui/`: the window and widget pages (plain HTML and JavaScript, no build step)
- `i18n/`: translations (`en.json` is required)

To try it, run `MARU_DEV_ADDONS=/absolute/path/to/this/directory make dev-fake` from the root of a Maru NAS
repository checkout. The dashboard loads the directory as a development package and picks up changes when you
reload the page. Pack it with `maru-addon pack .` and install the `.maru-addon` file from Settings → Addons.
