// The desktop widget. Widgets cannot be clicked; clicking one opens the window view.
const app = await maru.connect()
document.getElementById('label').textContent = app.t('widget')

async function refresh() {
  if (!app.context.active) return
  const info = await app.rpc('System', 'getInformation')
  document.getElementById('value').textContent = info.hostname
}

refresh()
setInterval(refresh, 30_000)
