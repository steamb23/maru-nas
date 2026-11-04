# Addon development

You can build addons outside the Maru NAS repository and distribute them as package files (`.maru-addon`). A package consists of window and widget screens made with HTML and JavaScript, and a `manifest.json` that declares the OMV read methods those screens call. Administrators install packages by uploading the file in the Settings app ([Addon packages](addons.md)).

The SDK (`sdk/`) is MIT licensed. The dashboard itself is GPL-3.0, but you can distribute packages under any license.

## How it works

- Package screens run in a sandboxed iframe inside the dashboard. They cannot reach the dashboard's cookies, storage or screens, and cannot send network requests (fetch and so on) themselves.
- Screens make requests to the dashboard through the SDK runtime the dashboard puts in the frame (`./_maru/sdk.js`, the global `maru`). The dashboard makes the call with the signed-in user's session and returns the result.
- Only the OMV RPC methods declared in `rpc.read` of `manifest.json` can be called. They run with the signed-in user's OMV permissions, so regular users only get what OMV gives regular users.
- A small value (JSON up to 64 KiB) can be saved per user.
- This version installs packages that only have screens. Backends that run on the server and their actions (OMV writes) come in a later version.

## Requirements

- A checkout of the Maru NAS repository (to run the development server). The SDK is not published to npm yet, so the CLI is also used from the checkout
- Node.js 20.19 or later (CLI), and npm for TypeScript packages
- The development server's build tools (Go, Node.js) are described in [CONTRIBUTING.md](https://github.com/steamb23/maru-nas/blob/main/CONTRIBUTING.md)

## Quick start

Run these from the repository root.

1. Create a new package. This makes a plain HTML and JavaScript package with no build step (`--ts` makes a TypeScript + Vite project).

    ```sh
    node sdk/js/cli/maru-addon.mjs init ~/addons/my-addon
    ```

2. Start the development server with a fake OMV and load the directory as a development package. Use an absolute path.

    ```sh
    MARU_DEV_ADDONS=$HOME/addons/my-addon make dev-fake
    ```

3. Open `http://<host>:5173/`, sign in with the fake account `admin` (password `admin`) and open "My Addon" from "All apps". You can also add its widget from the desktop's right-click menu.
4. After you edit files, reload the page to see the changes. What the dashboard checks (manifest errors and so on) is shown on the "Addons" page of the Settings app.
5. When you are done, check and pack it. This writes `my-addon-0.1.0.maru-addon` and prints its SHA-256.

    ```sh
    node sdk/js/cli/maru-addon.mjs pack ~/addons/my-addon
    ```

Start the examples in the repository with `FAKEADDONS=examples make dev-fake`. [hello-widget](https://github.com/steamb23/maru-nas/tree/main/sdk/examples/hello-widget) shows OMV reads, per-user storage, confirmation dialogs, notifications, opening the workbench and a widget.

## Development server

- `MARU_DEV_ADDONS` is a colon-separated list of package directories, used only by the development server (`make dev`, `make dev-fake`). If an installed package has the same id, the development package wins.
- A development package counts as a new version whenever its files change, and open windows and widgets reload the next time the dashboard fetches the app list (for example when you reload the page).
- A `dev/` directory in the package directory is for development files and is not packed.
- `make dev-fake` connects to a fake OMV that imitates only some methods (such as `System.getInformation`). Check plugin methods with `make dev`, which connects to the real OMV.
- The fake accounts are `admin` (administrator), `user` (regular user) and `totp` (administrator, second-factor code `123456`). Check the regular user's view too.

## Documents

| Document | Contents |
|---|---|
| [Manifest](addon-manifest.md) | Every field of `manifest.json` and its rules |
| [Building screens](addon-frontend.md) | The entry HTML, the SDK API, windows and widgets, frame restrictions, TypeScript + Vite |
| [Packing and distribution](addon-packaging.md) | Package file layout and limits, the `maru-addon` CLI, versions and updates |
| [Addon security](addon-security.md) | What the dashboard prevents and what it cannot, methods that are not allowed, what authors should do |
