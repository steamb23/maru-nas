# @@NAME@@

A Maru NAS addon package written in TypeScript and built with Vite.

- `addon/`: the package directory (`manifest.json`, `icon.svg`, `i18n/`). `npm run build` writes `addon/ui/`
- `window.html`, `src/`: the window view

Build with `npm install` and `npm run build`. To try it, run `npm run watch` here and
`MARU_DEV_ADDONS=/absolute/path/to/this/directory/addon make dev-fake` from the root of a Maru NAS repository
checkout; the dashboard picks up changes when you reload the page. `npm run pack` makes the `.maru-addon` file to
install from Settings → Addons.
