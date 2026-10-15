// 프로필 사진 바꾸기: 고른 파일을 확인하고 자르기 편집기(AvatarEditor)를 창 범위 대화상자로 연다.
import type { ParseKeys } from 'i18next'
import type { ImageInfo } from '../api/types.gen'
import type { Dialogs } from '../shell/dialogPresets'
import { AVATAR_EDITOR_VIEW, AvatarEditor } from './AvatarEditor'
import { checkImageFile } from './imageUpload'

// editAvatar는 고른 파일을 확인하고 자르기 편집기를 연다. 파일을 쓸 수 없으면 문구 키를 돌려준다.
export async function editAvatar(dialogs: Dialogs, file: File, title: string): Promise<ParseKeys | null> {
  const bad = checkImageFile(file)
  if (bad) return bad
  let bitmap: ImageBitmap
  try {
    // 사진의 회전 정보(EXIF)를 반영해 읽는다.
    bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
  } catch {
    return 'settings.image.unsupported'
  }
  try {
    await dialogs.open<ImageInfo>({
      title,
      size: { width: AVATAR_EDITOR_VIEW + 34 },
      render: (ctl) => <AvatarEditor bitmap={bitmap} onDone={(info) => (info ? ctl.close(info) : ctl.cancel())} />,
    })
  } finally {
    bitmap.close()
  }
  return null
}
