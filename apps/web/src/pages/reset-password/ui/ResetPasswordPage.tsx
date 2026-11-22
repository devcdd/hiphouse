import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { resetPassword, useAuth } from '@/entities/session'
import { EmailCodeFlow, afterSignInPath, returnPath } from '@/features/auth'
import { useToast } from '@/shared/ui/toast'

// /reset-password?next=... — 이메일 가입자의 비밀번호 재설정. 끝나면 바로 로그인된 상태로 next로 간다.
export function ResetPasswordPage() {
  const { isAuthed, isLoading, signIn } = useAuth()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const next = returnPath(params.get('next'))

  if (isLoading) return null
  if (isAuthed) return <Navigate to={next} replace />

  return (
    <EmailCodeFlow
      purpose="reset"
      title="비밀번호 재설정"
      lead="가입한 이메일로 인증 코드를 받아 새 비밀번호를 정해 주세요."
      passwordLabel="새 비밀번호"
      submitLabel="비밀번호 바꾸기"
      pendingLabel="바꾸는 중…"
      submit={resetPassword}
      onDone={(res) => {
        signIn(res.token, res.refresh_token, res.user)
        toast('비밀번호를 바꿨습니다. 다른 기기의 로그인은 모두 끊겼어요.')
        navigate(afterSignInPath(res.user, next), { replace: true })
      }}
      footer={<Link to={`/sign-in?next=${encodeURIComponent(next)}`}>로그인으로 돌아가기</Link>}
    />
  )
}
