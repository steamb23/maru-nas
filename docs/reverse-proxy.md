# Reverse proxy

The dashboard listens only on a loopback address or a Unix socket, and a reverse proxy in front of it handles HTTPS. The install script does not configure the proxy. The examples below use Caddy, and any other proxy works if it meets the [proxy requirements](#proxy-requirements).

The proxy configuration decides the host and path of the dashboard. You can put it at the root of its own subdomain, or under a path prefix (for example `/desk/`) on the same domain as OMV.

## Choosing a listen mode

| Where the proxy runs | Listen mode (`--listen`) | Proxy target |
|---|---|---|
| Directly on this host | Loopback TCP `127.0.0.1:8090` (default) | `http://127.0.0.1:8090` |
| In a Docker container (bridge network) | Unix socket `unix:/run/maru/maru.sock` | Bind-mount `/run/maru` into the container and connect to the socket |
| Anything else that really needs it | `--allow-non-loopback` with `0.0.0.0:8090` or similar | That address |

- `127.0.0.1` inside a container is the container itself and does not reach the host's loopback. For a proxy in Docker, the Unix socket is therefore recommended.
- The socket is owned by the service user and its primary group (`maru:maru` by default) with mode `0660`. A container running as root can connect, but with a proxy image that runs as a non-root user, or with userns-remap, the proxy process must be set up to have that group's permission. For a proxy running on the host, add the proxy user to the service group.
- `/run/maru` stays in place when maru restarts, so the container's bind mount does not break. If you removed it with `./install.sh --uninstall`, recreate the proxy container after installing again.

> [!WARNING]
> When the dashboard listens on an address that is not loopback, anyone who can reach that address can connect without going through the proxy and HTTPS, and passwords are sent over plain HTTP. They can also forge `X-Forwarded-For` and `X-Forwarded-Proto` to get around the sign-in attempt limit or to leave fake IP addresses in the OMV authentication log (fail2ban). If you use this mode, restrict access with a firewall.

## Proxy requirements

| Item | Requirement |
|---|---|
| `X-Forwarded-Proto` | Passes the protocol the browser used (`https` or `http`). The backend marks the session cookie `Secure` only when this is `https`. |
| `X-Forwarded-For` | The last entry must be the browser's IP address. The backend uses only the last entry, for the sign-in attempt limit and the OMV authentication log. Earlier entries are ignored because the browser can send anything there. |
| Path prefix | Strip the prefix when forwarding, and redirect the address without the trailing slash (`/desk`) to `/desk/`, because relative paths resolve against `/desk/`. |
| Cache headers | Pass the `Cache-Control` and `ETag` the backend sends unchanged, so that reloading after an upgrade fetches the new files. |
| WebSocket | Used by the web terminal (`/api/terminal`). Forward WebSocket and do not change the `Host` header (the backend checks that `Origin` matches `Host`). The backend sends a ping every 30 seconds, so the idle connection timeout must be longer than that. |
| Request body size | File explorer uploads send each file as one request body, and wallpapers in the Settings app can be up to 20 MB. Remove the size limit or set it above the largest file you will upload (in nginx, `client_max_body_size 0`; the default is 1 MB). There must also be no time limit on sending and receiving the body while large files transfer. |
| Response buffering | Copying and subfolder search in the file explorer keep sending their progress in a single response (`text/event-stream`). The proxy must pass the response on right away instead of collecting it, or the progress does not show. nginx needs no extra configuration. |
| Response timeout | Must be longer than 3 minutes. Operations such as turning a service on or off wait up to 3 minutes for the OMV task to finish. If the proxy cuts the connection first, the task keeps going but the screen shows a timeout error. nginx and its relatives default to 60 seconds, so raise `proxy_read_timeout`. |

Caddy's `reverse_proxy` meets all of these with its default settings. If there is no other proxy in front of Caddy, do not set `trusted_proxies`, so that Caddy overwrites any `X-Forwarded-For` the browser sent.

## Installing Caddy

You can use Caddy from the Debian 13 (trixie) repository as it is.

```sh
sudo apt install caddy
```

- The package's default configuration uses port 80. If the OMV web UI uses port 80, the installation reports that it could not start `caddy.service`. Change `/etc/caddy/Caddyfile` to one of the examples below and start it with `sudo systemctl restart caddy`.
- After later changes, reload the configuration with `sudo systemctl reload caddy`, and read the log with `journalctl -u caddy`.
- If you run Caddy in Docker, choose the Unix socket in [Choosing a listen mode](#choosing-a-listen-mode) and change the target in the examples below to the socket.

## Caddy example: keeping the OMV default port

If the OMV web UI uses port 80 (HTTP) as it does by default after installation, let Caddy use only port 443 (HTTPS). The OMV settings stay unchanged, and SSL/TLS in the OMV web UI settings (System → Workbench) must stay off (the default) so that port 443 does not clash.

- By default Caddy redirects HTTP to HTTPS on port 80, which clashes with OMV's port. The global option `auto_https disable_redirects` stops Caddy from creating this redirect and from opening port 80. `http://` addresses are then answered by the OMV web UI, and you reach the dashboard at an `https://` address.
- Without port 80, certificates cannot be issued with the HTTP-01 challenge. On a local network, use one of the following.
    - `tls internal`: Caddy issues the certificate from its own CA, for IP addresses as well as host names. To get rid of the browser warning and to use passkeys, add Caddy's root certificate as a trusted certificate on the devices that connect. With Caddy from the Debian package it is `/var/lib/caddy/.local/share/caddy/pki/authorities/local/root.crt`, otherwise `<Caddy data directory>/pki/authorities/local/root.crt`.
    - DNS-01 challenge: if you own a real domain, a Caddy build with your DNS provider's module can get a public certificate. See `dns` under the `tls` directive in the Caddy documentation.

At the root of a subdomain:

```caddyfile
{
	auto_https disable_redirects
}

dash.example.lan {
	tls internal
	reverse_proxy 127.0.0.1:8090
}
```

The next example puts the dashboard under a path prefix on the same address as OMV. `https://nas.example.lan/` goes through Caddy to the OMV web UI, and `https://nas.example.lan/desk/` to the dashboard. The existing `http://nas.example.lan/` is still the OMV web UI. In this case the [OMV Workbench address](configuration.md#opening-the-omv-workbench) can be `/`.

```caddyfile
{
	auto_https disable_redirects
}

nas.example.lan {
	tls internal
	redir /desk /desk/ 308
	handle_path /desk/* {
		reverse_proxy 127.0.0.1:8090
	}
	handle {
		reverse_proxy 127.0.0.1:80   # OMV web UI (default port)
	}
}
```

Both examples assume that Caddy runs on the host. For Caddy in a Docker container, connect to the dashboard as in the Unix socket example below, and change the OMV web UI address to a host address the container can reach.

## Caddy example: Caddy as the main reverse proxy

In this setup Caddy takes both ports 80 and 443, redirects to HTTPS and issues and renews certificates, with both the OMV web UI and Maru NAS behind it. Move OMV to another port and turn SSL/TLS off in the OMV web UI settings (System → Workbench). The examples below assume port `8001`. Set the OMV address of Maru NAS to the new port as well, `http://127.0.0.1:8001` (the install script finds the port in the nginx configuration and suggests it).

At the root of a subdomain, with the proxy running on the host:

```caddyfile
dash.example.com {
	reverse_proxy 127.0.0.1:8090
}
```

From Caddy in a Docker container over the Unix socket (add the volume `- /run/maru:/run/maru` to the container):

```caddyfile
dash.example.com {
	reverse_proxy unix//run/maru/maru.sock
}
```

Under a path prefix on the same domain as OMV:

```caddyfile
nas.example.com {
	redir /desk /desk/ 308
	handle_path /desk/* {
		reverse_proxy 127.0.0.1:8090
	}
	handle {
		reverse_proxy 127.0.0.1:8001   # OMV web UI (moved port)
	}
}
```
