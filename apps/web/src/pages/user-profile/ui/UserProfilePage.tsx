import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { AlbumCard, AlbumCardSkeleton } from '@/entities/album'
import { StarRating } from '@/features/rate-album'
import { fetchPublicRatedAlbums, fetchPublicUser } from '../api/userApi'
import styles from './UserProfilePage.module.css'

const joined = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'long' })

// /users/:id — 남의 활동을 볼 수 있는 유일한 공개 페이지. 로그인 불필요.
// 즐겨찾기·팔로우는 여기에 없다 (공개 엔드포인트 자체가 없음).
//
// 주인이 /me/privacy에서 공개를 끄면 닉네임과 가입일만 남는다. 서버가 이미 집계를
// 0/null로 비워 보내지만, 화면에서도 "0개"가 아니라 안내 문구가 나가야 비공개라는
// 사실이 읽힌다. 본인이 봐도 똑같이 보인다 — 마이페이지 메뉴가 이 페이지를 "내 공개
// 프로필"로 부르는 만큼, 남에게 보이는 그대로여야 설정이 먹었는지 확인이 된다.
export function UserProfilePage() {
  const { id = '' } = useParams()
  const profile = useQuery({ queryKey: ['user', id], queryFn: () => fetchPublicUser(id), enabled: !!id })
  const isPrivate = profile.data?.profile_public === false
  const rated = useQuery({
    queryKey: ['user-ratings', id],
    // 프로필 헤더가 404면 목록도 볼 필요 없고, 비공개면 어차피 빈 배열이 온다.
    enabled: !!id && profile.isSuccess && !isPrivate,
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
            {!isPrivate && (
              <>
                평가 {u.rating_count}개
                {u.rating_avg != null && <> · 평균 ★{u.rating_avg.toFixed(1)}</>}
                {u.comment_count > 0 && <> · 댓글 {u.comment_count}개</>}
              </>
            )}
            <span className={styles.joined}>{joined.format(new Date(u.created_at))} 가입</span>
          </p>
        )}
      </header>

      {!isPrivate && <h2 className={styles.section}>평가한 앨범</h2>}
      {isPrivate ? (
        <p className={styles.state}>이 사용자는 활동을 비공개로 설정했습니다.</p>
      ) : (
        <>
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
        </>
      )}
    </div>
  )
}
