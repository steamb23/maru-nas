// The window view. maru (the SDK runtime) is a global provided by the dashboard; its types come from
// @maru-nas/addon-sdk. app.data() asks the package's backend (backend/main.go) for its data, and app.action() runs
// an action the manifest declares.

// What the backend's data function returns.
interface BackendData {
  hostname: string
  requests: number
}

const app = await maru.connect()
const el = (id: string) => document.getElementById(id) as HTMLElement
el('title').textContent = app.t('title')
el('reset').textContent = app.t('reset')

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

async function load(): Promise<void> {
  try {
    const data = await app.data<BackendData>()
    el('host').textContent = app.t('host', { name: data.hostname })
    el('requests').textContent = app.t('requests', { count: data.requests })
  } catch (err) {
    // While the backend is not running the dashboard answers with code addon_backend_unavailable.
    el('host').textContent = app.t('failed', { message: message(err) })
  }
}

// The dashboard only accepts an action right after the user clicked this frame, so run it from the click handler.
el('reset').addEventListener('click', async () => {
  try {
    await app.action('reset')
  } catch (err) {
    el('host').textContent = app.t('failed', { message: message(err) })
  }
  await load()
})

await load()
