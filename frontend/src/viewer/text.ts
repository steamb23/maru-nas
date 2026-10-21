// 미리보기 창의 텍스트 보기. 받은 앞부분을 UTF-8로 읽는다. DOM에 의존하지 않아 node 환경에서 테스트한다.

// TEXT_LIMIT는 텍스트 보기가 받는 앞부분의 크기(바이트)다. 더 큰 파일은 여기까지만 보이고 잘렸다고 알린다.
export const TEXT_LIMIT = 1 << 20

// BINARY_SCAN은 이진 파일인지 볼 때 살피는 앞부분의 크기(바이트)다.
const BINARY_SCAN = 8000

export interface DecodedText {
  text: string
  // truncated면 파일이 받은 부분보다 크다.
  truncated: boolean
  // binary면 NUL 바이트가 있어 텍스트로 보이지 않는다.
  binary: boolean
}

// decodeText는 받은 바이트를 글자로 바꾼다. size는 파일 전체 크기다. 잘린 끝의 덜 받은 UTF-8 글자는 버리고(stream),
// UTF-8이 아닌 바이트는 대체 문자로 보인다.
export function decodeText(bytes: Uint8Array, size: number): DecodedText {
  const truncated = size > bytes.length
  if (bytes.subarray(0, BINARY_SCAN).includes(0)) return { text: '', truncated, binary: true }
  let text = new TextDecoder('utf-8').decode(bytes, { stream: truncated })
  // 바이트 순서 표시(BOM)는 보이지 않는다.
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1)
  return { text, truncated, binary: false }
}
