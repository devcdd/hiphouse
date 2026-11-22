import { Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { Mail, MessageCircle, UserPlus } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { returnPath, startKakaoLogin } from '@/features/auth'
import { useToast } from '@/shared/ui/toast'
import styles from './SignInPage.module.css'

// /sign-in?next=... — 로그인 수단 선택. next는 로그인을 시작한 페이지다.
export function SignInPage() {
  const { isAuthed, isLoading } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = returnPath(params.get('next'))

  if (isLoading) return <div className={styles.page} />
  if (isAuthed) return <Navigate to={next} replace />

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>로그인</h1>
      <p className={styles.lead}>평점, 댓글, 즐겨찾기, 팔로우는 로그인 후 이용할 수 있어요.</p>
      <div className={styles.methods}>
        <button type="button" className={`${styles.method} ${styles.kakao}`} onClick={() => startKakaoLogin(next)}>
          <MessageCircle size={18} fill="currentColor" aria-hidden />
          카카오로 로그인
        </button>
        {/* 이메일 로그인 API가 아직 없어 안내만 띄운다. */}
        <button
          type="button"
          className={`${styles.method} ${styles.email}`}
          onClick={() => toast('이메일 로그인은 준비 중입니다', 'info')}
        >
          <Mail size={18} aria-hidden />
          이메일로 로그인
        </button>
      </div>
      <p className={styles.divider}>회원가입</p>
      {/* 카카오는 첫 로그인이 곧 가입이라 회원가입은 이메일만 둔다. */}
      <button
        type="button"
        className={`${styles.method} ${styles.signUp}`}
        onClick={() => navigate(`/sign-up?next=${encodeURIComponent(next)}`)}
      >
        <UserPlus size={18} aria-hidden />
        이메일로 회원가입
      </button>
    </div>
  )
}
