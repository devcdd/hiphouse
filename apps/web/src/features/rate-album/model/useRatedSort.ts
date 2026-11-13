import { useSearchParams } from 'react-router-dom'
import { parseRatedSort, toRatedSortParam, type RatedSortKey } from './ratedSort'

// 정렬을 URL(?sort=)에 둬야 앨범 상세에 다녀와도 유지된다.
export function useRatedSort(): [RatedSortKey, (next: RatedSortKey) => void] {
  const [search, setSearch] = useSearchParams()
  const sort = parseRatedSort(search.get('sort'))

  function setSort(next: RatedSortKey) {
    const p = new URLSearchParams(search)
    const param = toRatedSortParam(next)
    if (param) p.set('sort', param)
    else p.delete('sort')
    setSearch(p, { replace: true })
  }

  return [sort, setSort]
}
