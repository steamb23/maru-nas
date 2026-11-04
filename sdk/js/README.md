# @maru-nas/addon-sdk

SDK for [Maru NAS](https://github.com/steamb23/maru-nas) addon packages (`.maru-addon`). MIT licensed
([LICENSE](https://github.com/steamb23/maru-nas/blob/main/sdk/LICENSE)); the dashboard itself is GPL-3.0.

This package contains:

- `types/`: TypeScript types for the SDK runtime. The runtime itself (`globalThis.maru`) is served by the dashboard
  to every addon frame as `./_maru/sdk.js`, so it is not part of this package
- `schema/manifest.schema.json`: the JSON Schema of `manifest.json`, generated from the dashboard's own checks
- `cli/`: the `maru-addon` command (Node.js 20.19 or later, no dependencies)

## maru-addon

```sh
maru-addon init my-addon          # plain HTML and JavaScript, no build step
maru-addon init my-addon --ts     # TypeScript + Vite; the package directory is my-addon/addon
maru-addon validate my-addon      # check a package directory or a .maru-addon file
maru-addon pack my-addon          # check and pack into my-addon-0.1.0.maru-addon
```

`validate` and `pack` check the manifest against the schema and the file layout rules (allowed paths, size limits,
translations, icon, backend executables). The dashboard checks more when it installs a package (for example
conditions between fields and control characters in texts), so also load your package in a development dashboard
with `MARU_DEV_ADDONS`.

`pack` writes the same tar for the same files (sorted paths, zero timestamps and owners, fixed modes). The gzip
bytes, and so the SHA-256 shown in the dashboard, can still differ between machines with different zlib versions.

Until the SDK is published to npm, use it from a checkout of the Maru NAS repository:
`node sdk/js/cli/maru-addon.mjs <command>`. `init --ts` then points the project's `@maru-nas/addon-sdk`
dependency at that checkout.

The developer guide is in the documentation: [Addon development](https://github.com/steamb23/maru-nas/blob/main/docs/en/addon-dev.md).
