// 올리기 이름 충돌 대화상자의 제목과 내용(문구, "남은 파일에도 같게 적용" 확인란).
import { useTranslation } from 'react-i18next'

interface Props {
  name: string
  // many면 확인란을 보인다(남은 파일이 있을 때).
  many: boolean
  onApplyAll: (v: boolean) => void
}

export function ConflictBody({ name, many, onApplyAll }: Props) {
  const { t } = useTranslation()
  return (
    <div className="modal-message files-conflict">
      <p>{t('files.upload.conflictMessage', { name })}</p>
      {many && (
        <label className="files-check">
          <input type="checkbox" onChange={(e) => onApplyAll(e.currentTarget.checked)} />
          {t('files.upload.applyAll')}
        </label>
      )}
    </div>
  )
}

// ConflictTitle은 대화상자 제목이다. 명령형으로 여는 대화상자라 번역 훅을 쓰는 컴포넌트로 그린다.
export function ConflictTitle() {
  const { t } = useTranslation()
  return <>{t('files.upload.conflictTitle')}</>
}
