import { KAKAO_REST_KEY, kakaoRedirectUri } from '@/shared/config'

// OAuth state로 되돌아온 경로를 그대로 믿지 않는다 — 같은 앱 안의 절대 경로만 허용
// ('//evil.com' 같은 protocol-relative URL 차단). 아니면 홈.
export function returnPath(raw: string | null): string {
  return raw && raw.startsWith('/') && !raw.startsWith('//') ? raw : '/'
}

// Redirect to Kakao's consent screen; it returns to /auth/kakao/callback?code=...&state=...
// state에 next를 실어 로그인 후 로그인을 시작한 페이지로 복귀한다.
export function startKakaoLogin(next: string) {
  if (!KAKAO_REST_KEY) {
    alert('카카오 로그인이 설정되지 않았습니다 (VITE_KAKAO_REST_API_KEY).')
    return
  }
  const u = new URL('https://kauth.kakao.com/oauth/authorize')
  u.searchParams.set('client_id', KAKAO_REST_KEY)
  u.searchParams.set('redirect_uri', kakaoRedirectUri())
  u.searchParams.set('response_type', 'code')
  u.searchParams.set('state', returnPath(next))
  window.location.href = u.toString()
}
