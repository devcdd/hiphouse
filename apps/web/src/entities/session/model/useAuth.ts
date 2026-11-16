import { useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchMe, logout } from '@/entities/session/api/authApi'
import type { User } from '@/entities/session/model/types'
import { useAdminMode } from './adminMode'
import { AUTH_CLEARED_EVENT, getRefreshToken, getToken, setTokens, clearToken } from '@/shared/lib/token'

// Current session, resolved from the stored token via GET /me.
//
// The token is read from localStorage on every render rather than mirrored into
// component state: useAuth runs in several components at once, and per-instance
// state would only update in whichever one called signIn — the header would keep
// showing 로그인 until something else forced it to re-render. The ['me'] cache is
// the shared signal instead; writing it re-renders every consumer, which then
// re-reads the token.
export function useAuth() {
  const qc = useQueryClient()
  const hasToken = !!getToken()
  const adminMode = useAdminMode()

  // The API layer can drop the session on its own (refresh token expired or
  // revoked). Clearing the cached user re-renders every consumer.
  useEffect(() => {
    const onCleared = () => qc.setQueryData(['me'], null)
    window.addEventListener(AUTH_CLEARED_EVENT, onCleared)
    return () => window.removeEventListener(AUTH_CLEARED_EVENT, onCleared)
  }, [qc])

  const { data: user, isLoading } = useQuery<User | null>({
    queryKey: ['me'],
    queryFn: fetchMe,
    enabled: hasToken,
    retry: false,
    staleTime: 5 * 60_000,
  })

  const signIn = (token: string, refreshToken: string, u: User) => {
    setTokens(token, refreshToken)
    qc.setQueryData(['me'], u)
  }

  const signOut = () => {
    // Fire and forget: revoking server-side is best effort, the local session
    // goes away either way.
    void logout(getRefreshToken())
    clearToken()
    qc.setQueryData(['me'], null)
    qc.removeQueries({ queryKey: ['favorites'] })
  }

  return {
    user: user ?? null,
    isAuthed: hasToken && !!user,
    // isAdmin은 관리 도구를 그릴지 판단하는 값이라 관리자 모드를 끄면 false가 된다.
    // 관리자 모드 스위치처럼 실제 권한이 필요한 곳은 isAdminUser를 쓴다.
    isAdmin: user?.role === 'admin' && adminMode,
    isAdminUser: user?.role === 'admin',
    isLoading: hasToken && isLoading,
    signIn,
    signOut,
  }
}
