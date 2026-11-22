import { useLocation } from 'react-router-dom'
import { useAuth } from '@/entities/session'
import { returnPath } from './kakao'

// 로그인 화면들은 자기 ?next=를 들고 있어서, 거기서 온 경우엔 그 next를 이어받는다.
const SIGN_IN_PAGES = new Set(['/sign-in', '/sign-up', '/reset-password'])
// 콜백은 로그인을 끝내는 도중이라 스스로 /welcome으로 보낸다.
const EXEMPT = new Set(['/welcome', '/auth/kakao/callback'])

// 로그인했지만 닉네임을 아직 정하지 않았으면 보내야 할 /welcome 경로를, 아니면 null을 돌려준다.
// 닉네임 없이 다른 화면을 쓰면 댓글·평점에 빈 이름이 남으므로 어느 화면에서든 /welcome으로 돌려보낸다.
export function useNicknameRedirect(): string | null {
  const { user, isAuthed } = useAuth()
  const { pathname, search, hash } = useLocation()
  if (!isAuthed || !user || user.nickname_set || EXEMPT.has(pathname)) return null
  const next = SIGN_IN_PAGES.has(pathname)
    ? returnPath(new URLSearchParams(search).get('next'))
    : pathname + search + hash
  return `/welcome?next=${encodeURIComponent(next)}`
}
