// 배경 이미지 올리기 검사와 오류 문구. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 형식·크기는 백엔드가 다시 검사한다(server/images.go). 여기서는 보내기 전에 바로 알려 주는 용도다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'

// 백엔드 maxImageBytes와 맞춘다.
export const MAX_IMAGE_BYTES = 20 * 1024 * 1024

export const ACCEPTED_IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/webp'] as const

// checkImageFile은 올리기 전에 형식과 크기를 본다. 문제없으면 null, 있으면 문구 키다.
export function checkImageFile(file: { type: string; size: number }): ParseKeys | null {
  if (!(ACCEPTED_IMAGE_TYPES as readonly string[]).includes(file.type)) return 'settings.image.unsupported'
  if (file.size > MAX_IMAGE_BYTES) return 'settings.image.tooLarge'
  return null
}

// uploadErrorKey는 올리기·지우기 실패를 문구 키로 바꾼다.
export function uploadErrorKey(err: unknown): ParseKeys {
  if (err instanceof ApiError) {
    if (err.code === 'image_too_large') return 'settings.image.tooLarge'
    if (err.code === 'unsupported_image') return 'settings.image.unsupported'
  }
  return 'settings.image.failed'
}
