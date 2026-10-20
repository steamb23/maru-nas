// 창 하나의 올리기 목록. 고른 파일의 이름과 충돌을 먼저 확인하고(같은 이름이면 덮어쓰기·건너뛰기를 묻는다) 하나씩
// 차례로 올린다. 올리는 동안은 세션 활동으로 알리고(입력 없이도 로그아웃되지 않게) 페이지를 떠나려 하면 경고한다.
// 창을 닫으면(언마운트) 그 창의 올리기를 취소한다.
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, fetchFileInfo, uploadFile } from '../api/client'
import { reportActivity } from '../auth/activity'
import type { Dialogs } from '../shell/dialogPresets'
import { askConflict } from './conflictDialog'
import { uploadError } from './messages'
import { joinPath, uploadNameOk } from './paths'
import {
  type NewUpload,
  type UploadState,
  active,
  canceled,
  clearFinished,
  emptyUploads,
  enqueue,
  failed,
  finished,
  nextQueued,
  progressed,
  retried,
  started,
} from './uploadQueue'

// ACTIVITY_MS는 올리는 동안 세션 활동을 알리는 간격이다(세션 유지기가 최대 60초에 한 번 연장한다).
const ACTIVITY_MS = 30_000

export interface Uploads {
  state: UploadState
  add: (dir: string, files: readonly File[]) => Promise<void>
  cancel: (id: number) => void
  retry: (id: number, overwrite?: boolean) => void
  clear: () => void
}

export function useUploads(dialogs: Dialogs, onUploaded: (dir: string) => void): Uploads {
  const { t } = useTranslation()
  const [state, setState] = useState<UploadState>(emptyUploads)
  const files = useRef(new Map<number, File>())
  const nextId = useRef(1)
  const controllers = useRef(new Map<number, AbortController>())
  const uploadedRef = useRef(onUploaded)
  useLayoutEffect(() => {
    uploadedRef.current = onUploaded
  })

  const add = useCallback(
    async (dir: string, picked: readonly File[]) => {
      const list: NewUpload[] = []
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
          if (err instanceof ApiError && err.status === 401) return
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
        const id = nextId.current++
        list.push({ id, dir, name: f.name, size: f.size, overwrite })
        files.current.set(id, f)
      }
      if (bad.length > 0) {
        await dialogs.alert({
          title: t('files.upload.title'),
          message: bad.map((name) => t('files.upload.badName', { name })).join('\n'),
        })
      }
      if (list.length === 0) return
      setState((s) => enqueue(s, list))
    },
    [dialogs, t],
  )

  // 다음 파일을 올린다(하나씩).
  useEffect(() => {
    const next = nextQueued(state)
    if (!next) return
    const file = files.current.get(next.id)
    if (!file) {
      setState((s) => failed(s, next.id, 'unknown'))
      return
    }
    const ctl = new AbortController()
    controllers.current.set(next.id, ctl)
    setState((s) => started(s, next.id, Date.now()))
    let lastActivity = 0
    uploadFile(joinPath(next.dir, next.name), file, {
      overwrite: next.overwrite,
      signal: ctl.signal,
      onProgress: (sent) => {
        setState((s) => progressed(s, next.id, sent))
        const now = Date.now()
        if (now - lastActivity >= ACTIVITY_MS) {
          lastActivity = now
          reportActivity()
        }
      },
    })
      .then(
        () => {
          setState((s) => finished(s, next.id))
          files.current.delete(next.id)
          uploadedRef.current(next.dir)
        },
        (err: unknown) => {
          if (err instanceof DOMException && err.name === 'AbortError') setState((s) => canceled(s, next.id))
          else setState((s) => failed(s, next.id, uploadError(err)))
        },
      )
      .finally(() => controllers.current.delete(next.id))
  }, [state])

  // 창을 닫으면 그 창의 올리기를 취소한다.
  useEffect(() => {
    const ctls = controllers.current
    return () => {
      for (const c of ctls.values()) c.abort()
    }
  }, [])

  // 올리는 중에 페이지를 떠나려 하면 브라우저가 묻게 한다.
  const busy = active(state) > 0
  useEffect(() => {
    if (!busy) return
    const onBeforeUnload = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => window.removeEventListener('beforeunload', onBeforeUnload)
  }, [busy])

  const cancel = useCallback((id: number) => {
    controllers.current.get(id)?.abort()
    setState((s) => canceled(s, id))
  }, [])
  const retry = useCallback((id: number, overwrite?: boolean) => setState((s) => retried(s, id, overwrite)), [])
  const clear = useCallback(() => {
    setState((s) => {
      const next = clearFinished(s)
      const keep = new Set(next.items.map((i) => i.id))
      for (const id of files.current.keys()) if (!keep.has(id)) files.current.delete(id)
      return next
    })
  }, [])

  return { state, add, cancel, retry, clear }
}
