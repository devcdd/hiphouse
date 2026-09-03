export type AlbumType = 'single' | 'ep' | 'album'
export type SortKey = 'recent' | 'popular' | 'rating' | 'tracks'

// Multi-select types -> comma-separated server param (undefined = 전체).
export const toTypeParam = (types: AlbumType[]): string | undefined => (types.length ? types.join(',') : undefined)
export const toSortParam = (s: SortKey): string | undefined => (s === 'recent' ? undefined : s)

const VALID_TYPES = new Set<AlbumType>(['single', 'ep', 'album'])
export const parseTypes = (raw: string | null): AlbumType[] =>
  (raw ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter((s): s is AlbumType => VALID_TYPES.has(s as AlbumType))

export const TYPE_OPTIONS: { key: AlbumType; label: string }[] = [
  { key: 'album', label: '정규' },
  { key: 'ep', label: 'EP' },
  { key: 'single', label: '싱글' },
]

// 첫 방문 기본값 — 싱글이 물량으로 피드를 덮어서 정규/EP만 켜고 시작한다.
export const DEFAULT_FILTER_QUERY = 'type=album,ep'

export const SORT_OPTIONS: { key: SortKey; label: string; hint: string }[] = [
  { key: 'recent', label: '최신순', hint: '발매일이 최근인 앨범부터' },
  { key: 'popular', label: '인기순', hint: '별점을 많이 받은 앨범부터' },
  { key: 'rating', label: '별점 높은 순', hint: '평균 별점이 높은 앨범부터' },
  { key: 'tracks', label: '트랙 많은 순', hint: '수록곡이 많은 앨범부터' },
]

const VALID_SORTS = new Set<SortKey>(SORT_OPTIONS.map((o) => o.key))
export const parseSort = (raw: string | null): SortKey =>
  VALID_SORTS.has(raw as SortKey) ? (raw as SortKey) : 'recent'

// The whole list query (year + type + sort) is remembered as one string rather
// than field by field — it's the same thing the URL already encodes, so there's
// nothing to keep in sync. The URL still wins; this only fills in when the list
// is opened with no query at all. Storage can throw (Safari private mode) and a
// lost filter isn't worth crashing the page over.
const FILTER_STORAGE_KEY = 'hiphouse:album-filters'

// null(저장된 적 없음)과 ''(사용자가 '전체'로 지운 상태)를 구분해야 첫 방문에만
// DEFAULT_FILTER_QUERY를 적용하고, 직접 지운 필터는 되살리지 않는다.
export function readStoredFilters(): string | null {
  try {
    return sessionStorage.getItem(FILTER_STORAGE_KEY)
  } catch {
    return null
  }
}

export function storeFilters(query: string) {
  try {
    sessionStorage.setItem(FILTER_STORAGE_KEY, query)
  } catch {
    /* filters just won't persist */
  }
}
