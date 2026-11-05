# Addon packages

Addon packages are addons built outside the Maru NAS repository. Like built-in apps they become windows in "All apps" and desktop widgets, and an administrator installs them by uploading a file (`.maru-addon`) in the Settings app. The dashboard does not restart.

A package's screens run in a sandbox separate from the dashboard, and a package can only call the OMV methods it declared in advance. Still, package code is not code the dashboard has reviewed. Review the permissions and install packages only from authors you trust. How to build a package is in [Addon development](addon-dev.md).

## Installing

1. On the "Addons" page of the Settings app (administrators), press "Upload package…" and choose a `.maru-addon` file.
2. Check the package information and permissions on the review screen. See "Reading the review screen" below.
3. Press "Install" and enter your password again. If you registered a security method (passkey, authenticator app), confirm with it as well.

- Once installed, the package appears in "All apps" for every user right away (for administrators only if it is an administrator-only package). Shortcuts and widgets are per user.
- Package files can be up to 64 MiB. The reverse proxy's request body size limit must also be larger than that ([Proxy requirements](reverse-proxy.md#proxy-requirements)).
- An uploaded file stays on the server for up to 30 minutes while you review it and is deleted if you do not install it.
- Packages cannot be installed while the dashboard runs without a state directory (a development server, for example).
- Packages with a backend (a program that runs on the server) can be installed, but this version does not run backends on installed dashboards. The review screen warns "Addon backends are turned off on this installation. You can install it, but it can't be used until backends are turned on." After installing, the app shows "This app can't be used because addon backends are turned off on this installation." and its widgets "Backend turned off". An install option that turns backends on comes in a later version.
- A package with a backend that has no executable for this server's architecture is not installed ("The package has no backend for this server's architecture.").

## Reading the review screen

The name, description and author on the review screen are written by the package itself and are not verified. Packages are not signed. Compare the SHA-256 on the review screen with the one the author published to make sure you got the right file.

| Item | Meaning |
|---|---|
| Available to | "All users" or "Administrators only". Administrator-only packages are not shown to regular users |
| Required plugins | OMV plugins the package uses. If one is not installed, the app shows as unavailable |
| OMV reads | OMV RPC methods the package can read. They are called with the signed-in user's OMV permissions, so a user only reads what they can see in OMV |
| OMV writes | OMV settings the package can change. If there are any, they are listed as a risk |
| Actions | Operations the package can run (such as restarting a service) |
| Backend | Whether the package includes a program that runs on the server |
| Backend network | Where the backend can connect: "No network access", "LAN" (other devices on this network), "Other services on this server (loopback)" |

- Always read the risks in the red box. OMV write permissions call for trust close to root (some settings an OMV administrator can change end up running commands as root).
- For packages with a backend the risks also say that the backend is shared by all users (values read with an administrator's session may be shown to regular users), and, with network access, that "LAN" may reach the internet through the LAN's router or a proxy and that loopback reaches the other services on this server.
- Uploading a new version of the same package shows "Review addon update" with the installed and new versions and the permission changes ("New", "Removed"). It tells you when the permissions widen.
- Warnings at the top of the review screen tell you when the author changed, the version is older than the installed one, the version number is the same but the contents differ, or the file is the same as the installed one. They do not block installing.

## Updates

- Uploading a package file with the same id updates the package. Updates go through the same confirmation as installs.
- Package windows and widgets users have open reload with the new version the next time the dashboard fetches the app list.
- There are no automatic updates. Upgrading the dashboard leaves installed packages as they are.

## Turning packages off and removing them

- Use the switch in the list of installed addons to turn a package off or on. Turning it off removes it from every user's apps and desktop, and open windows change to "Unavailable addon".
- Turning a package back on goes through the same confirmation (password) as installing. Turning it off does not ask.
- "Remove" deletes the package files. For packages with a backend you choose whether to also delete the data its backend saved (this needs the identity confirmation).
- Users' shortcuts and widget placements are kept when you turn a package off or remove it, and come back when you turn it on or install it again. Values users saved in a package are not shown after a reinstall (a newly installed package cannot read the earlier data).
- Development packages loaded by a development server ("Development" badge) cannot be turned off or removed here ([Addon development](addon-dev.md)).

## Packages that cannot be used

Packages with an "Invalid", "Incompatible", "Conflict" or "Not allowed" badge in the list are not used even when they are on. "Details" shows why.

- Incompatible: the package needs a newer SDK than this dashboard version. Update the dashboard or use a package made for this version.
- Conflict: its id is the same as a built-in addon's or another package installed earlier. If a dashboard upgrade adds a built-in addon with the same id, the built-in addon is used.
- Not allowed: the package declares an OMV method addon packages may not use (running commands, reading credentials and so on), or uses an id reserved by the dashboard.
- Invalid: the installed files cannot be read. Install the package again.
- If the install record file cannot be read, the page says so at the top and none of the installed packages are used. Check the server log.

## What users see

- Package windows have an "Addon package" badge in the title bar. Packages choose their own names, so a name can be the same as a built-in app's.
- Confirmation dialogs a package asks for are titled "Request from the <name> addon", and its notifications "Addon <name>".
- The dashboard never asks for a password or security method inside a package window. If a package window shows a password field, do not type into it.
- The values a package saves per user can be up to 64 KiB per package and 1 MiB per user.

## Backing up packages

Installed packages and the values users saved in them are in the dashboard's state directory (`/var/lib/maru`). Backing up that directory as in [State directory and backups](configuration.md#state-directory-and-backups) includes them.

## Risks and limitations

- Package screens cannot reach the dashboard's cookies, storage or other apps' screens, and cannot send network requests themselves. Some browser features (WebRTC, DNS prefetching) are not fully blocked, though, so assume that values shown in a package screen can reach its author.
- The OMV values a package can read are limited to the "OMV reads" on the review screen, but it reads everything those methods return.
- A package's actions that change OMV settings run with the signed-in user's OMV permissions. The dashboard asks for the password again (or two-step verification, if the security policy for root elevation requires it) once per session before the first of them, and the check lasts until the session ends.
- A backend is shared by all users and can show regular users values it read with an administrator's session. Once installed backends run (a later version), a backend with "LAN" access can also reach Docker networks and, through the LAN's resolvers and proxies, the internet, and one with loopback access can reach OMV's web interface, SSH and the dashboard itself ([Addon security](addon-security.md#what-the-dashboard-cannot-prevent)).
- The dashboard cannot stop a fake input form drawn inside a package window.
- There are no signatures, catalog or automatic updates. Check where you got the file and its SHA-256.
