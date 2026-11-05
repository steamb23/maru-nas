// 예제 창: 패키지 백엔드의 데이터(app.data)를 보이고, 관리자는 조작(app.action)으로 OMV 성능 통계를 끄고 켠다.
// 백엔드는 sdk/go/examples/hello-backend다. 문구는 모두 패키지 번역(app.t)이다.
const app = await maru.connect()
const $ = (id) => document.getElementById(id)

let data = null
// error는 마지막 실패의 번역 키와 인수다(언어가 바뀌면 다시 그린다).
let error = null
let busy = false

function render() {
  $('host-title').textContent = app.t('hostTitle')
  $('host').textContent = data ? app.t('host', { name: data.hostname }) : app.t('loading')
  const version = $('version')
  version.hidden = !data?.version
  if (data?.version) version.textContent = app.t('version', { version: data.version })

  $('requests-title').textContent = app.t('requestsTitle')
  $('requests').textContent = data ? String(data.dataRequests) : ''
  $('requests-hint').textContent = app.t(data?.persistent ? 'requestsKept' : 'requestsMemory')

  // 성능 통계는 백엔드가 관리자에게만 준다.
  const perf = data?.perfStats
  $('perf').hidden = !perf
  if (perf) {
    $('perf-title').textContent = app.t('perfTitle')
    $('perf-state').textContent = app.t(perf.enable ? 'on' : 'off')
    $('perf-hint').textContent = app.t('perfHint')
    const toggle = $('toggle')
    toggle.textContent = app.t(perf.enable ? 'turnOff' : 'turnOn')
    toggle.disabled = busy
    const last = $('last-change')
    last.hidden = !data.lastChange
    if (data.lastChange) {
      const at = new Date(data.lastChange.at).toLocaleString(app.context.language)
      last.textContent = app.t(data.lastChange.enable ? 'lastOn' : 'lastOff', { user: data.lastChange.user, at })
    }
  }

  const banner = $('error')
  banner.hidden = !error
  if (error) banner.textContent = app.t(error.key, error.params)
  $('reload').textContent = app.t('reload')
  $('reload').disabled = busy
}

// fail은 실패를 문구로 바꾼다. 백엔드가 준 코드(addonCode)는 이 패키지가 정한 것이라 번역 키로 고른다.
// security_required는 사용자가 호스트의 본인 확인 창을 취소했을 때만 오므로 확인 취소(bridge_cancelled)처럼 넘긴다.
function fail(err) {
  if (err?.code === 'bridge_cancelled' || err?.code === 'security_required') return null
  if (err?.addonCode === 'config_dirty') return { key: 'configDirty' }
  if (err?.code === 'addon_backend_unavailable' || err?.code === 'addon_backend_disabled') return { key: 'backendDown' }
  return { key: 'failed', params: { message: err?.message ?? String(err) } }
}

// load는 데이터를 다시 읽는다. after는 방금 실패한 조작의 오류다(데이터를 읽어도 지우지 않는다).
async function load(after = null) {
  try {
    data = await app.data()
    error = after
  } catch (err) {
    error = after ?? fail(err)
  }
  render()
}

// 조작은 사용자가 이 프레임을 방금 눌렀을 때만 호스트가 받으므로 click 안에서 부른다. manifest가 확인(confirm)을
// 요구하므로 호스트가 먼저 묻고, 쓰기 패키지라 비밀번호 재확인도 호스트가 받는다.
$('toggle').addEventListener('click', async () => {
  if (!data?.perfStats || busy) return
  busy = true
  error = null
  render()
  let failed = null
  try {
    await app.action('perfstats', { input: { enable: !data.perfStats.enable } })
  } catch (err) {
    failed = fail(err)
  }
  busy = false
  await load(failed)
})

$('reload').addEventListener('click', () => load())
app.on('context', render)
render()
load()
