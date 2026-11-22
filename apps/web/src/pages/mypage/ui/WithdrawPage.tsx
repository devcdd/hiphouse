import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ChevronLeft } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { withdraw } from '@/entities/session'
import { clearToken } from '@/shared/lib/token'
import { useToast } from '@/shared/ui/toast'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

const CONFIRM_WORD = '탈퇴'

export function WithdrawPage() {
  return (
    <AuthGate>
      <Withdraw />
    </AuthGate>
  )
}

function Withdraw() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const toast = useToast()
  const [confirm, setConfirm] = useState('')

  const run = useMutation({
    mutationFn: withdraw,
    onSuccess: () => {
      // 서버가 refresh token까지 지웠으니 logout을 부를 필요 없이 로컬 상태만 비운다.
      clearToken()
      qc.clear()
      navigate('/', { replace: true })
      toast('탈퇴했습니다. 그동안 이용해 주셔서 고맙습니다.')
    },
    onError: (e: Error) => toast(e.message || '탈퇴에 실패했습니다', 'error'),
  })

  const canSubmit = confirm.trim() === CONFIRM_WORD && !run.isPending

  return (
    <div className={styles.page}>
      <Link to="/me" className={styles.back}>
        <ChevronLeft size={16} aria-hidden />
        마이페이지
      </Link>
      <h1 className={styles.title}>회원 탈퇴</h1>
      <section className={styles.card}>
        <p className={styles.notice}>탈퇴하면 아래 기록이 바로 지워지고 되돌릴 수 없습니다.</p>
        <ul className={styles.noticeList}>
          <li>앨범 평가와 즐겨찾기, 아티스트 팔로우</li>
          <li>담벼락 글과 프로필 사진</li>
          <li>아티스트 본인 인증 신청</li>
          <li>댓글. 다른 사람의 답글이 달린 댓글은 내용 없이 "삭제된 댓글"로 남습니다.</li>
        </ul>
        <label className={styles.label} htmlFor="withdraw-confirm">
          계속하려면 "{CONFIRM_WORD}"를 입력하세요
        </label>
        <div className={styles.nickRow}>
          <input
            id="withdraw-confirm"
            className={styles.input}
            value={confirm}
            autoComplete="off"
            disabled={run.isPending}
            onChange={(e) => setConfirm(e.target.value)}
          />
          <button type="button" className={styles.danger} disabled={!canSubmit} onClick={() => run.mutate()}>
            {run.isPending ? '처리 중…' : '탈퇴하기'}
          </button>
        </div>
      </section>
    </div>
  )
}
