import { useEffect, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { YearFilter, useYears, toYearParam, ALL, type YearOption } from '@/features/filter-albums-by-year'
import {
  TypeFilter,
  AwardedFilter,
  SortSelect,
  toTypeParam,
  toSortParam,
  parseTypes,
  parseSort,
  readStoredFilters,
  storeFilters,
  DEFAULT_FILTER_QUERY,
} from '@/features/album-filters'
import { AlbumFeed } from '@/widgets/album-feed'
import styles from './AlbumsPage.module.css'

// Filter state lives in the URL so it survives navigating to a detail page and back.
export function AlbumsPage() {
  const [search, setSearch] = useSearchParams()
  const years = useYears()

  const yearRaw = search.get('year')
  const year: YearOption = yearRaw && /^\d+$/.test(yearRaw) ? Number(yearRaw) : ALL
  const types = parseTypes(search.get('type'))
  const sort = parseSort(search.get('sort'))
  const awarded = search.get('awarded') === '1'

  // "트랙 많은 순"은 싱글만 선택했을 땐 의미 없음.
  const singleOnly = types.length === 1 && types[0] === 'single'

  function patch(next: Record<string, string | undefined>) {
    const p = new URLSearchParams(search)
    for (const [k, v] of Object.entries(next)) {
      if (v) p.set(k, v)
      else p.delete(k)
    }
    storeFilters(p.toString())
    setSearch(p, { replace: true })
  }

  // Opened with no query at all (logo click, fresh tab) → restore the last
  // filters, or the 정규+EP 기본값 on a first visit. Any explicit query,
  // including one cleared back to empty, wins.
  const query = search.toString()
  useEffect(() => {
    if (query !== '') return
    const next = readStoredFilters() ?? DEFAULT_FILTER_QUERY
    if (next !== '') setSearch(next, { replace: true })
  }, [query])

  const typeParam = toTypeParam(types)
  const params = useMemo(
    () => ({ year: toYearParam(year), albumType: typeParam, sort: toSortParam(sort), awarded }),
    [year, typeParam, sort, awarded],
  )

  return (
    <div className={styles.page}>
      <div className={styles.controls}>
        <YearFilter
          years={years}
          value={year}
          onChange={(y) => patch({ year: y === ALL ? undefined : String(y) })}
        />
        <div className={styles.row}>
          <div className={styles.group}>
            <TypeFilter
              value={types}
              onChange={(next) => {
                // 싱글만 남으면 트랙수 정렬 무의미 → 최신순으로 되돌림.
                const nextSingleOnly = next.length === 1 && next[0] === 'single'
                patch({ type: toTypeParam(next), ...(nextSingleOnly && sort === 'tracks' ? { sort: undefined } : {}) })
              }}
            />
            <AwardedFilter value={awarded} onChange={(v) => patch({ awarded: v ? '1' : undefined })} />
          </div>
          <SortSelect
            value={sort}
            onChange={(s) => patch({ sort: toSortParam(s) })}
            disabledKeys={singleOnly ? ['tracks'] : []}
          />
        </div>
      </div>
      <AlbumFeed params={params} />
    </div>
  )
}
