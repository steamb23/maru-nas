// 배경 이미지 올리기·지우기의 진행 상태와 오류 문구. 사용자 화면 페이지와 관리자 페이지가 함께 쓴다.
import type { ParseKeys } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ImageInfo } from '../api/types.gen'
import { useDialogs } from '../shell/dialogPresets'
import { checkImageFile, uploadErrorKey } from './imageUpload'

export function useImageActions() {
  const { t } = useTranslation()
  const dialogs = useDialogs()
  const [busy, setBusy] = useState(false)
  const [errorKey, setErrorKey] = useState<ParseKeys | null>(null)

  // upload는 형식·크기를 먼저 보고 send로 올린 뒤 done에 결과를 넘긴다.
  const upload = async (file: File, send: (file: File) => Promise<ImageInfo>, done: (info: ImageInfo) => void) => {
    const bad = checkImageFile(file)
    if (bad) {
      setErrorKey(bad)
      return
    }
    setBusy(true)
    setErrorKey(null)
    try {
      done(await send(file))
    } catch (err) {
      setErrorKey(uploadErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  // remove는 한 번 물은 뒤 send로 지우고 done을 부른다. 묻는 문구는 text로 바꿀 수 있다(기본은 배경 이미지).
  const remove = async (
    send: () => Promise<void>,
    done: () => void,
    text: { title: ParseKeys; message: ParseKeys } = {
      title: 'settings.image.delete',
      message: 'settings.image.deleteConfirm',
    },
  ) => {
    const ok = await dialogs.confirm({
      title: t(text.title),
      message: t(text.message),
      confirmLabel: t('settings.image.deleteButton'),
      danger: true,
    })
    if (!ok) return
    setBusy(true)
    setErrorKey(null)
    try {
      await send()
      done()
    } catch (err) {
      setErrorKey(uploadErrorKey(err))
    } finally {
      setBusy(false)
    }
  }

  return { busy, errorKey, upload, remove }
}
