# Passkey sign-in

Sign in with a registered passkey instead of typing your password. It is turned on per browser and works regardless of the levels in the [security policy](security.md#security-policy).

## Requirements

- The same requirements as for [passkeys](security.md#registering-sign-in-methods) (a public address and an HTTPS domain the browser trusts).
- The passkey must support the PRF extension. A passkey without PRF cannot turn this on and is used only for two-step verification. PRF support differs between platforms and passkey storages.
- With storages that must create PRF together with the passkey, such as security keys, a passkey registered before this feature existed may not have PRF. Register the passkey again.

## Turning it on and signing in

- **Turning it on**: when a user with a registered passkey signs in with their password, they are offered "Sign in with a passkey on this browser from now on" right after signing in. Confirming once with the passkey turns it on. It can also be turned on from the "Security" page of the Settings app after confirming the password and a passkey. One user can turn it on in up to 20 browsers.
- **Signing in**: press the "Sign in with passkey" button on the sign-in screen and confirm with your passkey. This session counts as having passed two-step verification, so features that require it do not ask again. Accounts that use OpenMediaVault two-factor authentication still go through that verification.

## Expiry and turning it off

- **Expiry**: it expires after the period an administrator chose on the "Security policy" page (7, 30, 90, 180 or 365 days, 30 by default), counted from the day it was turned on. Signing in with the passkey does not extend the period. If an administrator shortens the period, it applies right away to browsers already turned on, and a longer period applies to browsers turned on from then on. Changing the period requires a session that re-confirmed the password or passed two-step verification.
- **After changing your password**: the saved password no longer works, so signing in with the passkey in that browser reports the failure and removes the registration. Registrations in other browsers are removed when they are used there. To clean them up right away, turn them off on the Security page.
- **Turning it off**: the "Browsers that sign in with a passkey" list on the Security page shows each browser's passkey name, the day it was turned on, the last use and the expiry date, and the browser you are using now is marked "This browser". You can turn it off right away without confirming your password or a method again. Deleting that passkey, or an administrator resetting the user's methods, also removes it.
- If it expired or the registration was removed, sign in with your password and turn it on again.

## Copied browser profiles

At every passkey sign-in, the dashboard replaces the key share, and the browser encrypts the saved password again with the new share.

- A copy of the browser profile becomes unusable once the original browser signs in again. Trying to sign in with such a copy makes the dashboard remove the registration, so the original browser must also sign in with the password and turn it on again.
- Conversely, if the copy is used first, the original browser's next passkey sign-in fails because the saved password cannot be decrypted, and the registration is removed. If the original browser suddenly shows this error, someone may have used a copy, so change your password and check the devices that hold your passkey.

## Design

OMV has no passwordless sign-in such as tokens or SSO, and both the OMV session and the privileged helper require the password. So passkey sign-in keeps the password encrypted, decrypts it after the passkey check and sends it to OMV as usual, and protects the password by splitting the key in two.

- The password ciphertext lives only in the localStorage of the browser where the feature was turned on, and the dashboard keeps only the other share of the key in the sign-in method record. The decryption key can be made only from the passkey's PRF result together with this share, and it is never stored anywhere.
- The dashboard hands out the share only after the passkey check (with user verification required) passes. So every decryption of the password goes through the dashboard's logging and attempt limit.
- A leaked state directory or backup does not reveal the password from the share alone, and a leaked browser entry does not reveal it without the share and the passkey. Turning off the registration deletes the share, so the ciphertext left in the browser becomes useless too.

## Limitations

- Other web apps on the same origin can read the browser's localStorage entries. For example, if the dashboard is under a path prefix on the same domain as OMV, the OMV web UI is on the same origin. The entry cannot be decrypted without the share and the passkey, but to keep even the entry separate, give the dashboard its own host name.
- Clearing site data or using a private window leaves the browser without its entry, so the feature has to be turned on again.
- Even with a synced passkey such as one in iCloud Keychain, passkey sign-in works only in the browser where it was turned on. Turn it on separately on other devices.
- Someone who has both the passkey (authenticator) and its lock (PIN, biometrics) can sign in from that browser until the registration is turned off, and in the process the password is decrypted in the browser. This goes through the dashboard, so it is logged. If you lose a device, turn off that registration or delete the passkey on the Security page of another browser.
