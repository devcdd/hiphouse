import { apiGet, apiPost, apiPut } from '@/shared/api/client'
import type { User } from '@/entities/session/model/types'

export function fetchMe(): Promise<User> {
  return apiGet<User>('/me')
}

// PUT /me is a partial update — send only the field being changed.
export function updateNickname(nickname: string): Promise<User> {
  return apiPut<User>('/me', { nickname })
}

// 프로필 공개 여부 토글. 끄면 공개 프로필에 닉네임과 가입일만 남는다.
export function updateProfilePublic(profile_public: boolean): Promise<User> {
  return apiPut<User>('/me', { profile_public })
}

export interface Session {
  token: string
  refresh_token: string
  user: User
}

export function loginWithKakao(code: string, redirectUri: string): Promise<Session> {
  return apiPost('/auth/kakao', { code, redirect_uri: redirectUri })
}

// Revokes the refresh token server-side. Never rejects — signing out locally
// must not depend on the network.
export function logout(refreshToken: string | null): Promise<void> {
  return apiPost<void>('/auth/logout', { refresh_token: refreshToken }).catch(() => undefined)
}
