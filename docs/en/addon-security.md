# Addon security

The dashboard works on the assumption that package code is not trusted. This page summarizes what the dashboard prevents, what it cannot prevent and what package authors should do. The explanation for administrators is in "Risks and limitations" of [Addon packages](addons.md).

## What the dashboard prevents

### Screen isolation

- Package screens run without an origin in a `sandbox="allow-scripts"` iframe, and the responses for frame files carry the same sandbox and a Content-Security-Policy. Opening a frame address directly in a new tab is the same.
- They cannot reach the cookies, browser storage, screens (DOM) or API of the dashboard or the OMV workbench. Frame file requests carry a token chosen per session and package in the path instead of cookies, and the token stops working when the user signs out.
- Network requests of their own, scripts and images from other sites, form submissions, popups and navigating the dashboard page are blocked. The dashboard page only shows frames from its own origin, so a frame cannot move itself to another site either.

### Requests to the dashboard

- A request is handled only in the name of the package that loaded the frame. Writing another package's id in a message has no effect.
- Requests that need a user action (opening the workbench, and actions later) are accepted only right after the user pressed that frame. Widgets can only make read requests.
- Request size, the number of requests at once and the request rate are limited ([Building screens](addon-frontend.md#api)).
- Confirmation dialogs, notifications and title bars the dashboard shows for a package carry the package name and an "Addon package" mark.

### OMV calls

- Packages call only the methods declared in the manifest's `rpc.read`. The dashboard makes the call with the signed-in user's OMV session, so the OMV session ID never reaches the package and the package cannot go beyond that user's OMV permissions.
- The general RPC relay the built-in addons use does not pass package methods.
- OMV writes are made only by actions (a later version), and actions of packages that declare writes ask the user to confirm their identity every time they run.

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

- Only OMV administrators install packages, and installing, updating and turning a package back on ask for the password again (and a security method, if one is registered).
- Package files are checked for paths, formats and sizes, unpacked separately and then moved into place. File permissions from the archive are not used.
- The values a package saves per user are kept separately per package and user. A package that was removed and installed again (possibly another author's package with the same id) cannot read the earlier values.

## What the dashboard cannot prevent

- Leaking what a screen shows: WebRTC and DNS prefetching are not fully blocked by the Content-Security-Policy. Assume that values a package screen received can reach its author.
- Fake input forms: the dashboard cannot tell a password field drawn inside a package window apart. The dashboard tells users it never asks for a password inside a package window.
- Values from declared methods: declaring `System.getInformation` reads its whole result, including values OMV gives only to administrators when an administrator opens it.
- The author's identity: there are no signatures. Administrators check where they got the file and its SHA-256.

## What authors should do

- Declare only the methods you need, and explain in the README why you use each one.
- Handle administrator-only values by checking `context.admin`. Do not leave administrator values in regular users' screens.
- Do not ask for passwords, tokens or OMV account information in your screens or keep them in saved values.
- Put strings OMV returned (host names, shared folder names and so on) and user input into the page with `textContent`, not `innerHTML`. Even inside the frame they can break another user's screen.
- Do not build features that reach external services. Screens cannot reach the network, and Maru NAS is meant for the local network only.
