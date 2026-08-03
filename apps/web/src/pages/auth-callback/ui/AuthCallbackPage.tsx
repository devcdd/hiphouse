import { useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { loginWithKakao, useAuth } from '@/entities/session'
import { kakaoRedirectUri } from '@/shared/config'
import styles from './AuthCallbackPage.module.css'

export function AuthCallbackPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { signIn } = useAuth()
  const [error, setError] = useState<string | null>(null)
  const ran = useRef(false)

  useEffect(() => {
    if (ran.current) return // codes are single-use; guard StrictMode double-run
    ran.current = true
    const code = params.get('code')
    if (!code) {
      setError('인증 코드가 없습니다.')
      return
    }
    loginWithKakao(code, kakaoRedirectUri())
      .then((res) => {
        signIn(res.token, res.refresh_token, res.user)
        // 닉네임을 아직 본인이 정하지 않았으면 인사 + 닉네임 설정부터.
        navigate(res.user.nickname_set ? '/' : '/welcome', { replace: true })
      })
      .catch((e: unknown) => setError(String(e)))
  }, [params, signIn, navigate])

  return (
    <div className={styles.page}>
      {error ? <p>로그인 실패: {error}</p> : <p>로그인 중…</p>}
    </div>
  )
}
