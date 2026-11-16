import { useMemo } from 'react'
import { AlbumCard, AlbumCardSkeleton, useAlbumFeed } from '@/entities/album'
import type { AlbumQuery } from '@/entities/album'
import { useAuth } from '@/entities/session'
import { useMyGiven } from '@/features/rate-album'
import { useInfiniteScroll } from '@/shared/lib/useInfiniteScroll'
import styles from './AlbumFeed.module.css'

// Album grid + infinite scroll for any filter (year / artist / search).
// Soft-deleted albums only reach admins (server-gated) and render dimmed.
export function AlbumFeed({ params }: { params: AlbumQuery }) {
  const { data, fetchNextPage, hasNextPage, isFetching, isLoading, isFetchingNextPage, error } =
    useAlbumFeed(params)
  const albums = useMemo(() => data?.pages.flat() ?? [], [data])
  const { isAuthed } = useAuth()
  const myGiven = useMyGiven(isAuthed)
  const sentinel = useInfiniteScroll<HTMLDivElement>(fetchNextPage, hasNextPage && !isFetching)

  return (
    <div>
      {error && <p className={styles.state}>불러오기 실패: {String(error)}</p>}
      <div className={styles.grid}>
        {albums.map((a) => (
          <div key={a.id} className={a.deleted_at ? `${styles.item} ${styles.dimmed}` : styles.item}>
            <AlbumCard album={a} given={myGiven(a.id)} />
          </div>
        ))}
        {isLoading &&
          Array.from({ length: 12 }, (_, i) => (
            <div key={`skeleton-${i}`} className={styles.item}>
              <AlbumCardSkeleton />
            </div>
          ))}
      </div>
      {hasNextPage && <div ref={sentinel} className={styles.sentinel} />}
      {isFetchingNextPage && <p className={styles.state}>불러오는 중…</p>}
      {!isLoading && !error && albums.length === 0 && <p className={styles.state}>앨범이 없습니다.</p>}
    </div>
  )
}
