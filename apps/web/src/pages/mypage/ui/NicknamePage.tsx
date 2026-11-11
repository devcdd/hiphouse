import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronLeft } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth, updateNickname } from '@/entities/session'
import { useToast } from '@/shared/ui/toast'
import { ArtistClaimSection } from '@/features/artist-claim'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

export function NicknamePage() {
  return (
    <AuthGate>
      <Editor />
    </AuthGate>
  )
}

function Editor() {
  const { user } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState('')

  // Seed once the session resolves (and if it changes elsewhere).
  useEffect(() => {
    if (user?.nickname != null) setDraft(user.nickname)
  }, [user?.nickname])

  const save = useMutation({
    mutationFn: updateNickname,
    onSuccess: (u) => {
      qc.setQueryData(['me'], u)
      toast('닉네임을 바꿨습니다')
    },
    // 409(중복)면 서버 문구를 그대로 보여준다.
    onError: (e: Error) => toast(e.message || '닉네임 변경에 실패했습니다', 'error'),
  })

  const trimmed = draft.trim()
  const canSave = trimmed !== '' && trimmed !== user?.nickname && !save.isPending

  return (
    <div className={styles.page}>
      <Link to="/me" className={styles.back}>
        <ChevronLeft size={16} aria-hidden />
        마이페이지
      </Link>
      <h1 className={styles.title}>닉네임 수정</h1>
      <section className={styles.card}>
        <label className={styles.label} htmlFor="nickname">
          닉네임
        </label>
        <div className={styles.nickRow}>
          <input
            id="nickname"
            className={styles.input}
            value={draft}
            maxLength={20}
            disabled={save.isPending}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              // !isComposing: 한글 IME는 조합 종료 Enter로 keydown이 한 번 더 뜬다 → 중복 저장 방지
              if (e.key === 'Enter' && !e.nativeEvent.isComposing && canSave) save.mutate(trimmed)
            }}
          />
          <button
            type="button"
            className={styles.save}
            disabled={!canSave}
            onClick={() => save.mutate(trimmed)}
          >
            {save.isPending ? '저장 중…' : '저장'}
          </button>
        </div>
      </section>
      <ArtistClaimSection />
    </div>
  )
}
