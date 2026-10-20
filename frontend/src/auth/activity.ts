// 입력 없이 이어지는 사용자 작업(파일 올리기)을 세션 활동으로 알린다. useSessionKeeper가 키·포인터 입력과 같게 받아
// 최대 60초에 한 번 OMV 세션을 연장한다. 올리기가 진행되는 동안만 부른다(전송 상한 6시간).

export const ACTIVITY_EVENT = 'onmaru-activity'

export function reportActivity(): void {
  document.dispatchEvent(new Event(ACTIVITY_EVENT))
}
