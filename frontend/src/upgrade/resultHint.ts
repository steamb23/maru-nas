// 업그레이드로 다시 시작한 뒤 "다음 로그인에서 업데이트 결과를 보여 줘라"는 화면 힌트(이 탭의 sessionStorage 키 하나).
// 로그인 상태를 저장하는 것이 아니다(그것은 하지 않는다, .claude/rules/auth-omv.md). 세션은 다시 시작하며 끝나므로,
// 새로 고친 로그인 화면이 안내를 보이고, 관리자가 로그인하면 데스크톱이 설정의 업데이트 페이지를 연다.
// sessionStorage를 쓸 수 없으면(사생활 보호 모드 등) 힌트 없이 동작한다.
const KEY = 'onmaru.upgradeResult'

export function setUpgradeHint(): void {
  try {
    sessionStorage.setItem(KEY, '1')
  } catch {
    // 힌트 없이 다시 로그인하면 된다.
  }
}

export function hasUpgradeHint(): boolean {
  try {
    return sessionStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

export function clearUpgradeHint(): void {
  try {
    sessionStorage.removeItem(KEY)
  } catch {
    // 남아도 다음 로그인에서 한 번 더 페이지를 열 뿐이다.
  }
}
