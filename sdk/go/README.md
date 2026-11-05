# maruaddon: Go SDK for addon backends

Go SDK for the backends of [Maru NAS](https://github.com/steamb23/maru-nas) addon packages (`.maru-addon`). MIT
licensed ([LICENSE](https://github.com/steamb23/maru-nas/blob/main/sdk/go/LICENSE)); the dashboard itself is GPL-3.0.
The module uses only the Go standard library and builds with Go 1.23 or later.

An addon package may ship a backend: a static Linux executable (`backend/linux-amd64`, `backend/linux-arm64`) that
systemd runs as a dynamic user once the package is installed. The dashboard (maru) forwards the frontend's data
requests and actions to it, and the backend can call the OMV methods its manifest declares through maru. This
package implements the backend side of that protocol: the server, the request headers, the response and error
format, and the client for maru's host socket.

The full protocol and the limits backends run under are described in the documentation:
[Addon backends](https://github.com/steamb23/maru-nas/blob/main/docs/en/addon-backend.md).

## Example

The manifest declares the backend, the OMV method it reads and the action:

```json
{
  "backend": { "network": [] },
  "rpc": { "read": ["Network.getGeneralSettings"] },
  "actions": [{ "name": "hostname", "admin": true, "label": { "en": "Host name" } }]
}
```

```go
package main

import (
	"context"
	"log"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

func main() {
	err := maruaddon.Serve(maruaddon.Handler{
		// The frontend's data requests: GET /v1/data?name=...
		Data: func(ctx context.Context, r *maruaddon.Request) (any, error) {
			name := r.Query.Get("name")
			if name == "" {
				name = r.User
			}
			return map[string]string{"greeting": "Hello, " + name}, nil
		},
		Actions: map[string]maruaddon.HandlerFunc{
			// The frontend's action "hostname" with input {"short": true}
			"hostname": func(ctx context.Context, r *maruaddon.Request) (any, error) {
				var in struct {
					Short bool `json:"short"`
				}
				if err := r.DecodeInput(&in); err != nil {
					return nil, err // 400 bad_input
				}
				var dns struct {
					Hostname   string `json:"hostname"`
					Domainname string `json:"domainname"`
				}
				if err := r.Host.RPCInto(ctx, "Network", "getGeneralSettings", nil, &dns); err != nil {
					return nil, err // 500, the error goes to the journal
				}
				if in.Short || dns.Domainname == "" {
					return map[string]string{"hostname": dns.Hostname}, nil
				}
				return map[string]string{"hostname": dns.Hostname + "." + dns.Domainname}, nil
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

Build static executables for the package:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o addon/backend/linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o addon/backend/linux-arm64 .
```

## Protocol summary

maru talks to the backend over HTTP/1.1 on a Unix socket.

| Request | Handler |
|---|---|
| `GET /v1/data?<query>` | `Handler.Data`. `Request.Query` is the query string the frontend sent |
| `POST /v1/actions/{name}` with `{"target": string, "input": object or null}` | `Handler.Actions[name]`. `Request.Target`, `Request.Input`, `Request.DecodeInput` |

Every request has `X-Maru-User` (`Request.User`), `X-Maru-Admin` (`Request.Admin`), `X-Maru-Lang`
(`Request.Lang`) and `X-Maru-Call`, the call token `Request.Host` uses.

maru checks the browser's request before it reaches the backend: the data query string is at most 2 KiB, the
browser's action body at most 16 KiB and `input` a JSON object of at most 8 KiB. The SDK itself refuses request
bodies over 64 KiB (400 `bad_request`) as a guard of its own.

| Handler result | Response |
|---|---|
| data `(v, nil)` | 200, `v` as JSON (`nil` is `null`) |
| action `(nil, nil)` | 204 |
| action `(v, nil)` | 200, `v` as JSON |
| `*maruaddon.Error` with status 400, 404, 409, 422 or 429 and a code matching `^[a-z][a-z0-9_]{0,31}$` | that status, `{"code", "message"}`; maru passes the code to the frontend |
| any other error, a panic, or a result larger than 4 MiB | 500 `{"code":"internal","message":"internal error"}`; the error goes to `Handler.ErrorLog` (standard error) |

The SDK itself answers 400 `bad_request` (malformed request), 404 `no_data`, `unknown_action` or `not_found`, and
405 `method_not_allowed`. `Request.DecodeInput` is strict (unknown fields are an error) and returns a 400
`bad_input` error the handler can return as it is.

`Request.Host` calls OMV through maru's host socket while the handler runs:

| Method | Host endpoint |
|---|---|
| `RPC`, `RPCInto` | `POST /v1/rpc`: a declared read method, or during an action a declared write method |
| `RPCBg` | `POST /v1/rpc-bg`: a declared method that runs in the background, waiting until it ends (administrators only) |
| `Apply` | `POST /v1/apply`: up to 16 declared write calls, then OMV's "apply changes" (actions only) |

Errors from maru are `*maruaddon.HostError` values; `maruaddon.IsCode(err, "config_dirty")` checks the code. The call
token is valid only during the request, so a backend cannot call OMV on its own, and the SDK never logs it.

`maruaddon.StateDir()` is the backend's private data directory (`STATE_DIRECTORY` from systemd), shared by all
dashboard users.

## Development

Run a development dashboard (`make dev` or `make dev-fake` in a Maru NAS checkout, which turn on development
backends) with the unpacked package directory in `MARU_DEV_ADDONS`. maru then opens a host socket for the package
under its state directory and connects to the backend's socket there. Start the backend with:

```sh
MARU_ADDON_SOCKET=<state>/addon-dev/<id>.sock \
MARU_ADDON_HOST=<state>/addon-host/<id>.sock \
MARU_ADDON_STATE=/tmp/<id>-data \
go run .
```

- `MARU_ADDON_SOCKET`: the socket the backend listens on. A leftover socket file nobody listens on is replaced; the
  socket is made mode 0600 and removed on exit. Unix socket paths are limited to 107 bytes
- `MARU_ADDON_HOST`: maru's host socket for this package (installed backends use `/run/maru-host/host.sock`)
- `MARU_ADDON_STATE`: optional, what `StateDir()` returns

Installed backends (on installations where an administrator turned addon backends on with
`./install.sh --addon-backends`) are started by systemd socket activation (`LISTEN_FDS`, file descriptor 3) and need no
variables; `StateDir()` returns systemd's `STATE_DIRECTORY` and the log goes to the systemd journal. `Serve` stops gracefully on SIGTERM or SIGINT: it stops accepting connections and waits up to
60 seconds for running requests, which keep their context until then. If requests are still running after that, it
closes their connections (their context ends) and returns an error. A second SIGTERM or SIGINT during the wait (a
second Ctrl+C) ends the process at once.

For tests or custom setups, `ServeListener` serves on a listener you made, and `ServeContext` stops when a context
ends instead of on a signal.

## Examples

- [`examples/hello-backend`](https://github.com/steamb23/maru-nas/tree/main/sdk/go/examples/hello-backend): the
  backend of the example package
  [`sdk/examples/hello-backend`](https://github.com/steamb23/maru-nas/tree/main/sdk/examples/hello-backend). Data read
  through `Host.RPCInto`, a value kept in `StateDir()`, and an administrator action that changes an OMV setting with
  `Host.Apply`
- [`examples/probe`](https://github.com/steamb23/maru-nas/tree/main/sdk/go/examples/probe): a test tool, not an
  example, that checks what an installed backend's sandbox blocks (`sdk/testdata/probe`)

In a Maru NAS checkout, `make sdk-examples` builds both into their packages' `backend/` directories, and
`FAKEADDONS=examples make dev-fake` starts the example backends with `go run`. `maru-addon init <dir> --backend`
creates a new package with a backend module to start from.

## Using the module

The module is not published yet. Until the first public release, use it from a checkout of the Maru NAS repository
with a `replace` directive in your backend's `go.mod`:

```
require github.com/steamb23/maru-nas/sdk/go v0.0.0

replace github.com/steamb23/maru-nas/sdk/go => ../maru-nas/sdk/go
```
