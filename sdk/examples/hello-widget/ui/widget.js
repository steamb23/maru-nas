// 예제 위젯: 저장한 인사말과 호스트 이름. 위젯은 누를 수 없고(누르면 창이 열린다) 읽기 요청만 쓸 수 있다.
const app = await maru.connect()
const $ = (id) => document.getElementById(id)

let host = ''
let name = ''

function render() {
  $('hello').textContent = name ? app.t('helloName', { name }) : app.t('hello')
  $('host').textContent = host
}

async function refresh() {
  // 보이지 않는 동안(창 최소화, 탭 숨김)은 요청하지 않는다.
  if (!app.context.active) return
  const [saved, info] = await Promise.all([app.storage.get().catch(() => null), app.rpc('System', 'getInformation').catch(() => null)])
  name = saved?.name ?? ''
  host = info?.hostname ?? ''
  render()
}

app.on('context', () => {
  render()
  refresh()
})
refresh()
setInterval(refresh, 60_000)
