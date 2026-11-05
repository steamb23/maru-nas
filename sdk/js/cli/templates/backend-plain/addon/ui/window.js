// The window view. app.data() asks the package's backend (backend/main.go) for its data, and app.action() runs an
// action the manifest declares.
const app = await maru.connect()
const $ = (id) => document.getElementById(id)
$('title').textContent = app.t('title')
$('reset').textContent = app.t('reset')

async function load() {
  try {
    const data = await app.data()
    $('host').textContent = app.t('host', { name: data.hostname })
    $('requests').textContent = app.t('requests', { count: data.requests })
  } catch (err) {
    // While the backend is not running the dashboard answers with code addon_backend_unavailable.
    $('host').textContent = app.t('failed', { message: err.message })
  }
}

// The dashboard only accepts an action right after the user clicked this frame, so run it from the click handler.
$('reset').addEventListener('click', async () => {
  try {
    await app.action('reset')
  } catch (err) {
    $('host').textContent = app.t('failed', { message: err.message })
  }
  load()
})

load()
