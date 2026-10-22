# Contributing to Maru NAS

한국어: [CONTRIBUTING.ko.md](CONTRIBUTING.ko.md)

Thank you for your interest in Maru NAS, a desktop-like web dashboard for openmediavault 8. This document explains what you need to know before opening an issue or a pull request. Design rules for each area live in `CLAUDE.md` and `.claude/rules/`; they are written in Korean, so if something there is unclear, ask in an issue.

## Scope

- Maru NAS shows status and handles simple operations. Deep configuration stays in the OMV web UI (workbench). We do not rebuild OMV screens; we link to them.
- Local network only. We do not add features or settings that assume exposure to the internet.
- All OMV 8.x releases are supported. A new RPC call must exist in the OMV 8.0 sources as well.
- `ROADMAP.md` lists planned features. Open an issue before starting one so we can agree on the design first.

## Development setup

Tool versions and installation steps are in the "빌드 및 설치" section of `README.md`. In short: Go 1.27.1 or later, Node.js 22.12 or later, git, make.

```sh
git clone https://github.com/steamb23/maru-nas.git
cd maru-nas
git submodule update --init --depth 1
npm --prefix frontend ci
make dev-fake
```

`make dev-fake` starts the backend, the Vite dev server and a fake OMV, so you can develop without touching a real OMV. Open `http://<host>:5173/` and sign in with one of the fake accounts.

| Account | Password | Role | Note |
|---|---|---|---|
| `admin` | `admin` | administrator | |
| `user` | `user` | regular user | |
| `totp` | `totp` | administrator | second factor code `123456` |

To test against a real OMV, put `MARU_OMV_URL=<your OMV address>` in `frontend/.env.local` and run `make dev`. Never commit that file.

Useful commands (run from the repository root):

| Command | What it does |
|---|---|
| `make check` | lint (`gofmt`, `go vet`, oxlint, `tsc`) and tests (`go test`, Vitest) |
| `make build` | production build into `bin/maru`, the privileged helper `bin/maru-helper` and the installer `bin/maru-install` |
| `make types` | regenerate TypeScript from Go structs and addon manifests |

## Branches and pull requests

- Work on a branch from `main` named `<type>/<short-description>`, lowercase with hyphens. Types: `feat` (new feature or app), `fix` (bug fix), `docs`, `i18n` (translations), `chore` (build, CI, cleanup), `refactor` (no behavior change).
- Fork the repository and open a pull request against `main`. One change per pull request.
- Pull requests are rebased onto `main` without merge commits. Force-pushing your own pull request branch is fine.
- CI runs `make check` and `make build` on every pull request and must pass. Run `make check` locally before pushing.
- `main` is the development branch. Releases are tagged `v<version>` and the `stable` branch always points at the latest release.

## Commit messages and language

- Write commit messages and pull request descriptions in English or Korean. Other languages are accepted, but one of those two helps review.
- The maintainer writes commits and code comments in Korean, so you will see Korean throughout the history and the code. You may write your own comments in English or Korean.
- The first line of a commit message says what changed and why, in one sentence.

## Code rules

- The backend uses the Go standard library, plus `golang.org/x/crypto/ssh` and `github.com/coder/websocket` for the web terminal only. The privileged helper links neither. The installer (`installer/`, a separate Go module run by `install.sh`) uses `charm.land/huh/v2` for its terminal UI; that dependency lives only in `installer/go.mod`, and the installer uses nothing from the backend except `backend/config`. The frontend is TypeScript, Vite, React and Zustand, plus xterm.js for the web terminal and uqr for the authenticator-app QR code.
- Before adding a dependency, explain in the pull request why it is needed and what the alternatives were.
- If you change a struct under `backend/internal/api` or an addon manifest, run `make types` and commit the generated files (`frontend/src/api/types.gen.ts`, `frontend/src/addons/manifests.gen.ts`). Tests check that they are up to date.
- Identifiers, log messages and error messages stay in English.
- Every string shown on screen, including `aria-label` and `title`, goes through `t()` and gets a key in both `en.json` and `ko.json`. If you do not write Korean, put the English text into `ko.json` under the same key and say so in the pull request; the maintainer translates it before merging.

## Privileges and security

Maru NAS runs as an unprivileged system user and must stay that way.

- The service process never escalates privileges. Information and changes that need root go through the OMV RPC API, using the signed-in user's own OMV session.
- Do not add code that calls `omv-rpc`, connects to the OMV engine socket or reads the Docker or Podman socket directly. We do not accept pull requests that add the service user to the `docker` or `openmediavault-engined` group.
- Privileged work that OMV RPC cannot do (a user's own files, root operations) goes only through the privileged helper (`backend/cmd/maru-helper`), as typed operations from its fixed list. There is no generic command execution. Adding a root operation must be discussed in an issue first.
- Never put passwords, OMV session IDs or tokens in logs, test fixtures or commits. Authenticator-app secrets and recovery codes of the security hardening mode count as secrets too.
- Test fixtures (`testdata/` and the like) contain synthetic data only. Replace disk serials, MAC and IP addresses, host names and user names with fake values.
- Personal environment details (domains, IP addresses) belong in `*.local.md` files, which are ignored by git. Do not hardcode them in committed files.

## Translations

Translations are the easiest way to contribute.

- Catalogs are in `frontend/src/i18n/locales/<language>.json`. `en` and `ko` are maintained by the maintainer and are always complete; every other language may be partially translated, and missing keys fall back to English.
- Translate from `en.json`. Do not translate product and technology names (OpenMediaVault, OMV, SMART, SSH, SMB/CIFS, NFS, systemd, Go), paths, commands, units or example domains.
- Plural forms use the i18next v4 suffixes (`key_one`, `key_other` and so on). The set of categories depends on the language. If you translate a plural key, include every category your language uses.
- Terminology and tone per language are listed in the "번역 기준" table of `.claude/rules/i18n.md`.
- Check your work with the catalog tests:

  ```sh
  cd frontend && npx vitest run src/i18n
  ```

- To add a language, create `locales/<code>.json` (BCP 47 tag, for example `pt-PT` or `zh-Hant`) and add it to `SUPPORTED_LANGUAGES` in `frontend/src/i18n/languages.ts`.
- Keep JSON escapes such as the soft hyphen as escapes in the file. Do not paste the invisible character itself; the catalog test reports such characters with their line number.

## Icons and third-party assets

- Icons come from the Colloid theme only and are referenced by freedesktop name through `getIcon(name)`. Do not add other icon themes or inline icon paths.
- A third-party asset (for example a brand logo) is accepted only when its license is compatible with GPL-3.0. Put the unmodified original under `vendor/icons/<source>/` with a `LICENSE` or `NOTICE` file, add a section to `vendor/icons/README.md` and a source table under `frontend/src/icons/sources/`. Details are in `.claude/rules/icons.md`.

## License

Maru NAS is licensed under the GNU General Public License v3.0 (see `LICENSE`). By contributing, you agree that your contribution is licensed under the same terms.
