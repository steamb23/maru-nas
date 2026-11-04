// The window view. maru (the SDK runtime) is a global provided by the dashboard; its types come from
// @maru-nas/addon-sdk.

// The part of System.getInformation this view uses.
interface SystemInformation {
  hostname: string
}

const app = await maru.connect()
const title = document.getElementById('title') as HTMLElement
const host = document.getElementById('host') as HTMLElement
title.textContent = app.t('title')

try {
  // Only the methods listed in addon/manifest.json (rpc.read) can be called.
  const info = await app.rpc<SystemInformation>('System', 'getInformation')
  host.textContent = app.t('host', { name: info.hostname })
} catch (err) {
  host.textContent = err instanceof Error ? err.message : String(err)
}
