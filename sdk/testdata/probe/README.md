# Sandbox probe (test tool)

**This is a test tool for Maru NAS development, not an example.** Install it only on a test machine. It checks that
the systemd sandbox of an installed addon backend blocks what it should.

The backend's source is in the Go SDK module,
[`sdk/go/examples/probe`](https://github.com/steamb23/maru-nas/tree/main/sdk/go/examples/probe). Its only action,
`run` (administrators only), tries the following one by one and returns a table. Each row says only whether the
attempt got through and the error class; nothing that was read is returned or logged.

| Check | What it tries |
|---|---|
| `state-users`, `state-security`, `state-site` | read the dashboard's state directory (`/var/lib/maru`) |
| `addon-sync` | list the backend sync records (`/var/lib/maru-addons`) |
| `maru-runtime` | list `/run/maru` |
| `addon-sockets`, `addon-hosts` | list other addons' backend sockets (`/run/maru-addon`) and host socket directories (`/run/maru-addon-host`) |
| `helper-socket` | connect to the privileged helper (`/run/maru-helper/helper.sock`) |
| `dbus-system`, `resolved-varlink` | connect to the system D-Bus and systemd-resolved sockets |
| `omv-config` | read OMV's configuration database (`/etc/openmediavault/config.xml`) |
| `srv`, `home` | list shared folders and home directories |
| `abstract-unix` | connect to a listening abstract Unix socket visible in `/proc/net/unix` |
| `loopback-http` | TCP to `127.0.0.1:80` (OMV's web server) |
| `lan` | TCP to the LAN address typed in the window (skipped when empty) |
| `public` | TCP to `1.1.1.1:443` |
| `host-without-token` | call maru's host socket without a call token |

Results:

- `blocked`: the path is missing or hidden (`ENOENT`, `ENOTDIR`), access was denied (`EACCES`), the address family
  is not allowed (`EAFNOSUPPORT`, from `RestrictAddressFamilies=`), the IP address is denied (`EPERM`, from
  `IPAddressDeny=`), or the host socket refused the call (`call_invalid`)
- `reachable`: the file or directory was opened, the socket accepted the connection, or a TCP connection was
  made or refused by the other side (`ECONNREFUSED` means the packet arrived)
- `timeout`: no answer in 2 seconds (a firewall dropping packets, an address nobody uses). This is not a sandbox
  block
- `error`: any other failure, such as `ENETUNREACH`, or a missing host socket

The window's "Network in the manifest" boxes must match `backend.network` in `manifest.json`; they set the expected
result. Without network every row must be blocked. With `loopback`, `loopback-http` is expected to get through; with
`lan`, the LAN address is. With any network the backend shares the host's network namespace, so reaching an
abstract Unix socket is a known limit and that row is not judged. Under the network-less unit (`PrivateNetwork=yes`)
`127.0.0.1` is the unit's own loopback, so a TCP attempt there would only be refused; `RestrictAddressFamilies=AF_UNIX`
is what turns it into `blocked`.

## Run it

On a development server there is no sandbox, so most rows get through. That is expected; the probe is for an
installed backend. To try the tool itself on the development server, load this directory with
`MARU_DEV_ADDONS=$PWD/sdk/testdata/probe make dev-fake` (it is not part of `FAKEADDONS=examples`) and start the
backend from `sdk/go`:

```sh
cd sdk/go
MARU_ADDON_SOCKET=<state>/addon-dev/probe.sock MARU_ADDON_HOST=<state>/addon-host/probe.sock go run ./examples/probe
```

To check a sandbox, build it with `make sdk-examples`, pack it with
`node sdk/js/cli/maru-addon.mjs pack sdk/testdata/probe`, and install it on a test machine whose installation has
addon backends turned on (`./install.sh --addon-backends`).
Repeat with `backend.network` set to `[]`, `["lan"]`, `["loopback"]` and `["lan", "loopback"]` (change the version
each time so the dashboard installs it as an update).
