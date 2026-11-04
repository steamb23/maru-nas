// 호스트 확인용 최소 창: 번역 문구와 System.getInformation의 호스트 이름을 보인다.
const app = await maru.connect()
document.getElementById('hello').textContent = app.t('hello')
const info = await app.rpc('System', 'getInformation')
document.getElementById('host').textContent = app.t('host', { name: info.hostname })
