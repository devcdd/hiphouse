import { useMemo } from 'react'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiDelete, apiGet } from '@/shared/api/client'
import { useInfiniteScroll } from '@/shared/lib/useInfiniteScroll'
import { useToast } from '@/shared/ui/toast'
import styles from './AdminPage.module.css'

interface AlbumTombstone {
  id: string
  apple_id: string | null
  name: string
  artists: string
  deleted_at: string
}

const PAGE = 100

// 지운 앨범은 행이 없고 id만 남아서 크롤러가 다시 넣지 않는다. 차단을 풀어도 앨범이 바로 돌아오지는 않는다.
export function DeletedAlbumsTab() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data, fetchNextPage, hasNextPage, isFetching, isLoading, isFetchingNextPage, error } = useInfiniteQuery({
    queryKey: ['admin-album-tombstones'],
    queryFn: ({ pageParam }) => apiGet<AlbumTombstone[]>('/admin/album-tombstones', { limit: PAGE, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (last, pages) => (last.length < PAGE ? undefined : pages.length * PAGE),
  })
  const rows = useMemo(() => data?.pages.flat() ?? [], [data])
  const sentinel = useInfiniteScroll<HTMLDivElement>(fetchNextPage, hasNextPage && !isFetching)

  const unblock = useMutation({
    mutationFn: (id: string) => apiDelete(`/admin/album-tombstones/${encodeURIComponent(id)}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['admin-album-tombstones'] })
      toast('차단을 풀었습니다. 다음 크롤이나 수동 추가로 다시 들어옵니다')
    },
    onError: () => toast('처리에 실패했습니다', 'error'),
  })

  return (
    <div>
      <p className={styles.tombHint}>
        삭제한 앨범은 별점, 댓글과 함께 지워졌고 크롤해도 다시 들어오지 않습니다. 차단 해제는 다시 받을 수 있게만
        합니다.
      </p>
      {error && <p className={styles.state}>불러오기 실패: {String(error)}</p>}
      <ul className={styles.tombList}>
        {rows.map((t) => (
          <li key={t.id} className={styles.tombRow}>
            <div className={styles.tombText}>
              <div className={styles.tombName}>{t.name || t.id}</div>
              <div className={styles.tombMeta}>
                {[t.artists, t.deleted_at.slice(0, 10), t.id].filter(Boolean).join(' · ')}
              </div>
            </div>
            <button
              type="button"
              className={styles.unblock}
              disabled={unblock.isPending}
              onClick={() => unblock.mutate(t.id)}
            >
              차단 해제
            </button>
          </li>
        ))}
      </ul>
      {hasNextPage && <div ref={sentinel} className={styles.sentinel} />}
      {(isLoading || isFetchingNextPage) && <p className={styles.state}>불러오는 중…</p>}
      {!isLoading && !error && rows.length === 0 && <p className={styles.state}>삭제한 앨범이 없습니다.</p>}
    </div>
  )
}
