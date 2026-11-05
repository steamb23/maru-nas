# @@NAME@@

A Maru NAS addon package with a backend written in Go.

- `addon/`: the package directory: `manifest.json` (id, name, views, the OMV methods, actions and the backend it
  declares), `ui/` (the window, plain HTML and JavaScript, no build step), `i18n/` (`en.json` is required) and, once
  built, `backend/linux-amd64` and `backend/linux-arm64`
- `backend/`: the backend's Go module (`main.go`, using the Maru NAS Go SDK)

@@GO_SETUP@@

## Try it

From the root of a Maru NAS repository checkout, run
`MARU_DEV_ADDONS=/absolute/path/to/this/directory/addon make dev-fake`. The dashboard loads `addon/` as a development
package and opens a host socket for its backend. Then start the backend:

```sh
cd backend
MARU_ADDON_SOCKET=<state>/addon-dev/@@ID@@.sock \
MARU_ADDON_HOST=<state>/addon-host/@@ID@@.sock \
go run .
```

`<state>` is the development dashboard's state directory (`MARU_STATE_DIR`; for `make dev-fake` it is
`.dev-state/fake` in the main checkout). The dashboard picks up changes to `addon/` when you reload the page; restart
the backend after changing `main.go`.

## Build and pack

Build static executables for both architectures, then pack the package:

```sh
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ../addon/backend/linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../addon/backend/linux-arm64 .
cd ..
maru-addon pack addon
```

`maru-addon pack` refuses a package that declares a backend but has no executable. Installed packages' backends run
only on installations where an administrator turned addon backends on (`./install.sh --addon-backends`); elsewhere a
package with a backend installs from Settings → Addons but its app is shown as unavailable.
