import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Globe, Lock } from 'lucide-react'
import { AlbumCard, AlbumCardSkeleton } from '@/entities/album'
import { useAuth, updateProfilePublic } from '@/entities/session'
import type { User } from '@/entities/session'
import { StarRating } from '@/features/rate-album'
import { useToast } from '@/shared/ui/toast'
import { fetchPublicRatedAlbums, fetchPublicUser } from '../api/userApi'
import styles from './UserProfilePage.module.css'

const joined = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'long' })

// /users/:id — 남의 활동을 볼 수 있는 유일한 공개 페이지. 로그인 불필요.
// 즐겨찾기·팔로우는 여기에 없다 (공개 엔드포인트 자체가 없음).
//
// 공개 설정 토글도 여기 있다 — 설정 화면을 따로 두면 "남에게 어떻게 보이나"를 확인하러
// 결국 이 페이지로 와야 한다. 주인 본인에게는 비공개여도 내용이 그대로 보이고(서버가
// caller를 보고 열어 준다), 남에게 어떻게 보이는지는 헤더의 배지가 알려준다.
export function UserProfilePage() {
  const { id = '' } = useParams()
  const { user } = useAuth()
  const isOwner = !!user && user.id === id
  const profile = useQuery({ queryKey: ['user', id], queryFn: () => fetchPublicUser(id), enabled: !!id })
  // 주인이 아닐 때만 가린다. 서버도 같은 기준이라 hidden이면 목록은 어차피 빈 배열이다.
  const hidden = !isOwner && profile.data?.profile_public === false
  const rated = useQuery({
    queryKey: ['user-ratings', id],
    // 프로필 헤더가 404면 목록도 볼 필요 없다.
    enabled: !!id && profile.isSuccess && !hidden,
    queryFn: () => fetchPublicRatedAlbums(id),
  })

  if (profile.error) return <p className={styles.state}>프로필을 불러오지 못했습니다.</p>

  const u = profile.data
  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={styles.headerMain}>
          <h1 className={styles.title}>{u?.nickname || '　'}</h1>
          {u && (
            <p className={styles.stats}>
              {!hidden && (
                <>
                  평가 {u.rating_count}개
                  {u.rating_avg != null && <> · 평균 ★{u.rating_avg.toFixed(1)}</>}
                  {u.comment_count > 0 && <> · 댓글 {u.comment_count}개</>}
                </>
              )}
              <span className={styles.joined}>{joined.format(new Date(u.created_at))} 가입</span>
            </p>
          )}
        </div>
        {isOwner && <PrivacyToggle />}
      </header>

      {!hidden && <h2 className={styles.section}>평가한 앨범</h2>}
      {hidden ? (
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

// 현재 상태는 ['me'] 캐시에서 읽는다 — 프로필 응답의 profile_public은 주인에게 항상
// 내용이 열려 있어 화면과 어긋나 보이고, ['me']는 낙관적 업데이트로 즉시 뒤집힌다.
function PrivacyToggle() {
  const { user } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const isPublic = user?.profile_public ?? true

  const save = useMutation({
    mutationFn: updateProfilePublic,
    // 낙관적 업데이트: 버튼이 응답을 기다리며 굳어 있으면 안 눌린 것처럼 보인다.
    onMutate: async (next: boolean) => {
      await qc.cancelQueries({ queryKey: ['me'] })
      const prev = qc.getQueryData<User>(['me'])
      if (prev) qc.setQueryData<User>(['me'], { ...prev, profile_public: next })
      return prev
    },
    onSuccess: (u, next) => {
      qc.setQueryData(['me'], u)
      toast(next ? '내 활동을 공개로 바꿨습니다' : '내 활동을 비공개로 바꿨습니다')
    },
    onError: (_e, _next, prev) => {
      if (prev) qc.setQueryData(['me'], prev)
      toast('공개 설정 변경에 실패했습니다', 'error')
    },
  })

  return (
    <div className={styles.privacy}>
      <button
        type="button"
        className={styles.privacyBtn}
        aria-pressed={!isPublic}
        disabled={!user}
        onClick={() => save.mutate(!isPublic)}
      >
        {isPublic ? <Globe size={14} aria-hidden /> : <Lock size={14} aria-hidden />}
        {isPublic ? '공개' : '비공개'}
      </button>
      <p className={styles.privacyHint}>
        {isPublic
          ? '누르면 비공개로 바뀝니다'
          : '남에게는 닉네임과 가입일만 보입니다. 앨범에 쓴 댓글은 그대로 남습니다.'}
      </p>
    </div>
  )
}
