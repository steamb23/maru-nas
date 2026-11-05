# Addon backends

A package can include a backend: a program that runs on the server next to the dashboard. The package's screens call it with `app.data()`, `app.action()` and `app.query()`, and the backend can call the OMV methods the manifest declares through the dashboard. Use a backend when a package needs OMV writes (actions), combines several OMV calls into one answer, keeps data on the server or reads system files such as `/proc`.

> [!NOTE]
> A dashboard installed with `install.sh` runs backends only when an administrator has turned addon backends on (`./install.sh --addon-backends`). Other installations accept packages with a backend, but their apps and widgets show as unavailable ([Installed backends](#installed-backends)).

## How it works

```
package screen   app.data(), app.action(), app.query()
  → dashboard    checks the session, the manifest and the user's rights
    → backend    GET /v1/data, POST /v1/actions/<name>
      → dashboard host socket   POST /v1/rpc, /v1/rpc-bg, /v1/apply
        → OMV    with the signed-in user's OMV session
```

- The dashboard talks to the backend over HTTP/1.1 on a Unix socket. Browsers never connect to the backend.
- Each request tells the backend who the signed-in user is, whether they are an OMV administrator and which screen language they use, and carries a call token for that request.
- The backend calls OMV by sending the call token to the dashboard's host socket. The dashboard makes the call with the signed-in user's OMV session, so the backend never sees the OMV session ID or the password and cannot go beyond that user's OMV permissions.
- Only the methods in the manifest's `rpc.read` (data requests and actions) and `rpc.write` (actions only) can be called. Methods that are not allowed for packages ([Addon security](addon-security.md#methods-that-are-not-allowed)) cannot be declared at all.
- One backend process serves every user of the dashboard.

## Declaring a backend

```json
{
  "rpc": {
    "read": ["System.getInformation", "PerfStats.get"],
    "write": ["PerfStats.set"]
  },
  "actions": [
    {
      "name": "perfstats",
      "admin": true,
      "confirm": true,
      "timeoutSeconds": 300,
      "label": { "en": "Change statistics" },
      "confirmText": { "en": "Change OMV's performance statistics setting and apply it now?" }
    }
  ],
  "backend": { "network": [] }
}
```

- `backend` marks the package as having a backend. `network` says where an installed backend may connect from inside its sandbox; an empty array means no network ([Sandbox](#sandbox)).
- `actions` and `rpc.write` need a backend. The fields are described in [Manifest](addon-manifest.md#actions-and-backends).
- Put the executables in the package as `backend/linux-amd64` and `backend/linux-arm64` ([Packing and distribution](addon-packaging.md#files)).

## Go SDK

The Go SDK (module `github.com/steamb23/maru-nas/sdk/go`, package `maruaddon`) implements the backend side of the protocol: the server, the request headers, the response and error format, and the client for the host socket. It is MIT licensed, uses only the Go standard library and builds with Go 1.23 or later. The full API is in the [SDK README](https://github.com/steamb23/maru-nas/blob/main/sdk/go/README.md).

The module is not published yet. Until the first public release, use it from a checkout of the Maru NAS repository with a `replace` directive in the backend's `go.mod`. `maru-addon init <directory> --backend` writes that `go.mod` for you ([Development](#development)).

### A minimal backend

```go
package main

import (
	"context"
	"log"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

func main() {
	err := maruaddon.Serve(maruaddon.Handler{
		// app.data() in the screen: GET /v1/data
		Data: func(ctx context.Context, r *maruaddon.Request) (any, error) {
			var info struct {
				Hostname string `json:"hostname"`
			}
			// Only methods declared in the manifest's rpc.read can be called.
			if err := r.Host.RPCInto(ctx, "System", "getInformation", nil, &info); err != nil {
				return nil, err
			}
			return map[string]string{"hostname": info.Hostname, "user": r.User}, nil
		},
		// app.action('reset') in the screen: POST /v1/actions/reset
		Actions: map[string]maruaddon.HandlerFunc{
			"reset": func(ctx context.Context, r *maruaddon.Request) (any, error) {
				return nil, nil // 204, no result
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

`Serve` listens on the socket the environment names, answers requests until it receives SIGTERM or SIGINT, and then waits up to 60 seconds for running requests.

### Building

Build a static executable for each architecture into the package's `backend/` directory:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ../addon/backend/linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o ../addon/backend/linux-arm64 .
```

`CGO_ENABLED=0` makes the executable independent of the libraries installed on the NAS.

### Data and actions

`Handler.Data` answers data requests and `Handler.Actions` answers actions by name. A handler gets a `*maruaddon.Request`:

| Field | Contents |
|---|---|
| `User` | The signed-in user's OMV user name |
| `Admin` | Whether the user is an OMV administrator |
| `Lang` | The dashboard's screen language (`en`, `ko`, `zh-Hans` and so on, `en` for languages the dashboard does not support) |
| `Query` | The query of a data request (`app.data({ name: 'x' })` becomes `name=x`) |
| `Action`, `Target`, `Input` | The action's name, its target string and its input object as raw JSON. `r.DecodeInput(&v)` decodes the input strictly (unknown fields are an error) |
| `Host` | Calls OMV on behalf of this request (below). Valid only while the handler runs |

| Handler result | What the screen gets |
|---|---|
| Data `(v, nil)` | `v` as JSON (`nil` becomes `null`) |
| Action `(nil, nil)` | Nothing (`app.action()` resolves) |
| Action `(v, nil)` | `v` as JSON (`app.query()` resolves with it) |
| `maruaddon.NewError(status, code, message)` with status 400, 404, 409, 422 or 429 | A `MaruError` with that status, `code` `addon_error`, `addonCode` your code and `message` your message ([Building screens](addon-frontend.md#errors)) |
| Any other error or a panic | A `MaruError` with `code` `addon_error` and no `addonCode`. The error goes to standard error |

The code must match `^[a-z][a-z0-9_]{0,31}$`. The dashboard does not translate the message; write it in English or pick the text with `r.Lang`.

### Calling OMV

`r.Host` calls OMV through the dashboard while the request runs:

| Method | Use |
|---|---|
| `RPC(ctx, service, method, params)` | Calls a declared method and returns the result as raw JSON |
| `RPCInto(ctx, service, method, params, &out)` | The same, decoding the result into `out` (fields `out` does not have are ignored) |
| `RPCBg(ctx, service, method, params, output)` | Calls a declared method that runs in the background in OMV (one that returns a background status file name) and waits until it ends. With `output` true it returns the job's output (up to 1 MiB). Administrator sessions only |
| `Apply(ctx, calls...)` | Runs up to 16 declared write calls (`maruaddon.Call{Service, Method, Params}`) and then applies the configuration changes, waiting until they are applied. Actions only |

- During a data request only `rpc.read` methods can be called. During an action, `rpc.write` methods can be called too.
- Use `Apply` for methods that change OMV's configuration. A write called with `RPC` only stores the change in OMV's database and leaves it pending, so the setting does not take effect until someone applies it in the workbench.
- If OMV already has pending changes, `Apply` refuses with the code `config_dirty`, so the package never deploys changes someone else made. If one call fails, nothing is applied and the changes of the earlier calls stay pending.
- OMV applies changes only for administrator sessions, so make actions that call `Apply` administrator-only (`"admin": true`).
- Errors from the dashboard are `*maruaddon.HostError` values; check them with `maruaddon.IsCode(err, "config_dirty")` ([Host socket errors](#host-socket-errors)). Returning one as it is answers 500, which the screen sees as `addon_error` without `addonCode`. To show a reason in the screen, return your own error:

```go
// http is net/http.
_, err := r.Host.Apply(ctx, maruaddon.Call{Service: "PerfStats", Method: "set", Params: map[string]bool{"enable": in.Enable}})
if maruaddon.IsCode(err, "config_dirty") {
	return nil, maruaddon.NewError(http.StatusConflict, "config_dirty",
		"OMV has pending configuration changes; apply or revert them in the OMV workbench first")
}
if err != nil {
	return nil, err
}
```

If the user's OMV session has ended, the host answers `unauthorized` and the dashboard signs the user out, whatever the backend answers.

### Data directory

`maruaddon.StateDir()` is the backend's own data directory, or an empty string if there is none. Every user shares it, it survives updates, and an administrator can delete it when removing the package. A package that was removed and installed again starts with an empty directory ([Data and removal](#data-and-removal)). Keep files small and write them atomically (a temporary file, then rename).

## Protocol for other languages

Backends do not have to be written in Go. A backend in another language implements the requests below. They are the contract the Go SDK follows.

### Requests from the dashboard

HTTP/1.1 on a Unix socket, with `Host: addon`.

| Request | Contents |
|---|---|
| `GET /v1/data?<query>` | A data request. The query string is what the screen passed to `app.data()`, at most 2 KiB of printable ASCII |
| `POST /v1/actions/<name>` | An action, `Content-Type: application/json`, body `{"target": string, "input": object or null}`. `input` is a JSON object of at most 8 KiB |

Every request has these headers:

| Header | Contents |
|---|---|
| `X-Maru-User` | The signed-in user's OMV user name |
| `X-Maru-Admin` | `1` for OMV administrators, `0` otherwise |
| `X-Maru-Lang` | The dashboard's screen language, one the dashboard supports, otherwise `en` |
| `X-Maru-Call` | The call token for the host socket. Do not log it |

Answer unknown paths and methods with a JSON error body too (for example 404 `{"code": "not_found", "message": "not found"}`); a plain-text 404 counts as an invalid answer.

### Responses

| Backend answer | What the screen gets |
|---|---|
| 200 with a JSON value, up to 4 MiB | The value (the data, or the result of `app.query()`) |
| 204 | Nothing for an action, `null` for a data request |
| 400, 404, 409, 422 or 429 with a JSON object `{"code": string, "message": string}` | A `MaruError` with the same status, `code` `addon_error`, `addonCode` the backend's code if it matches `^[a-z][a-z0-9_]{0,31}$`, and `message` the backend's message with control and bidirectional control characters removed, cut to 300 characters (`addon backend error` if empty) |
| Any other status, one of the statuses above without a JSON object body, an answer over 4 MiB, invalid JSON or a redirect | 502 `addon_error` without `addonCode` |

The dashboard does not pass the backend's response headers to the browser, and it never passes a backend's 401, 403 or 5xx through as it is. A backend cannot sign users out or imitate the dashboard's own error codes.

### Time limits

- Connecting to the backend: 2 seconds. If the backend is not running, data requests and actions fail with 503 `addon_backend_unavailable`.
- Data requests: 15 seconds, then 504 `addon_backend_unavailable`.
- Actions: the action's `timeoutSeconds` (60 seconds if omitted, 600 at most), then 504 `action_timeout`. An action keeps running until it finishes or its time limit ends, even if the user closes the window. Only one action runs at a time for each package and target (409 `action_busy`).

### The host socket

HTTP/1.1 on a Unix socket, with `Host: maru`. Installed backends reach it at `/run/maru-host/host.sock`; on the development server its path is in `MARU_ADDON_HOST`. Each backend sees only its own host socket.

Every request is a `POST` with `Content-Type: application/json` and `X-Maru-Call: <the token of the request being handled>`. Bodies are at most 64 KiB and are read strictly (unknown fields are an error).

| Path | Body | Answer |
|---|---|---|
| `/v1/rpc` | `{"service", "method", "params"}` | `{"result": <OMV result>}`. Declared read methods, and during an action declared write methods |
| `/v1/rpc-bg` | `{"service", "method", "params", "output": bool}` | `{"output": string, "truncated": bool}`. Administrator sessions only. The method's result must be a background status file name; the dashboard waits until the job ends and, if `output` is true, returns up to 1 MiB of its output |
| `/v1/apply` | `{"calls": [{"service", "method", "params"}, …]}`, 1 to 16 calls | `{"results": [<OMV result>, …]}`. Actions only, and every call must be a declared write method. The dashboard runs the calls, applies the configuration changes and waits until they are applied |

- The call token is valid only until the dashboard's request to the backend ends. It is a read token for data requests and a write token for actions, and the host rejects tokens of other packages.
- Limits: headers within 5 seconds and up to 8 KiB, the body within 5 seconds, idle connections closed after 30 seconds, 16 connections and 8 requests at once (more requests get 429 `busy`).

### Host socket errors

Errors are `{"error": message, "code": code}`.

| Status | Code | Meaning |
|---|---|---|
| 400 | `bad_request` | A malformed body, too large, or too many calls |
| 401 | `call_invalid` | The token is missing, unknown, expired or belongs to another package |
| 401 | `unauthorized` | The user's OMV session ended. The dashboard signs the user out and ends the request with 401, whatever the backend answers |
| 403 | `rpc_forbidden` | The method is not declared, or a write or `apply` was attempted during a data request |
| 403 | `admin_required` | `rpc-bg` from a session that is not an administrator's |
| 409 | `config_dirty` | OMV already has pending configuration changes (`apply`) |
| 429 | `busy` | Too many requests at once |
| 502 | `omv_error` | OMV returned an error. The message is OMV's message (up to 1 KiB) |
| 503 | `omv_unavailable` | OMV could not be reached |
| 504 | `timeout` | The request's time limit has passed |

### Process environment

| Item | Installed backends | Development server |
|---|---|---|
| Socket to listen on | Socket activation: `LISTEN_FDS=1`, `LISTEN_PID` is the backend's process ID, the socket is file descriptor 3 | `MARU_ADDON_SOCKET` names the socket path. Create it with mode 0600 and remove it on exit. Replace a leftover socket file only when connecting to it is refused (nobody listens); treat any other file, or a socket another process listens on, as an error |
| Host socket | `/run/maru-host/host.sock` | `MARU_ADDON_HOST` |
| Data directory | `STATE_DIRECTORY` | `MARU_ADDON_STATE`, optional |

- On SIGTERM, stop accepting connections, finish the running requests and exit.
- Write logs to standard error. Never log the `X-Maru-Call` header.
- On the development server, the backend's log is in the terminal you started it in (for the examples, the `make dev-fake` terminal). When a data request or action fails, the dashboard's log (the development server's terminal) also records the package id and the error code. Installed backends log to the systemd journal ([Logs](#logs)).

## Development

The development server (`make dev`, `make dev-fake`) turns on development backends: the development scripts set `MARU_DEV_ADDON_BACKENDS=1` (set it to `0` to see how a package behaves on an installation with backends turned off). It needs the state directory the development scripts set by default.

For each development package with a backend, the dashboard opens a host socket at `<state>/addon-host/<id>.sock` and connects to the backend at `<state>/addon-dev/<id>.sock`. `<state>` is the development state directory (`MARU_STATE_DIR`; by default `.dev-state/fake` for `make dev-fake` and `.dev-state/real` for `make dev` in the main checkout). The dashboard never starts the backend itself; you start it, and until you do, data requests and actions fail with `addon_backend_unavailable`. Development packages load without the executables in `backend/`.

### Running a backend

1. Create a package with a Go backend. This makes `addon/` (the package directory) and `backend/` (the backend's Go module, `go.mod` pointing at the checkout's SDK) side by side. Add `--ts` for a TypeScript + Vite screen.

    ```sh
    node sdk/js/cli/maru-addon.mjs init ~/addons/my-backend --backend
    ```

2. Start the development server with the package directory.

    ```sh
    MARU_DEV_ADDONS=$HOME/addons/my-backend/addon make dev-fake
    ```

3. In another terminal, start the backend with the development sockets.

    ```sh
    cd ~/addons/my-backend/backend
    MARU_ADDON_SOCKET=<state>/addon-dev/my-backend.sock \
    MARU_ADDON_HOST=<state>/addon-host/my-backend.sock \
    MARU_ADDON_STATE=/tmp/my-backend-data \
    go run .
    ```

4. Restart the backend after changing its code. Reload the page after changing `addon/`.
5. Build the executables ([Building](#building)) and pack `addon/` with `maru-addon pack`. Packing refuses a package that declares a backend but has no executable.

- Unix socket paths can be at most 107 bytes. If the state directory's path is long (for example when `MARU_STATE_DIR` points deep inside a git worktree), the SDK refuses to start; set a shorter `MARU_STATE_DIR`.
- The backend runs as your user on the development server, without the sandbox of installed backends. Check what it needs against [Sandbox](#sandbox).

### Examples and the fake OMV

- `FAKEADDONS=examples make dev-fake` loads the repository's examples and starts the backends of the examples that have one with `go run` (their data directories are under `<state>/addon-state/`). Packages you add with `MARU_DEV_ADDONS` are not started.
- [hello-backend](https://github.com/steamb23/maru-nas/tree/main/sdk/examples/hello-backend) (backend in [sdk/go/examples/hello-backend](https://github.com/steamb23/maru-nas/tree/main/sdk/go/examples/hello-backend)) shows data read with `Host.RPCInto`, values kept in the data directory, and an administrator action that changes an OMV setting with `Host.Apply` and turns `config_dirty` into an `addonCode`.
- `make sdk-examples` builds static executables of the example backends into their packages' `backend/` directories (they are not committed). The development server does not need them; they are for packing.
- The fake OMV of `make dev-fake` imitates only some methods. For other methods (plugin methods, for example), put fixed answers in `dev/fake-rpc.json` in the package directory. `make dev-fake` collects these files from the `MARU_DEV_ADDONS` directories, and you can also pass files yourself as a colon-separated list in `FAKEOMV_EXTRA_RPC`.

    ```json
    {
      "Example.getStatus": { "result": { "running": true }, "admin": true },
      "Example.getList": { "result": [] }
    }
    ```

    - `result` is required (any JSON value, `null` too) and is returned whatever the parameters are. With `"admin": true`, only administrator sessions get it.
    - It answers only methods the fake OMV does not implement, and the usual session checks still apply. `Session` methods and unknown fields are errors, and a file that cannot be read keeps the fake OMV from starting. If two files list the same method, the first one is used.
    - Check plugin methods against the real OMV with `make dev` before you publish.

## Constraints

- **Executables**: `backend/linux-amd64` and `backend/linux-arm64`, each a 64-bit Linux ELF for that architecture of up to 64 MiB. Build them statically. A package is installed only if it has the executable for the server's architecture ("The package has no backend for this server's architecture."), so ship both.
- **Time**: data requests get 15 seconds and actions their `timeoutSeconds`, at most 600 seconds. Work that takes longer cannot be done.
- **OMV only during a request**: the call token is valid only while the dashboard's request runs, so a backend cannot call OMV on its own (at start-up, from a timer or after the answer). Work that needs no OMV, such as reading `/proc` or its own files, it can do at any time.
- **One backend for all users**: every user's requests go to the same process, possibly at the same time. Decide what to return from each request's user and administrator flag, and do not keep values read with an administrator's session to return them to regular users. The data directory is shared too.
- **Screens are unchanged**: the screens still cannot reach the network. Only the backend can connect, and an installed backend only where `backend.network` allows.

## Installed backends

A dashboard installed with `install.sh` runs package backends only when an administrator has turned addon backends on. Other installations can still install packages with a backend, but the review screen warns about it and their apps and widgets show as unavailable.

### Turning them on and off

- An administrator runs `./install.sh --addon-backends` in the repository over SSH ([Installation](install.md#addon-backends), [Install options](install-options.md)). It is off by default and the install script does not ask. Once on, it stays on through later upgrades and `--reinstall`.
- To turn them off, run `./install.sh --no-addon-backends`. Every running backend stops. Backend data is not deleted, but if you turn backends on again, which installation that data belongs to can no longer be confirmed, so a package's data is deleted before that package's backend starts again.
- The install script restarts the dashboard, so every user signs in again.

### How backends are turned on

- The dashboard never runs a backend program itself. Whenever a package is installed, turned on or off, or removed, it writes the list of backends to keep on into its state directory, and a sync service running as root turns backends on and off to match that list. The sync service does not read package files; it treats the list as untrusted input and checks its format strictly.
- A backend that is on starts on its first request and is restarted if it fails. An update restarts it with the new version.
- After an install, update or reinstall, the dashboard sends the backend no requests until the sync result shows that this version of this installation is on. Until then data requests and actions fail with 503 `addon_backend_unavailable`, so a request never reaches a backend left over from an earlier installation or version of the same id.
- Up to 32 packages with a backend can be turned on. Development packages don't count.
- The last sync result is on the "Addons" page of the Settings app. A backend that could not be turned on gets a "Backend failed" badge and the reason in the package list, and its app shows as unavailable ([Addon packages](addons.md#addon-backends)).

### Sandbox

Each backend runs in a sandbox of its own:

- A system user of its own that exists only while the backend runs, without root or any other privilege.
- Its own package files, read-only, and its own data directory. It cannot reach the dashboard's state (users' settings, security records, other packages), OMV's configuration, shared folders and data disks (`/srv`, `/export`, `/mnt`, `/media`) or home folders (`/home`). `/run` holds only its host socket, so system services' sockets such as D-Bus are not reachable. Other processes are hidden.
- The dashboard checks which user connected to a host socket and closes the connection unless it comes from root or from the dynamic system users that backends run as, so ordinary local accounts on this server cannot use host sockets. A backend cannot reach another package's host socket either, because its `/run` holds only its own.
- Its own `/tmp` and shared memory (`/dev/shm`).
- Memory up to 256 MiB, up to 64 tasks (processes and threads), and a low CPU priority.
- Writable memory cannot be made executable, so runtimes that compile code while they run (JIT compilers) do not work. A statically built program (Go, Rust, C) does.
- No devices, no kernel tunables and no clock changes, and only the system calls ordinary services use.
- The network the manifest's `backend.network` grants:

    | `network` | Where the backend can connect |
    |---|---|
    | `[]` | Nowhere. Only Unix sockets |
    | `["lan"]` | Private addresses (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7), link-local and multicast addresses |
    | `["loopback"]` | This server's own services (127.0.0.0/8, ::1) |
    | `["lan", "loopback"]` | Both |

    Other addresses, the internet included, are blocked. There is no grant for the internet. The risks of `lan` and `loopback` are in [Addon security](addon-security.md#what-the-dashboard-cannot-prevent).

systemd enforces these limits. The points below can depend on the server's setup, so check what is blocked on your server with the test package [probe](https://github.com/steamb23/maru-nas/tree/main/sdk/testdata/probe) (a test tool for administrators, not an example):

- The address limits apply only if the kernel and systemd support IP address filtering.
- System files under `/run` are hidden, so on a server that keeps its DNS configuration under `/run`, even a backend with network access may fail to resolve host names. Connect by IP address in that case.

### Data and removal

- The data directory is `/var/lib/private/maru-addon/<id>`, readable only by root. The backend sees it as `STATE_DIRECTORY`.
- Updating (installing a new file with the same id) and turning a package off keep the data. Turning addon backends off for the installation keeps it as well, but it is deleted when you turn them on again (see "Turning them on and off" above).
- If you choose "Also delete the data saved by its backend" when removing a package, the sync service deletes the data.
- If you don't, the data stays on the server until a package with the same id is installed again, which deletes it before the backend is turned on. This keeps a reinstalled package (possibly another author's package with the same id) from reading the old data, the same rule as for per-user values.

### Logs

- What a backend writes to standard output and standard error goes to the systemd journal. Root and members of the `adm` group read it with `journalctl -u 'maru-addon*@<id>.service'`.
- When a data request or action fails, the dashboard's log (`journalctl -u maru`) also records the package id and the error code.
