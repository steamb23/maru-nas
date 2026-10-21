// 올리기 전 확인. 고른(끌어 놓은) 파일마다 이름을 검사하고 같은 이름이 있는지 먼저 확인해(있으면 덮어쓰기·건너뛰기를
// 묻는다, 남은 파일에 같게 적용할 수 있다) 올릴 목록을 만든다. 올리기는 파일 작업 목록(transferStore.ts)이 한다.
import type { TFunction } from 'i18next'
import { ApiError, fetchFileInfo } from '../api/client'
import type { Dialogs } from '../shell/dialogPresets'
import { askConflict } from './conflictDialog'
import { joinPath, uploadNameOk } from './paths'
import type { NewUpload } from './uploadJobs'

// checkUploads는 dir에 올릴 파일과 덮어쓸지를 돌려준다. 이름을 쓸 수 없는 파일은 알리고 뺀다. 세션이 끝났거나(401)
// 충돌 대화상자를 닫으면 그때까지 고른 파일만 돌려준다(401이면 아무것도 올리지 않는다).
export async function checkUploads(
  dialogs: Dialogs,
  t: TFunction,
  dir: string,
  picked: readonly File[],
): Promise<(NewUpload & { file: File })[]> {
  const list: (NewUpload & { file: File })[] = []
  const bad: string[] = []
  let all: 'overwrite' | 'skip' | null = null
  for (let k = 0; k < picked.length; k++) {
    const f = picked[k]
    if (!uploadNameOk(f.name)) {
      bad.push(f.name)
      continue
    }
    let exists = false
    try {
      await fetchFileInfo(joinPath(dir, f.name))
      exists = true
    } catch (err) {
      // 없으면(not_found) 그대로 올린다. 그 밖의 오류는 올릴 때 다시 알린다.
      if (err instanceof ApiError && err.status === 401) return []
    }
    let overwrite = false
    if (exists) {
      let action = all
      if (!action) {
        const choice = await askConflict(dialogs.scope, f.name, k < picked.length - 1)
        if (!choice) break
        action = choice.action
        if (choice.all) all = choice.action
      }
      if (action === 'skip') continue
      overwrite = true
    }
    list.push({ name: f.name, size: f.size, overwrite, file: f })
  }
  if (bad.length > 0) {
    await dialogs.alert({
      title: t('files.upload.title'),
      message: bad.map((name) => t('files.upload.badName', { name })).join('\n'),
    })
  }
  return list
}
