import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { AlbumCard, AlbumCardSkeleton } from '@/entities/album'
import { StarRating } from '@/features/rate-album'
import { fetchPublicRatedAlbums, fetchPublicUser } from '../api/userApi'
import styles from './UserProfilePage.module.css'

const joined = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'long' })

// /users/:id — 다른 사람이 평가한 앨범을 보는 공개 페이지. 로그인 불필요.
// 즐겨찾기·팔로우는 공개하지 않는다 (평가만 공개 대상).
export function UserProfilePage() {
  const { id = '' } = useParams()
  const profile = useQuery({ queryKey: ['user', id], queryFn: () => fetchPublicUser(id), enabled: !!id })
  const rated = useQuery({
    queryKey: ['user-ratings', id],
    // 프로필 헤더가 404면 목록도 볼 필요 없다.
    enabled: !!id && profile.isSuccess,
    queryFn: () => fetchPublicRatedAlbums(id),
  })

  if (profile.error) return <p className={styles.state}>프로필을 불러오지 못했습니다.</p>

  const u = profile.data
  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <h1 className={styles.title}>{u?.nickname || '　'}</h1>
        {u && (
          <p className={styles.stats}>
            평가 {u.rating_count}개
            {u.rating_avg != null && <> · 평균 ★{u.rating_avg.toFixed(1)}</>}
            {u.comment_count > 0 && <> · 댓글 {u.comment_count}개</>}
            <span className={styles.joined}>{joined.format(new Date(u.created_at))} 가입</span>
          </p>
        )}
      </header>

      <h2 className={styles.section}>평가한 앨범</h2>
      {rated.isSuccess && rated.data.length === 0 && <p className={styles.state}>아직 평가한 앨범이 없습니다.</p>}
      <div className={styles.grid}>
        {rated.data?.map((a) => (
          <div key={a.id} className={styles.item}>
            <AlbumCard album={a} />
            <StarRating score={a.score} size={15} />
          </div>
        ))}
        {(profile.isLoading || rated.isLoading) &&
          Array.from({ length: 8 }, (_, i) => (
            <div key={`skeleton-${i}`} className={styles.item}>
              <AlbumCardSkeleton />
            </div>
          ))}
      </div>
    </div>
  )
}
