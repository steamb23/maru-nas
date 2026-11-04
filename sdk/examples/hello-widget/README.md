# Hello widget

An example Maru NAS addon package in plain HTML and JavaScript (no build step). It shows how to:

- call an OMV read method declared in `manifest.json` (`System.getInformation`)
- keep a small per-user value with `app.storage`
- ask with the dashboard's confirmation dialog (`app.confirm`) and send a notification (`app.notify`)
- open an OMV workbench page from a button (`app.openWorkbench`)
- show the same data as a desktop widget, and pause while the widget is not visible (`app.context.active`)

Try it from the repository root with `FAKEADDONS=examples make dev-fake`, then open "Hello widget" from All apps or
add its widget from the desktop's right-click menu. Pack it with
`node sdk/js/cli/maru-addon.mjs pack sdk/examples/hello-widget`.
