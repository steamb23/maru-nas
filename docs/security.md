# Security hardening mode

한국어: [보안 강화 모드](security.ko.md)

Security-sensitive features such as the web terminal, root elevation and the file explorer can be limited to sessions that have passed one more check. The goal is that a browser left open, a stolen session cookie or a leaked password alone is not enough to use those features. It is off by default.

## Security policy

An administrator chooses a level for each feature on the "Security policy" page of the Settings app.

| Level | Check |
|---|---|
| Off | No extra check. |
| Password again | The password must be entered once more in that session. The password entered at sign-in does not count. |
| Two-step verification | A passkey, an authenticator app (TOTP) or a recovery code is required. A session signed in with OpenMediaVault two-factor authentication also counts as having passed two-step verification. |

- By default the file explorer checks even for browsing folders and downloading. If you change "File explorer check scope" to "Writes and deletions only", browsing and downloading need no check, and the check is asked only before uploading, creating folders, renaming, moving, deleting and trash operations. Changing the scope requires a session that has passed that level.
- The same page also sets how long [passkey sign-in](passkey-login.md) lasts.
- A check stays with the session. You are not asked again until you sign out or the session expires (no input for the OMV automatic logout time). If OMV automatic logout is turned off, a checked session lasts until sign-out, so the Security policy page shows a warning.
- When a feature requires two-step verification, users who have registered a sign-in method are offered two-step verification right after signing in. On a device without the method they can skip it, and the same check appears when they open that feature.

### Changing the policy

- The administrator changing the policy must also be in a session that has passed the higher of the feature's old and new levels. This keeps someone with a stolen session from lowering the policy and opening the feature.
- Raising a policy immediately closes the web terminals of sessions that do not meet it, and ends their root elevation.
- The file explorer is blocked from the next request, and a verify button appears in the window. With "Writes and deletions only", a check appears when you write, and after the check the operation continues. Transfers that have already started run to the end.

## Registering sign-in methods

Each user registers methods on the "Security" page of the Settings app. Registering and deleting methods and creating new recovery codes are possible for 5 minutes after confirming your password and (if you already have one) a registered method. When adding a passkey on a new device, a device code is also accepted in place of that method ([Adding a passkey on a new device](#adding-a-passkey-on-a-new-device)).

- **Passkey**: confirm with the device's screen lock (fingerprint, face, PIN) or a security key. This is the most recommended method.
    - The public address (`-public-origin`, [Configuration](configuration.md)) must be set, and you must reach the dashboard at an HTTPS domain with a certificate the browser trusts. IP addresses and self-signed certificates do not work, and if you change the public address, passkeys must be registered again.
    - Registration asks you to confirm once more with the new passkey, and the passkey is not registered if the passkey storage does not report user verification (PIN, biometrics). Examples are password managers that skip verification while unlocked, and passkeys on Windows that are saved only to Microsoft Password Manager. Register again, choosing this device (Windows Hello and the like) or a security key in the browser's dialog.
    - For a passkey that was not registered, the dashboard has the browser remove it from the storage if the browser supports that, and otherwise explains how to remove it.
    - Passkeys synced across devices, such as those in iCloud Keychain or password managers, are marked "Synced" in the list. They are not blocked.
- **Authenticator app (TOTP)**: a 6-digit code from an app such as Google Authenticator or 2FAS. Register it by scanning a QR code or entering the key. It works without a public address.
- **Recovery codes**: 10 codes are created when you register your first method, and they are shown only once. Each code works once, and you can create new ones on the Security page.

## Adding a passkey on a new device

A new device often does not have the passkey created on another device. So when you add a passkey, the identity check first asks for the authenticator app if you have one, and otherwise for a device code. A synced passkey such as one in iCloud Keychain, or the passkey on your phone, can be used directly by pressing "Use a passkey".

1. On the Security page of a device that has your passkey, press "Create code" under "Add a passkey on another device". That device also asks again for your password and a registered method.
2. On the new device's Security page, press "Add a passkey", then enter your password and this code in the identity check.

- A code works once within 10 minutes, and creating a new one makes the previous code unusable. Unlike a recovery code, using it does not reduce the number of recovery codes left.
- An identity check completed with a code allows, for 5 minutes, not only adding a passkey but also deleting methods and creating new recovery codes. Do not share the code with anyone.
- A session that completed the identity check with a code does not count as having passed two-step verification. Once you register the new passkey, the session counts as two-step verified.

## Recovery

- A user who lost their method confirms with a recovery code and registers a new method.
- If there is no recovery code either, an administrator who has passed two-step verification resets that user's methods under "Users' sign-in methods" on the Security policy page. That user's sessions end, and the user can register again after signing in.
- If a record file is corrupted and cannot be read, the dashboard does not treat that user as having no two-step verification, and blocks the check instead. It recovers when an administrator resets the methods or root deletes the file as below.

If an administrator lost their own methods, root deletes the record file in the state directory. A user name with characters other than letters, digits, `_` and `-` is written with `%XX` in the file name, so check first with `sudo ls /var/lib/maru/security/`.

```sh
sudo rm /var/lib/maru/security/<user name>.json
```

## Design and limitations

- Checks are done by the dashboard and were not put into the [privileged helper](helper.md). The helper's security design is unchanged.
- Sign-in method records are in `security/<user name>.json` in the state directory (directory `0700`, files `0600`). They contain authenticator app secrets, so protect them when you back up the state directory. Recovery codes are kept only as hashes. The key share of passkey sign-in is also in this record, but it cannot decrypt anything on its own ([Design](passkey-login.md#design)).
- A password re-check confirms the password by signing in to OMV once more, and failures count toward the sign-in failure count. Failed second-step attempts follow the same rule (locked from the 5th failure within 15 minutes, per browser IP and user name), but they are counted separately from sign-in failures.
- Successful and failed checks, registering and deleting methods, turning on, turning off and signing in with passkey sign-in, policy changes and resets are recorded in `journalctl -u maru`. Secret values such as passwords and codes are not recorded.

There are also things it cannot prevent.

- Accounts that have not registered a sign-in method yet. The first method can be registered with the password alone, so if a password leaks, someone else can register first. If a feature will require two-step verification, have every user register a method in advance ("Users' sign-in methods" on the Security policy page shows who has registered).
- A compromise of the dashboard (maru) itself. The values being checked also pass through the dashboard.
- If an OMV administrator password leaks, the attacker can sign in to the OMV web UI directly and do the equivalent of root. We recommend also using OpenMediaVault two-factor authentication (OMV 8.5 and later) for administrator accounts.
- The SSH server is open to the LAN anyway, so direct SSH access is governed by OMV's SSH settings.
