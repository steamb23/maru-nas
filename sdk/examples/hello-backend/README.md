# Hello backend

An example Maru NAS addon package with a backend written in Go. The window is plain HTML and JavaScript (`ui/`);
the backend's source is in the Go SDK module,
[`sdk/go/examples/hello-backend`](https://github.com/steamb23/maru-nas/tree/main/sdk/go/examples/hello-backend).
It shows how to:

- answer the frontend's data requests (`app.data()`) from a backend, which reads OMV through maru with
  `Host.RPCInto` (`System.getInformation`, and for administrators `PerfStats.get`)
- keep a value in the backend itself (a count of data requests, and who last changed the setting) in the backend's
  data directory (`maruaddon.StateDir()`), or in memory when there is none. Every dashboard user talks to the same
  backend, so the count is shared and the name of the last administrator is only sent to administrators
- run an administrator-only action that the dashboard confirms first (`"admin": true`, `"confirm": true`) and that
  changes OMV's configuration with `Host.Apply`, which also applies the change instead of leaving it pending
- turn a backend error code into a message in the frame (`config_dirty` becomes `MaruError.addonCode`)

## The OMV change it makes

The action turns OMV's performance statistics (the "Enabled" box under System → Monitoring in the workbench,
`PerfStats.set` with `{"enable": true|false}`) off or on. It was chosen because it is part of OMV itself, takes one
boolean, and is easy to undo from the same window: switching it off stops collectd and rrdcached and pauses the
performance graphs, and switching it back on restarts them. Statistics already recorded stay on disk. It does not
touch the network, users, authentication, storage or the web interface, and it is not on the dashboard's list of
methods packages may not declare. The method, its parameters and the services it redeploys were checked against the
OMV 8.0 to 8.6 sources (`/usr/share/openmediavault/engined/rpc/perfstats.inc`, `rpc.perfstats.set`) and are the
same from 8.0 to 8.6.

Applying the change runs OMV's configuration deployment (Salt), and switching the statistics back on also redraws
the graphs, which can take a while on a NAS with many disks. That is why the action has a long time limit
(`"timeoutSeconds": 300`). If OMV already has changes waiting to be applied, maru refuses to apply them on the
package's behalf and the window asks you to apply or revert them in the workbench first.

Because the package declares an OMV write (`rpc.write`), the dashboard shows it as needing root-level trust when you
install it. The dashboard also asks you to confirm the action each time it runs. Before the first run, if your session
has not confirmed your password yet (or a second factor, when the root elevation policy asks for one), the dashboard
asks for that once; the confirmation lasts for the rest of the session, so installing the package may already have
done it.

## Try it

From the repository root, `FAKEADDONS=examples make dev-fake` loads this directory and starts its backend with
`go run` (the backend's socket and data directory are under the development state directory). Sign in as an
administrator of the fake OMV, open "Hello backend" from All apps, and switch the statistics off and on again. The
fake OMV keeps the setting in memory.

To run the backend yourself (for example with `MARU_DEV_ADDONS` and `make dev`, which talks to the real OMV), start it
from `sdk/go` while the development dashboard is running:

```sh
cd sdk/go
MARU_ADDON_SOCKET=<state>/addon-dev/hello-backend.sock \
MARU_ADDON_HOST=<state>/addon-host/hello-backend.sock \
MARU_ADDON_STATE=/tmp/hello-backend-data \
go run ./examples/hello-backend
```

`<state>` is the development state directory (`MARU_STATE_DIR`, by default `.dev-state/real` or `.dev-state/fake` in
the main checkout).

## Build and pack

`make sdk-examples` builds static executables for both architectures into `backend/linux-amd64` and
`backend/linux-arm64` (they are not committed). Then pack the package:

```sh
make sdk-examples
node sdk/js/cli/maru-addon.mjs pack sdk/examples/hello-backend
```

Packing refuses a package that declares a backend but has no executable.
