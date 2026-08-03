import { Link } from 'react-router-dom'
import { ChevronLeft } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth, updateProfilePublic } from '@/entities/session'
import type { User } from '@/entities/session'
import { useToast } from '@/shared/ui/toast'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

export function PrivacyPage() {
  return (
    <AuthGate>
      <Settings />
    </AuthGate>
  )
}

function Settings() {
  const { user } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()

  const save = useMutation({
    mutationFn: updateProfilePublic,
    // 낙관적 업데이트: 스위치가 응답을 기다리며 굳어 있으면 안 눌린 것처럼 보인다.
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

  const isPublic = user?.profile_public ?? true

  return (
    <div className={styles.page}>
      <Link to="/me" className={styles.back}>
        <ChevronLeft size={16} aria-hidden />
        마이페이지
      </Link>
      <h1 className={styles.title}>공개 설정</h1>
      <section className={styles.card}>
        <div className={styles.toggleRow}>
          <label className={styles.toggleLabel} htmlFor="profile-public">
            내 활동 공개
          </label>
          <input
            id="profile-public"
            type="checkbox"
            className={styles.checkbox}
            checked={isPublic}
            disabled={!user}
            onChange={(e) => save.mutate(e.target.checked)}
          />
        </div>
        <p className={styles.hint}>
          {isPublic
            ? '다른 사람이 내 프로필에서 내가 평가한 앨범과 평균 별점, 활동 수를 볼 수 있습니다.'
            : '내 프로필에 닉네임과 가입일만 남습니다. 평가한 앨범도, 평가·댓글 수도 다른 사람에게 보이지 않습니다. 앨범의 평균 별점에는 내 점수가 계속 반영되지만 누가 줬는지는 드러나지 않습니다.'}
        </p>
        {/* 팔로우·즐겨찾기는 공개 엔드포인트가 아예 없다. 토글로 덮을 게 없으니
            "숨김"이 아니라 "원래 나만 본다"로 적는다. */}
        <p className={styles.hint}>
          팔로우한 아티스트와 즐겨찾기한 앨범은 이 설정과 무관하게 처음부터 나만 볼 수 있습니다.
        </p>
        {/* 댓글 본문은 앨범 페이지에 닉네임과 함께 붙는 글이라 지울 수 없다 —
            숨기면 남의 답글이 대화 상대 없이 남는다. 지우려면 직접 삭제. */}
        <p className={styles.hint}>
          앨범에 쓴 댓글은 해당 앨범 페이지에 닉네임과 함께 남습니다. 지우려면 댓글을 직접 삭제하세요.
        </p>
      </section>
    </div>
  )
}
