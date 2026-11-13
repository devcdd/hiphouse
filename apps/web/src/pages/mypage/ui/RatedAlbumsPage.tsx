import { Link } from 'react-router-dom'
import { ChevronLeft } from 'lucide-react'
import { AlbumCard, AlbumCardSkeleton } from '@/entities/album'
import { RatedSortSelect, useRatedAlbums, useRatedSort } from '@/features/rate-album'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

export function RatedAlbumsPage() {
  return (
    <AuthGate>
      <RatedList />
    </AuthGate>
  )
}

function RatedList() {
  const [sort, setSort] = useRatedSort()
  const { data: albums, isLoading } = useRatedAlbums(true, sort) // AuthGate guarantees authed
  const empty = !isLoading && (albums?.length ?? 0) === 0

  return (
    <div className={styles.page}>
      <Link to="/me" className={styles.back}>
        <ChevronLeft size={16} aria-hidden />
        마이페이지
      </Link>
      <div className={styles.titleRow}>
        <h1 className={styles.title}>내가 평가한 앨범</h1>
        {!empty && <RatedSortSelect value={sort} onChange={setSort} />}
      </div>
      {empty ? (
        <p className={styles.state}>아직 평가한 앨범이 없습니다.</p>
      ) : (
        <div className={styles.grid}>
          {albums?.map((a) => (
            <div key={a.id} className={styles.item}>
              <AlbumCard album={a} given={{ score: a.score, label: '내가 준 평점' }} />
            </div>
          ))}
          {isLoading &&
            Array.from({ length: 8 }, (_, i) => (
              <div key={`skeleton-${i}`} className={styles.item}>
                <AlbumCardSkeleton />
              </div>
            ))}
        </div>
      )}
    </div>
  )
}
