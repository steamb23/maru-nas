// 시험 도구 창: 백엔드의 조작 run(app.query)을 실행해 결과 표를 그린다. 문구는 패키지 번역(app.t)이다.
// 네트워크 권한 칸은 이 패키지 manifest의 backend.network와 같게 고른다(기대 결과를 정한다).
const app = await maru.connect()
const $ = (id) => document.getElementById(id)

let report = null
let error = null
let busy = false

function cell(text, className) {
  const td = document.createElement('td')
  td.textContent = text
  if (className) td.className = className
  return td
}

function render() {
  $('notice').textContent = app.t('notice')
  $('hint').textContent = app.t('hint')
  $('network-label').textContent = app.t('network')
  $('lan-label').textContent = app.t('lanAddress')
  $('run').textContent = app.t(busy ? 'running' : 'run')
  $('run').disabled = busy
  for (const k of ['check', 'target', 'result', 'class', 'expect', 'verdict']) $(`col-${k}`).textContent = app.t(`col.${k}`)

  const banner = $('error')
  banner.hidden = !error
  if (error) banner.textContent = app.t('failed', { message: error })

  $('summary').hidden = !report
  $('table').hidden = !report
  const body = $('rows')
  body.replaceChildren()
  if (!report) return
  $('summary').textContent = app.t('summary', { passed: report.passed, failed: report.failed })
  for (const r of report.rows) {
    const tr = document.createElement('tr')
    tr.append(
      cell(r.check),
      cell(r.target, 'target'),
      cell(r.result),
      cell(r.class),
      cell(r.expect ?? ''),
      cell(app.t(`verdict.${r.verdict}`), r.verdict),
    )
    body.append(tr)
  }
}

$('run').addEventListener('click', async () => {
  if (busy) return
  busy = true
  error = null
  render()
  const network = []
  if ($('net-lan').checked) network.push('lan')
  if ($('net-loopback').checked) network.push('loopback')
  const lanAddress = $('lan-address').value.trim()
  try {
    report = await app.query('run', { input: { network, lanAddress } })
  } catch (err) {
    if (err?.code !== 'bridge_cancelled') error = err?.message ?? String(err)
  }
  busy = false
  render()
})

app.on('context', render)
render()
