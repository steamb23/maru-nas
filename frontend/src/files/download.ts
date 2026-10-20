// 내려받기. 브라우저의 내려받기 관리자가 받도록 숨긴 <a download>를 누른다(같은 사이트 이동이라 세션 쿠키가 간다).
// 내려받기 응답이 401이면 브라우저가 오류 본문을 파일로 저장하고 로그인 화면으로 가지도 않으므로, 누르기 직전에
// 정보를 받아 세션과 파일을 먼저 확인한다(401이면 API 클라이언트가 로그인 화면으로 보낸다).
import { ApiError, fetchFileInfo, fileDownloadUrl } from '../api/client'

export async function startDownload(path: string, name: string): Promise<void> {
  const info = await fetchFileInfo(path)
  if (info.type !== 'file' && !(info.type === 'symlink' && info.linkType === 'file')) {
    throw new ApiError(400, `${path}: not a regular file`, 'not_regular')
  }
  if (info.access && !info.access.read) throw new ApiError(403, `${path}: not readable`, 'permission_denied')
  const a = document.createElement('a')
  a.href = fileDownloadUrl(path)
  a.download = name
  a.style.display = 'none'
  document.body.append(a)
  a.click()
  a.remove()
}
