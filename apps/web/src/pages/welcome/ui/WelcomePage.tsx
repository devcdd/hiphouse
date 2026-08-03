import { useState } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth, updateNickname } from '@/entities/session'
import { useToast } from '@/shared/ui/toast'
import styles from './WelcomePage.module.css'

// /welcome — 첫 로그인 온보딩. 카카오가 준 이름으로 인사하고 닉네임을 받는다.
// 닉네임을 저장하지 않고 나가면 nickname_set이 false로 남아 다음 로그인 때 다시 온다.
export function WelcomePage() {
  const { user, isAuthed, isLoading } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const navigate = useNavigate()
  const [draft, setDraft] = useState('')

  const save = useMutation({
    mutationFn: updateNickname,
    onSuccess: (u) => {
      qc.setQueryData(['me'], u)
      navigate('/', { replace: true })
    },
    // 409(중복)면 서버 문구를 그대로 보여준다.
    onError: (e: Error) => toast(e.message || '닉네임 저장에 실패했습니다', 'error'),
  })

  if (isLoading) return <div className={styles.page} />
  if (!isAuthed) return <Navigate to="/" replace />
  if (user?.nickname_set) return <Navigate to="/" replace />

  const trimmed = draft.trim()
  const canSave = trimmed !== '' && !save.isPending

  return (
    <div className={styles.page}>
      <h1 className={styles.greeting}>
        {user?.nickname ? `안녕하세요, ${user.nickname}님.` : '안녕하세요.'}
      </h1>
      <p className={styles.lead}>힙하우스에서 쓸 닉네임을 정해주세요.</p>
      <section className={styles.card}>
        <label className={styles.label} htmlFor="nickname">
          닉네임
        </label>
        <div className={styles.row}>
          <input
            id="nickname"
            className={styles.input}
            value={draft}
            maxLength={20}
            autoFocus
            placeholder={user?.nickname || '닉네임'}
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
            {save.isPending ? '저장 중…' : '시작하기'}
          </button>
        </div>
      </section>
    </div>
  )
}
