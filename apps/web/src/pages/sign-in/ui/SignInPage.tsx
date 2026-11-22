import { useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Mail, MessageCircle, UserPlus } from 'lucide-react'
import { loginWithEmail, useAuth } from '@/entities/session'
import { afterSignInPath, returnPath, startKakaoLogin } from '@/features/auth'
import { useToast } from '@/shared/ui/toast'
import styles from './SignInPage.module.css'

// /sign-in?next=... — 로그인 수단 선택. next는 로그인을 시작한 페이지다.
export function SignInPage() {
  const { isAuthed, isLoading, signIn } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = returnPath(params.get('next'))
  const [emailOpen, setEmailOpen] = useState(false)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  const login = useMutation({
    mutationFn: () => loginWithEmail(email.trim(), password),
    onSuccess: (res) => {
      signIn(res.token, res.refresh_token, res.user)
      navigate(afterSignInPath(res.user, next), { replace: true })
    },
    onError: (e: Error) => toast(e.message || '로그인에 실패했습니다', 'error'),
  })

  if (isLoading) return <div className={styles.page} />
  if (isAuthed) return <Navigate to={next} replace />

  const nextQuery = `?next=${encodeURIComponent(next)}`
  const canLogin = email.trim() !== '' && password !== '' && !login.isPending
  const onLogin = (e: FormEvent) => {
    e.preventDefault()
    if (canLogin) login.mutate()
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>로그인</h1>
      <p className={styles.lead}>평점, 댓글, 즐겨찾기, 팔로우는 로그인 후 이용할 수 있어요.</p>
      <div className={styles.methods}>
        <button type="button" className={`${styles.method} ${styles.kakao}`} onClick={() => startKakaoLogin(next)}>
          <MessageCircle size={18} fill="currentColor" aria-hidden />
          카카오로 로그인
        </button>
        <button
          type="button"
          className={`${styles.method} ${styles.email}`}
          aria-expanded={emailOpen}
          onClick={() => setEmailOpen((v) => !v)}
        >
          <Mail size={18} aria-hidden />
          이메일로 로그인
        </button>
        {emailOpen && (
          <form className={styles.emailForm} onSubmit={onLogin}>
            <input
              className={styles.input}
              type="email"
              autoComplete="email"
              placeholder="이메일"
              aria-label="이메일"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoFocus
            />
            <input
              className={styles.input}
              type="password"
              autoComplete="current-password"
              placeholder="비밀번호"
              aria-label="비밀번호"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <button type="submit" className={styles.submit} disabled={!canLogin}>
              {login.isPending ? '로그인하는 중…' : '로그인'}
            </button>
            <Link to={`/reset-password${nextQuery}`} className={styles.forgot}>
              비밀번호를 잊으셨나요?
            </Link>
          </form>
        )}
      </div>
      <p className={styles.divider}>회원가입</p>
      {/* 카카오는 첫 로그인이 곧 가입이라 회원가입은 이메일만 둔다. */}
      <button
        type="button"
        className={`${styles.method} ${styles.signUp}`}
        onClick={() => navigate(`/sign-up${nextQuery}`)}
      >
        <UserPlus size={18} aria-hidden />
        이메일로 회원가입
      </button>
    </div>
  )
}
