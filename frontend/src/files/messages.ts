// 파일 탐색기의 오류 문구 키. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'
import type { UploadItem } from './uploadQueue'

const ERROR_KEYS: Partial<Record<string, ParseKeys>> = {
  not_found: 'files.errors.not_found',
  permission_denied: 'files.errors.permission_denied',
  outside_roots: 'files.errors.outside_roots',
  not_dir: 'files.errors.not_dir',
  is_dir: 'files.errors.not_regular',
  not_regular: 'files.errors.not_regular',
  helper_unavailable: 'files.errors.helper_unavailable',
  helper_reauth: 'files.errors.helper_reauth',
  helper_busy: 'files.errors.helper_busy',
}

// errorKey는 목록·정보·내려받기 오류의 문구 키다.
export function errorKey(err: unknown): ParseKeys {
  if (err instanceof ApiError && err.code) return ERROR_KEYS[err.code] ?? 'files.errors.generic'
  return 'files.errors.generic'
}

const UPLOAD_KEYS: Partial<Record<string, ParseKeys>> = {
  file_exists: 'files.upload.errors.file_exists',
  permission_denied: 'files.upload.errors.permission_denied',
  no_space: 'files.upload.errors.no_space',
  read_only: 'files.upload.errors.read_only',
  transfer_timeout: 'files.upload.errors.transfer_timeout',
  helper_busy: 'files.upload.errors.helper_busy',
  is_dir: 'files.upload.errors.is_dir',
  network: 'files.upload.errors.network',
}

// uploadErrorKey는 올리기 실패의 문구 키다.
export function uploadErrorKey(error: UploadItem['error']): ParseKeys {
  return (error && UPLOAD_KEYS[error]) ?? 'files.upload.errors.unknown'
}

// uploadError는 올리기 실패를 목록에 둘 이유로 바꾼다.
export function uploadError(err: unknown): NonNullable<UploadItem['error']> {
  if (err instanceof ApiError) return err.code ?? 'unknown'
  if (err instanceof TypeError) return 'network'
  return 'unknown'
}
