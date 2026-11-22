import { useLocation, useNavigate } from 'react-router-dom'

// 로그인 수단을 고르는 /sign-in으로 보낸다. 로그인이 끝나면 지금 보던 페이지로 돌아온다.
export function useStartSignIn() {
  const navigate = useNavigate()
  const { pathname, search, hash } = useLocation()
  return () => navigate(`/sign-in?next=${encodeURIComponent(pathname + search + hash)}`)
}
