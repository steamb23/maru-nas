// 계정 태그 입력 계산(DOM 없음). OMV 사용자 편집 화면의 "Tags"와 같은 값이며, 백엔드가 쉼표로 이어 저장한다.
// 형식 검사는 백엔드(server/profile.go의 cleanTags, validEmail)가 다시 한다. 여기서는 보내기 전에 바로 알린다.

// 백엔드 maxTags, maxCommentBytes와 맞춘다.
export const MAX_TAGS = 32
export const MAX_TAGS_BYTES = 255

const encoder = new TextEncoder()

// addTags는 입력한 글자를 쉼표로 나눠 태그로 더한다. 앞뒤 공백과 빈 값, 이미 있는 태그는 뺀다.
export function addTags(tags: readonly string[], text: string): string[] {
  const out = [...tags]
  for (const part of text.split(',')) {
    const t = part.trim()
    if (t && !out.includes(t)) out.push(t)
  }
  return out
}

// validTags는 태그를 저장할 수 있는지다. 콜론과 제어 문자는 받지 않고, 이은 길이와 개수를 제한한다.
export function validTags(tags: readonly string[]): boolean {
  if (tags.length > MAX_TAGS) return false
  if (tags.some((t) => t.includes(':') || t.includes(',') || /\p{Cc}/u.test(t))) return false
  return encoder.encode(tags.join(',')).length <= MAX_TAGS_BYTES
}

// validEmail은 전자 우편 주소 모양인지 대략 본다. 빈 값은 지우는 것이라 받는다.
export function validEmail(email: string): boolean {
  return email === '' || /^[^\s@<>()",;:]+@[^\s@<>()",;:]+$/.test(email)
}

export function sameTags(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((t, i) => t === b[i])
}
