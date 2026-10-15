import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { MAX_IMAGE_BYTES, checkImageFile, uploadErrorKey } from './imageUpload'

describe('checkImageFile', () => {
  it('PNG·JPEG·WebP를 크기 제한 안에서 받는다', () => {
    expect(checkImageFile({ type: 'image/png', size: 10 })).toBeNull()
    expect(checkImageFile({ type: 'image/webp', size: MAX_IMAGE_BYTES })).toBeNull()
  })

  it('다른 형식이나 큰 파일은 문구 키를 준다', () => {
    expect(checkImageFile({ type: 'image/svg+xml', size: 10 })).toBe('settings.image.unsupported')
    expect(checkImageFile({ type: '', size: 10 })).toBe('settings.image.unsupported')
    expect(checkImageFile({ type: 'image/jpeg', size: MAX_IMAGE_BYTES + 1 })).toBe('settings.image.tooLarge')
  })
})

describe('uploadErrorKey', () => {
  it('백엔드 오류 코드에 맞는 문구다', () => {
    expect(uploadErrorKey(new ApiError(413, 'x', 'image_too_large'))).toBe('settings.image.tooLarge')
    expect(uploadErrorKey(new ApiError(415, 'x', 'unsupported_image'))).toBe('settings.image.unsupported')
    expect(uploadErrorKey(new ApiError(500, 'x'))).toBe('settings.image.failed')
    expect(uploadErrorKey(new Error('network'))).toBe('settings.image.failed')
  })
})
