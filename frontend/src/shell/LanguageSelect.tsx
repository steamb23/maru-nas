// LanguageSelect는 로그인·연결 끊김 화면의 언어 선택 상자다. 로그인 후에는 프로필 메뉴에서 바꾼다.
// 접근성·키보드 조작은 브라우저 기본 <select>에 맡긴다.
import { useTranslation } from 'react-i18next'
import { setLanguage, useLanguage } from '../i18n'
import { SUPPORTED_LANGUAGES, isLanguage, languageName } from '../i18n/languages'
import { IconImage } from '../icons/IconImage'

export function LanguageSelect() {
  const { t } = useTranslation()
  const lang = useLanguage()
  return (
    <label className="language-select">
      <IconImage name="preferences-desktop-locale-symbolic" size={16} />
      <select
        aria-label={t('language.label')}
        value={lang}
        onChange={(e) => {
          if (isLanguage(e.target.value)) void setLanguage(e.target.value)
        }}
      >
        {SUPPORTED_LANGUAGES.map((code) => (
          <option key={code} value={code} lang={code}>
            {languageName(code)}
          </option>
        ))}
      </select>
    </label>
  )
}
