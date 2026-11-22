import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { signupWithEmail, useAuth } from '@/entities/session'
import { EmailCodeFlow, afterSignInPath, returnPath } from '@/features/auth'
import { useDocumentTitle } from '@/shared/lib/useDocumentTitle'

// /sign-up?next=... — 이메일 가입. 인증 코드를 받은 뒤 코드와 비밀번호를 함께 보내야 계정이 생긴다.
export function SignUpPage() {
  useDocumentTitle('회원가입')
  const { isAuthed, isLoading, signIn } = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = returnPath(params.get('next'))

  if (isLoading) return null
  if (isAuthed) return <Navigate to={next} replace />

  return (
    <EmailCodeFlow
      purpose="signup"
      title="회원가입"
      lead="이메일로 받은 인증 코드를 입력하면 가입이 끝나요."
      passwordLabel="비밀번호"
      submitLabel="가입하기"
      pendingLabel="가입하는 중…"
      submit={signupWithEmail}
      onDone={(res) => {
        signIn(res.token, res.refresh_token, res.user)
        navigate(afterSignInPath(res.user, next), { replace: true })
      }}
      footer={
        <>
          이미 계정이 있나요? <Link to={`/sign-in?next=${encodeURIComponent(next)}`}>로그인</Link>
        </>
      }
    />
  )
}
