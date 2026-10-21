// 폴더 만들기·이름 바꾸기·다른 이름으로 되돌리기 대화상자의 내용. 이름을 받아 바로 요청하고, 같은 이름이 있거나
// 권한이 없으면 대화상자를 닫지 않고 입력란 아래에 알린다. 여는 함수는 nameDialog.tsx.
import type { ParseKeys } from 'i18next'
import { type RefObject, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { FileItem } from '../api/types.gen'
import { opErrorKey } from './ops'
import { type NameProblem, nameProblem } from './paths'

const PROBLEM_KEYS: Record<NameProblem, ParseKeys> = {
  empty: 'files.name.errors.empty',
  dots: 'files.name.errors.dots',
  chars: 'files.name.errors.chars',
  tooLong: 'files.name.errors.tooLong',
  reserved: 'files.name.errors.reserved',
}

export type NameMode = 'mkdir' | 'rename' | 'restore'

const TITLE_KEYS: Record<NameMode, ParseKeys> = {
  mkdir: 'files.name.mkdirTitle',
  rename: 'files.name.renameTitle',
  restore: 'files.name.restoreTitle',
}

const SUBMIT_KEYS: Record<NameMode, ParseKeys> = {
  mkdir: 'files.name.create',
  rename: 'files.name.rename',
  restore: 'files.name.restore',
}

export interface NameFormProps {
  mode: NameMode
  // message는 입력란 위의 안내다(다른 이름으로 되돌리기).
  message?: string
  initial: string
  // selectEnd는 처음 고를 글자 수다(이름 바꾸기는 확장자를 빼고 고른다).
  selectEnd: number
  inputRef: RefObject<HTMLInputElement | null>
  // submit은 그 이름으로 만들거나 바꾼다. 실패하면 ApiError를 던진다.
  submit: (name: string) => Promise<FileItem>
  onDone: (item: FileItem) => void
  onCancel: () => void
}

export function NameForm({ mode, message, initial, selectEnd, inputRef, submit, onDone, onCancel }: NameFormProps) {
  const { t } = useTranslation()
  const [name, setName] = useState(initial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ParseKeys | null>(null)
  // 처음 포커스를 받을 때만 고르는 범위를 정한다(포커스는 Modal이 initialFocus로 옮긴다).
  const selected = useRef(false)

  useEffect(() => {
    if (error && !busy) inputRef.current?.focus()
  }, [error, busy, inputRef])

  const problem = nameProblem(name)
  const go = async () => {
    if (busy) return
    if (problem) {
      setError(PROBLEM_KEYS[problem])
      return
    }
    setBusy(true)
    setError(null)
    try {
      onDone(await submit(name))
    } catch (err) {
      setError(opErrorKey(err))
      setBusy(false)
    }
  }

  return (
    <form
      className="files-name-form"
      onSubmit={(e) => {
        e.preventDefault()
        void go()
      }}
    >
      {message && <p className="modal-message files-name-message">{message}</p>}
      <label className="files-name-field">
        <span>{t('files.name.label')}</span>
        <input
          ref={inputRef}
          className="files-name-input"
          name="file-name"
          autoComplete="off"
          spellCheck={false}
          value={name}
          disabled={busy}
          aria-invalid={error !== null || undefined}
          onFocus={(e) => {
            if (selected.current) return
            selected.current = true
            e.currentTarget.setSelectionRange(0, selectEnd)
          }}
          onChange={(e) => {
            setName(e.target.value)
            setError(null)
          }}
        />
      </label>
      {error && (
        <p className="files-name-error" role="alert">
          {t(error)}
        </p>
      )}
      <div className="modal-buttons">
        <button type="button" className="addon-button" disabled={busy} onClick={onCancel}>
          {t('dialog.cancel')}
        </button>
        <button type="submit" className="addon-button primary" disabled={busy || name === ''}>
          {t(SUBMIT_KEYS[mode])}
        </button>
      </div>
    </form>
  )
}

// NameTitle은 대화상자 제목이다. 명령형으로 여는 대화상자라 번역 훅을 쓰는 컴포넌트로 그린다.
export function NameTitle({ mode }: { mode: NameMode }) {
  const { t } = useTranslation()
  return <>{t(TITLE_KEYS[mode])}</>
}
