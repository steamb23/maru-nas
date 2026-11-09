// 서버 시계와 이 브라우저 시계의 차이(DOM 없음). 알림 저장소가 알림 응답의 서버 시각으로 두고, 예약된 다시 시작의
// 남은 시간(power/)이 쓴다.

// clockSkew는 서버 시계에서 이 브라우저 시계를 뺀 값(ms)이다. 서버 시각을 읽지 못하면 0이다.
export function clockSkew(serverNow: string, clientNow: number): number {
  const ms = Date.parse(serverNow)
  return Number.isNaN(ms) ? 0 : ms - clientNow
}
