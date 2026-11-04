// 예제 창: 선언한 OMV 읽기(System.getInformation), 사용자별 저장(storage), 호스트 확인 대화상자(confirm),
// 알림(notify), 워크벤치 열기(openWorkbench)를 보인다. 문구는 모두 패키지 번역(app.t)이다.
const app = await maru.connect()
const $ = (id) => document.getElementById(id)

let info = null
let saved = (await app.storage.get()) ?? {}

// render는 화면 문구를 지금 언어로 다시 쓴다. 언어가 바뀌면 'context' 이벤트로 다시 부른다.
function render() {
  $('title').textContent = app.t('title')
  $('host').textContent = info ? app.t('host', { name: info.hostname }) : app.t('hostUnknown')
  // 버전·가동 시간은 관리자에게만 온다(OMV가 일반 사용자에게는 시각과 호스트 이름만 준다).
  const system = $('system')
  system.hidden = !(info && info.version)
  if (info && info.version) {
    system.textContent = app.t('system', { version: info.version, hours: Math.floor((info.uptime ?? 0) / 3600) })
  }
  $('greeting-title').textContent = app.t('greeting')
  $('greeting-hint').textContent = app.t('greetingHint')
  $('name').placeholder = app.t('namePlaceholder')
  $('save').textContent = app.t('save')
  $('clear').textContent = app.t('clear')
  $('workbench').textContent = app.t('workbench')
}

async function loadInfo() {
  try {
    info = await app.rpc('System', 'getInformation')
  } catch {
    info = null
  }
  render()
}

$('name').value = saved.name ?? ''
render()
loadInfo()
app.on('context', render)

async function save() {
  const name = $('name').value.trim()
  if (!name) return
  saved = { name }
  await app.storage.set(saved)
  await app.notify('success', 'saved', { name })
}

$('save').addEventListener('click', save)
$('name').addEventListener('keydown', (e) => {
  if (e.key === 'Enter') save()
})

$('clear').addEventListener('click', async () => {
  if (!(await app.confirm({ message: app.t('clearConfirm'), confirmLabel: app.t('clear'), danger: true }))) return
  saved = {}
  await app.storage.set(saved)
  $('name').value = ''
  await app.notify('info', 'cleared')
})

// 워크벤치 열기는 사용자가 이 프레임을 방금 눌렀을 때만 호스트가 받는다(단추의 click 안에서 부른다).
$('workbench').addEventListener('click', () => {
  app.openWorkbench('dashboard').catch(() => {})
})
