// 목록 범위의 캐시. 한 보기(정렬·방향·찾기)의 항목을 PAGE_SIZE줄 단위 페이지로 받아 두고, 오래 쓰지 않은 페이지부터
// 버린다. 보기가 바뀌면 새 캐시를 쓴다. DOM에 의존하지 않아 node 환경에서 테스트한다.

// PAGE_SIZE는 한 번에 받는 줄 수다(백엔드 상한 500 안).
export const PAGE_SIZE = 200

// MAX_PAGES는 기억하는 페이지 수다(약 1만 2천 줄).
export const MAX_PAGES = 60

// pagesFor는 [first, last) 줄을 덮는 페이지 번호다.
export function pagesFor(first: number, last: number, pageSize = PAGE_SIZE): number[] {
  if (last <= first) return []
  const out: number[] = []
  for (let p = Math.floor(first / pageSize); p * pageSize < last; p++) out.push(p)
  return out
}

export class PageCache<T> {
  // pages는 넣거나 쓴 순서다(Map은 넣은 순서를 지킨다). 맨 앞이 가장 오래 쓰지 않은 페이지다.
  private pages = new Map<number, T[]>()
  private pending = new Set<number>()

  readonly pageSize: number
  readonly maxPages: number

  constructor(pageSize = PAGE_SIZE, maxPages = MAX_PAGES) {
    this.pageSize = pageSize
    this.maxPages = maxPages
  }

  // get은 index 줄이다. 아직 받지 않았으면 undefined다.
  get(index: number): T | undefined {
    const p = Math.floor(index / this.pageSize)
    const page = this.pages.get(p)
    return page?.[index - p * this.pageSize]
  }

  // missing은 [first, last) 줄 가운데 받지도 받는 중도 아닌 페이지다.
  missing(first: number, last: number): number[] {
    return pagesFor(first, last, this.pageSize).filter((p) => !this.pages.has(p) && !this.pending.has(p))
  }

  // touch는 보이는 페이지를 가장 최근에 쓴 것으로 옮긴다.
  touch(first: number, last: number): void {
    for (const p of pagesFor(first, last, this.pageSize)) {
      const page = this.pages.get(p)
      if (page) {
        this.pages.delete(p)
        this.pages.set(p, page)
      }
    }
  }

  // start는 페이지를 받는 중으로 표시한다. 같은 페이지를 겹쳐 요청하지 않게 한다.
  start(page: number): void {
    this.pending.add(page)
  }

  // fail은 받지 못한 페이지의 표시를 지운다(다음에 다시 요청한다).
  fail(page: number): void {
    this.pending.delete(page)
  }

  // put은 받은 페이지를 둔다. 넘치면 오래 쓰지 않은 페이지를 버린다.
  put(page: number, items: T[]): void {
    this.pending.delete(page)
    this.pages.delete(page)
    this.pages.set(page, items)
    while (this.pages.size > this.maxPages) {
      const oldest = this.pages.keys().next().value
      if (oldest === undefined) break
      this.pages.delete(oldest)
    }
  }

  // set은 받은 줄 하나를 바꾼다(올린 파일 반영 등). 그 페이지가 없으면 하지 않는다.
  set(index: number, item: T): void {
    const p = Math.floor(index / this.pageSize)
    const page = this.pages.get(p)
    if (page && index - p * this.pageSize < page.length) page[index - p * this.pageSize] = item
  }

  get size(): number {
    return this.pages.size
  }
}
