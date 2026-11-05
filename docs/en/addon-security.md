# Addon security

The dashboard works on the assumption that package code is not trusted. This page summarizes what the dashboard prevents, what it cannot prevent and what package authors should do. The explanation for administrators is in "Risks and limitations" of [Addon packages](addons.md).

## What the dashboard prevents

### Screen isolation

- Package screens run without an origin in a `sandbox="allow-scripts"` iframe, and the responses for frame files carry the same sandbox and a Content-Security-Policy. Opening a frame address directly in a new tab is the same.
- They cannot reach the cookies, browser storage, screens (DOM) or API of the dashboard or the OMV workbench. Frame file requests carry a token chosen per session and package in the path instead of cookies, and the token stops working when the user signs out.
- Network requests of their own, scripts and images from other sites, form submissions, popups and navigating the dashboard page are blocked. The dashboard page only shows frames from its own origin, so a frame cannot move itself to another site either.

### Requests to the dashboard

- A request is handled only in the name of the package that loaded the frame. Writing another package's id in a message has no effect.
- `app.action` and opening the workbench are accepted only right after the user pressed that frame. `app.query` runs the same actions without that, so give actions that change something `confirm`, and `admin` when only administrators should run them. Widgets can only make read requests.
- Request size, the number of requests at once and the request rate are limited ([Building screens](addon-frontend.md#api)).
- Confirmation dialogs, notifications and title bars the dashboard shows for a package carry the package name and an "Addon package" mark.

### OMV calls

- Screens call only the methods declared in the manifest's `rpc.read`, and backends only those in `rpc.read` and, while they run an action, `rpc.write`. The dashboard makes the call with the signed-in user's OMV session, so the OMV session ID never reaches the package and the package cannot go beyond that user's OMV permissions.
- The general RPC relay the built-in addons use does not pass package methods.
- OMV writes are made only by the backend while it runs an action. The dashboard checks the action's administrator flag and shows its confirmation dialog (if the action has `confirm`) before the request reaches the backend, every time.
- Actions of packages that declare OMV writes also need a session that has confirmed the password again (or passed two-step verification, when the security policy for root elevation asks for it), even when the security policy is off. The dashboard asks once per session, and the check lasts until the session ends. A stolen session cookie alone is not enough to make a package write to OMV.

### Backends

- The dashboard never runs a package's executable itself. Installed backends run only on installations where an administrator has turned addon backends on, as systemd units that a sync service running as root turns on, each in a sandbox of its own ([Addon backends](addon-backend.md#installed-backends)). The sync service does not read package files, and it treats the list the dashboard writes as untrusted input and checks it strictly.
- Each backend runs as a dynamic system user of its own and cannot reach the dashboard's state outside its own package files and data directory, OMV's configuration, shared folders, home folders or system services' sockets. A package that was removed and installed again cannot read the earlier backend data.
- A host socket checks which user connected and closes the connection unless it comes from root or from the dynamic system users that backends run as, so ordinary local accounts on this server cannot use host sockets. A backend's `/run` holds only its own package's host socket.
- Backends never get the OMV session ID, passwords, dashboard cookies or root. They call OMV through the dashboard's host socket with a call token made for each request. The token works only until that request ends and only on that package's host socket, so a backend cannot call OMV on its own or with another package's rights.
- The host socket runs only declared methods (only reads during data requests) and checks the table of methods that are not allowed again. Applying configuration is refused while OMV has pending changes, so a package never deploys changes someone else made.
- Backend answers pass only as JSON with a fixed set of statuses. Error codes are checked, messages are stripped of control and bidirectional control characters and cut to 300 characters, and response headers are dropped. A backend cannot sign users out or imitate the dashboard's error codes, and its messages appear only inside its own frame, not in the dashboard's dialogs or notifications.
- Every action is recorded in the server log with the user, package, action, target and result.

### Methods that are not allowed

Methods where the caller chooses a command, script, package, file contents or host path, methods that return credentials or check passwords, and flows the dashboard handles itself cannot be declared. A package that declares one is not installed ("Not allowed"). Service names are matched without regard to case.

| Target | Reason |
|---|---|
| The whole `Session`, `Exec` and `Config` services | Signing in, background jobs and applying configuration are handled by the dashboard |
| The whole `Cron`, `Scripts`, `PluginMgmt`, `Apttool`, `Omvextras` and `EnvVars` services | Running commands or scripts, installing packages or repositories, environment variables |
| `Apt.install`, `upgrade`, `update`, `upload`, `getChangeLog` | Package installation, arguments that end up in a shell command |
| The command, log and file methods of `Compose` (`doCommand`, `doCommandList`, `doPrune`, `doServiceCommand`, `doContainerCommand`, `getContainerLog`, `doAutocompose`, `doDockerCmd`, `doDockerVolumeCmd`, `doDockerNetworkCmd`, `doDockerImageCmd`, `doNetworkConnect`, `doGit`, `doBuild`, `doJob`, `setJob`, `setFile`, `setDockerfile`, `setConfig`, `setGlobalEnv`, `doRestore`) | They build Docker or compose commands or write files |
| `set*`, `delete*`, `import*` and `authUser` of `UserMgmt` | Changing users and groups (such as adding someone to sudo), checking passwords |
| `EmailNotification.get`, `Rsync.get`, `Network.getProxy`, `Network.getInterface`, `Network.getInterfaceList`, `Network.getWirelessIface` | They return passwords or wireless keys |
| `Symlinks.setSymlink`, `Kvm.setVmXml`, `Kvm.addFsPass`, `Kvm.addHostDisk` | They expose host paths or devices |

This table blocks only the direct paths. OMV writes that are not in it (extra options in service settings and so on) can still reach root, so packages that declare OMV writes are shown as a risk on the review screen. Please discuss proposals to shrink the table in an issue first.

### Installation and storage

- Only OMV administrators install packages, and installing, updating and turning a package back on ask for the password again (and a security method, if one is registered). Packages with OMV writes, a backend or network access also need the check their write actions need.
- Package files are checked for paths, formats and sizes, unpacked separately and then moved into place. File permissions from the archive are not used.
- The values a package saves per user are kept separately per package and user. A package that was removed and installed again (possibly another author's package with the same id) cannot read the earlier values.

## What the dashboard cannot prevent

- Leaking what a screen shows: WebRTC and DNS prefetching are not fully blocked by the Content-Security-Policy. Assume that values a package screen received can reach its author.
- Fake input forms: the dashboard cannot tell a password field drawn inside a package window apart. The dashboard tells users it never asks for a password inside a package window.
- Values from declared methods: declaring `System.getInformation` reads its whole result, including values OMV gives only to administrators when an administrator opens it.
- The author's identity: there are no signatures. Administrators check where they got the file and its SHA-256.
- A shared backend: one backend serves every user. It can show values it read with an administrator's session to regular users, and the data it keeps is shared by everyone. It also sees the results of its declared methods for every user who uses the package.
- The backend network of installed backends, as the manifest's `backend.network` grants it:
    - `lan` reaches the devices on the LAN, and through the LAN's DNS resolvers and proxies it can reach the internet. This server's own LAN address is a private address too, so it also reaches the services this server offers on the LAN (OMV's web server, sshd, SMB, the dashboard behind a reverse proxy, ports published by containers). Docker's networks on this server normally use the same private range (172.16.0.0/12), so their containers, and this server's services on the Docker bridge address, are reachable too.
    - `loopback` reaches every service on this server that listens on loopback: OMV's web server, sshd and the dashboard's own listener (where a backend could try passwords), and the local DNS resolver.
    - With either grant the backend shares the server's network namespace, so it can also reach abstract Unix sockets there (such as those of container tools).

## What authors should do

- Declare only the methods you need, and explain in the README why you use each one.
- Handle administrator-only values by checking `context.admin`. Do not leave administrator values in regular users' screens.
- Do not ask for passwords, tokens or OMV account information in your screens or keep them in saved values.
- Put strings OMV returned (host names, shared folder names and so on) and user input into the page with `textContent`, not `innerHTML`. Even inside the frame they can break another user's screen.
- In a backend, decide what to return from each request's user and administrator flag. Do not keep values read with an administrator's session to return them to regular users, and do not keep users' secrets in the data directory.
- Check an action's `target` and `input` in the backend before putting them into OMV parameters. Make actions that change OMV settings administrator-only with `confirm`, and apply them with `Host.Apply`.
- Ask for no network unless the backend needs it, and explain `lan` or `loopback` in the README. Never log the call token.
- Do not build features that reach external services. Screens cannot reach the network, backends have no grant for the internet, and Maru NAS is meant for the local network only.
