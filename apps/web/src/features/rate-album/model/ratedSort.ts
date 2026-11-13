export type RatedSortKey = 'score' | 'rated' | 'release'

export const RATED_SORT_OPTIONS: { key: RatedSortKey; label: string; hint: string }[] = [
  { key: 'score', label: '별점순', hint: '별점을 높게 준 앨범부터' },
  { key: 'rated', label: '최근 평가한 순', hint: '최근에 평가한 앨범부터' },
  { key: 'release', label: '최근 발매일 순', hint: '발매일이 최근인 앨범부터' },
]

export const DEFAULT_RATED_SORT: RatedSortKey = 'rated'

const VALID = new Set<RatedSortKey>(RATED_SORT_OPTIONS.map((o) => o.key))
export const parseRatedSort = (raw: string | null): RatedSortKey =>
  VALID.has(raw as RatedSortKey) ? (raw as RatedSortKey) : DEFAULT_RATED_SORT

// 기본 정렬은 URL과 서버 요청 양쪽에서 생략한다.
export const toRatedSortParam = (s: RatedSortKey): string | undefined => (s === DEFAULT_RATED_SORT ? undefined : s)
