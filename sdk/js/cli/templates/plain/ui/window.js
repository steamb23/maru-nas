// The window view. maru.connect() waits for the dashboard and returns the app (context, t, rpc, storage, ...).
const app = await maru.connect()
document.getElementById('title').textContent = app.t('title')

try {
  // Only the methods listed in manifest.json (rpc.read) can be called.
  const info = await app.rpc('System', 'getInformation')
  document.getElementById('host').textContent = app.t('host', { name: info.hostname })
} catch (err) {
  document.getElementById('host').textContent = err.message
}
