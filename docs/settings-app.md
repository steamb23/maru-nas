# Settings app

한국어: [설정 앱](settings-app.ko.md)

Open it with "Settings" in "All apps", the header (your user name) of the profile menu on the taskbar, or "Open screen settings" in the desktop's right-click menu. It covers only dashboard settings, your own account's profile, security methods and information.

| Page | Contents | Who |
|---|---|---|
| Profile | Profile photo (upload and crop), changing email and tags (the same values as Tags in the OMV user editor), opening the OMV user editor (administrators) | All users |
| Screen | Theme (Automatic, Light, Dark), desktop wallpaper (the default wallpaper or an uploaded image), taskbar icon alignment (Center, Left) | All users |
| General | Language (applies to this browser only) and animations | All users |
| Security | This session's verification level and "Verify now", managing passkeys, the authenticator app and recovery codes, device codes for a new device, browsers that sign in with a passkey ([Security hardening mode](security.md), [Passkey sign-in](passkey-login.md)) | All users |
| Login screen | Sign-in background, turning the idle animation on or off | Administrators |
| Default screen | The default theme and wallpaper for users who have not chosen their own screen settings | Administrators |
| Server | Changing the workbench address, the SMART check time and whether the web terminal is allowed ([Settings you can change on the web](configuration.md#settings-you-can-change-on-the-web)), privileged helper status and root elevation, file explorer options (reading the shared folder list, showing disk tops, following OMV shared folder privileges, creating disk trashes), current settings and how to change them | Administrators |
| Security policy | Hardening level per feature (web terminal, root elevation, file explorer and its check scope), how long passkey sign-in lasts, passkey public address status, resetting users' sign-in methods | Administrators |
| Update | [Update channel](upgrade.md#update-channel), checking for a new version, the commit list and new settings, [upgrading from the dashboard](upgrade.md#upgrading-from-the-dashboard) | Administrators |
| About | Version, license, open source notices | All users |

- Administrator pages are visible only to OMV administrator accounts, and their settings apply to all users.
- Wallpapers can be PNG, JPEG or WebP, up to 20 MB. The reverse proxy's request body size limit must also be larger than that ([Proxy requirements](reverse-proxy.md#proxy-requirements)).
- The sign-in background is shown before signing in, so anyone who can reach the dashboard address can see it.
- Email and tags in the profile are OMV account information. Saving them leaves pending "Apply changes" in OMV, so for administrator accounts saving also applies them (it does not save if OMV has changes that were not applied yet), and for regular user accounts an administrator has to apply them in the OMV web UI. System accounts such as the OMV default administrator (`admin`), and accounts that OMV does not allow to be edited, cannot be changed.
- Change the password, groups and other account information, and any other OMV or operating system settings, in the OMV web UI.
