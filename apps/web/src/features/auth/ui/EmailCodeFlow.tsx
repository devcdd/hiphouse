import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { useMutation } from '@tanstack/react-query'
import { requestEmailCode, type EmailCodePurpose, type Session } from '@/entities/session'
import { useToast } from '@/shared/ui/toast'
import styles from './EmailCodeFlow.module.css'

const PASSWORD_MIN = 8

interface Props {
  purpose: EmailCodePurpose
  title: string
  lead: string
  passwordLabel: string
  submitLabel: string
  pendingLabel: string
  submit: (email: string, password: string, code: string) => Promise<Session>
  onDone: (session: Session) => void
  footer?: ReactNode
}

// 이메일로 인증 코드를 받고, 코드와 비밀번호를 함께 보내는 화면. 가입과 비밀번호 재설정이 같이 쓴다.
export function EmailCodeFlow({ purpose, title, lead, passwordLabel, submitLabel, pendingLabel, submit, onDone, footer }: Props) {
  const toast = useToast()
  const [email, setEmail] = useState('')
  const [sentTo, setSentTo] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [resendAt, setResendAt] = useState(0)
  const now = useNow(resendAt > Date.now())

  const sendCode = useMutation({
    mutationFn: (to: string) => requestEmailCode(to, purpose),
    onSuccess: (res, sent) => {
      setSentTo(sent)
      setResendAt(Date.now() + res.resend_after * 1000)
      toast(`${sent}로 인증 코드를 보냈습니다`)
    },
    onError: (e: Error) => toast(e.message || '인증 코드를 보내지 못했습니다', 'error'),
  })

  const finish = useMutation({
    mutationFn: () => submit(sentTo ?? '', password, code),
    onSuccess: onDone,
    onError: (e: Error) => toast(e.message || '요청에 실패했습니다', 'error'),
  })

  const waitSec = Math.max(0, Math.ceil((resendAt - now) / 1000))
  const trimmedEmail = email.trim()
  const canSend = trimmedEmail !== '' && waitSec === 0 && !sendCode.isPending
  const canFinish = code.length === 6 && password.length >= PASSWORD_MIN && !finish.isPending

  const onSend = (e: FormEvent) => {
    e.preventDefault()
    if (canSend) sendCode.mutate(trimmedEmail)
  }
  const onFinish = (e: FormEvent) => {
    e.preventDefault()
    if (canFinish) finish.mutate()
  }
  const changeEmail = () => {
    setSentTo(null)
    setCode('')
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{title}</h1>
      <p className={styles.lead}>{lead}</p>

      <form className={styles.card} onSubmit={onSend}>
        <label className={styles.label} htmlFor="code-flow-email">
          이메일
        </label>
        <div className={styles.row}>
          <input
            id="code-flow-email"
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
        <form className={styles.card} onSubmit={onFinish}>
          <label className={styles.label} htmlFor="code-flow-code">
            인증 코드
          </label>
          <div className={styles.row}>
            <input
              id="code-flow-code"
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

          <label className={styles.label} htmlFor="code-flow-password">
            {passwordLabel}
          </label>
          <input
            id="code-flow-password"
            className={styles.input}
            type="password"
            autoComplete="new-password"
            placeholder={`${PASSWORD_MIN}자 이상`}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />

          <button type="submit" className={`${styles.primary} ${styles.submit}`} disabled={!canFinish}>
            {finish.isPending ? pendingLabel : submitLabel}
          </button>
        </form>
      )}

      {footer && <p className={styles.footer}>{footer}</p>}
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
