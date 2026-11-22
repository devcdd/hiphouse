import { useEffect, useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { requestEmailCode, signupWithEmail, useAuth } from '@/entities/session'
import { returnPath } from '@/features/auth'
import { useToast } from '@/shared/ui/toast'
import styles from './SignUpPage.module.css'

const PASSWORD_MIN = 8

// /sign-up?next=... — 이메일 가입. 인증 코드를 받은 뒤 코드와 비밀번호를 함께 보내야 계정이 생긴다.
export function SignUpPage() {
  const { isAuthed, isLoading, signIn } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = returnPath(params.get('next'))

  const [email, setEmail] = useState('')
  const [sentTo, setSentTo] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [resendAt, setResendAt] = useState(0)
  const now = useNow(resendAt > Date.now())

  const sendCode = useMutation({
    mutationFn: requestEmailCode,
    onSuccess: (res, sent) => {
      setSentTo(sent)
      setResendAt(Date.now() + res.resend_after * 1000)
      toast(`${sent}로 인증 코드를 보냈습니다`)
    },
    onError: (e: Error) => toast(e.message || '인증 코드를 보내지 못했습니다', 'error'),
  })

  const signUp = useMutation({
    mutationFn: () => signupWithEmail(sentTo ?? '', password, code),
    onSuccess: (res) => {
      signIn(res.token, res.refresh_token, res.user)
      navigate(res.user.nickname_set ? next : `/welcome?next=${encodeURIComponent(next)}`, { replace: true })
    },
    onError: (e: Error) => toast(e.message || '가입에 실패했습니다', 'error'),
  })

  if (isLoading) return <div className={styles.page} />
  if (isAuthed) return <Navigate to={next} replace />

  const waitSec = Math.max(0, Math.ceil((resendAt - now) / 1000))
  const trimmedEmail = email.trim()
  const canSend = trimmedEmail !== '' && waitSec === 0 && !sendCode.isPending
  const canSignUp = code.length === 6 && password.length >= PASSWORD_MIN && !signUp.isPending

  const onSend = (e: FormEvent) => {
    e.preventDefault()
    if (canSend) sendCode.mutate(trimmedEmail)
  }
  const onSignUp = (e: FormEvent) => {
    e.preventDefault()
    if (canSignUp) signUp.mutate()
  }
  const changeEmail = () => {
    setSentTo(null)
    setCode('')
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>회원가입</h1>
      <p className={styles.lead}>이메일로 받은 인증 코드를 입력하면 가입이 끝나요.</p>

      <form className={styles.card} onSubmit={onSend}>
        <label className={styles.label} htmlFor="signup-email">
          이메일
        </label>
        <div className={styles.row}>
          <input
            id="signup-email"
            className={styles.input}
            type="email"
            autoComplete="email"
            placeholder="you@example.com"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            readOnly={sentTo !== null}
            autoFocus
          />
          {sentTo === null ? (
            <button type="submit" className={styles.primary} disabled={!canSend}>
              {sendCode.isPending ? '보내는 중…' : '인증 코드 받기'}
            </button>
          ) : (
            <button type="button" className={styles.secondary} onClick={changeEmail}>
              변경
            </button>
          )}
        </div>
      </form>

      {sentTo !== null && (
        <form className={styles.card} onSubmit={onSignUp}>
          <label className={styles.label} htmlFor="signup-code">
            인증 코드
          </label>
          <div className={styles.row}>
            <input
              id="signup-code"
              className={`${styles.input} ${styles.code}`}
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="6자리 숫자"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              autoFocus
            />
            <button
              type="button"
              className={styles.secondary}
              disabled={waitSec > 0 || sendCode.isPending}
              onClick={() => sendCode.mutate(sentTo)}
            >
              {waitSec > 0 ? `다시 받기 ${waitSec}초` : '다시 받기'}
            </button>
          </div>
          <p className={styles.hint}>메일이 안 보이면 스팸함도 확인해 주세요. 코드는 10분 동안 쓸 수 있어요.</p>

          <label className={styles.label} htmlFor="signup-password">
            비밀번호
          </label>
          <input
            id="signup-password"
            className={styles.input}
            type="password"
            autoComplete="new-password"
            placeholder={`${PASSWORD_MIN}자 이상`}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />

          <button type="submit" className={`${styles.primary} ${styles.submit}`} disabled={!canSignUp}>
            {signUp.isPending ? '가입하는 중…' : '가입하기'}
          </button>
        </form>
      )}

      <p className={styles.footer}>
        이미 계정이 있나요? <Link to={`/sign-in?next=${encodeURIComponent(next)}`}>로그인</Link>
      </p>
    </div>
  )
}

// 재발송 대기 초를 화면에 갱신하려고 active인 동안만 1초마다 다시 그린다.
function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    setNow(Date.now())
    if (!active) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [active])
  return now
}
