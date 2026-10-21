// 파일 탐색기 쓰기 요청의 보안 확인. 보안 강화 정책을 쓰기·삭제에만 적용하면 목록은 보이고 쓰기 요청만
// security_required로 막히므로, 창 전체를 잠그지 않고 확인 대화상자를 띄운 뒤 같은 요청을 한 번 다시 한다.
// 읽기까지 막는 정책에서는 목록부터 막혀 창이 확인 화면을 보인다(FilesWindow).
import { ApiError } from '../api/client'
import { requestStepUp } from '../security/stepUpDialog'
import type { DialogScope } from '../shell/dialogs'

// 여러 요청이 함께 막히면(올리기와 삭제 등) 확인 대화상자를 하나만 띄우고 그 결과를 함께 쓴다.
let pending: Promise<unknown> | null = null

// withStepUp은 fn을 하고, 보안 확인이 필요하다는 답이면 확인한 뒤 한 번 더 한다. 확인을 취소하면 원래 오류를 던진다.
export async function withStepUp<T>(scope: DialogScope, fn: () => Promise<T>): Promise<T> {
  try {
    return await fn()
  } catch (err) {
    if (!(err instanceof ApiError && err.code === 'security_required')) throw err
    // 확인하면 바뀐 보안 상태, 취소하면 null이다.
    pending ??= requestStepUp(scope, { level: err.required, target: 'files' }).finally(() => {
      pending = null
    })
    if (!(await pending)) throw err
    return fn()
  }
}
