import { useMemo } from 'react'
import { AlbumCard, AlbumCardSkeleton, useAlbumFeed } from '@/entities/album'
import type { AlbumQuery } from '@/entities/album'
import { useAuth } from '@/entities/session'
import { useMyGiven } from '@/features/rate-album'
import { CopyAnnouncementButton, PromoImageButton } from '@/features/album-promo-image'
import { useInfiniteScroll } from '@/shared/lib/useInfiniteScroll'
import styles from './AlbumFeed.module.css'

// Album grid + infinite scroll for any filter (year / artist / search).
export function AlbumFeed({ params }: { params: AlbumQuery }) {
  const { isAuthed, isAdmin } = useAuth()
  // 삭제된 앨범은 관리자 모드일 때만 흐리게 섞는다. queryKey에도 들어가서 로그아웃하거나 관리자 모드를 끄면
  // 삭제된 앨범이 섞인 캐시를 다시 보여주지 않고 새로 받는다.
  const { data, fetchNextPage, hasNextPage, isFetching, isLoading, isFetchingNextPage, error } = useAlbumFeed({
    ...params,
    includeDeleted: isAdmin,
  })
  const albums = useMemo(() => data?.pages.flat() ?? [], [data])
  const myGiven = useMyGiven(isAuthed)
  const sentinel = useInfiniteScroll<HTMLDivElement>(fetchNextPage, hasNextPage && !isFetching)

  return (
    <div>
      {error && <p className={styles.state}>불러오기 실패: {String(error)}</p>}
      <div className={styles.grid}>
        {albums.map((a) => (
          <div key={a.id} className={a.deleted_at ? `${styles.item} ${styles.dimmed}` : styles.item}>
            <AlbumCard
              album={a}
              given={myGiven(a.id)}
              artAction={
                isAdmin ? (
                  <>
                    <CopyAnnouncementButton album={a} />
                    <PromoImageButton album={a} />
                  </>
                ) : undefined
              }
            />
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
