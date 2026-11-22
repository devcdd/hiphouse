import { apiDelete, apiGet, apiPost, apiPut, apiPutBlob } from '@/shared/api/client'
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

// 서버는 256KB, 1024px까지만 받는다. 원본 사진은 resizeAvatar로 줄여서 보낸다.
export function uploadAvatar(image: Blob): Promise<User> {
  return apiPutBlob<User>('/me/avatar', image)
}

export function deleteAvatar(): Promise<User> {
  return apiDelete<User>('/me/avatar')
}

export interface Session {
  token: string
  refresh_token: string
  user: User
}

export function loginWithKakao(code: string, redirectUri: string): Promise<Session> {
  return apiPost('/auth/kakao', { code, redirect_uri: redirectUri })
}

export interface EmailCodeSent {
  expires_in: number // 초
  resend_after: number // 초
}

export type EmailCodePurpose = 'signup' | 'reset'

// 이메일 인증 코드를 메일로 보낸다. signup은 이미 가입된 이메일이면 409, reset은 이메일 계정이 없으면 404.
export function requestEmailCode(email: string, purpose: EmailCodePurpose = 'signup'): Promise<EmailCodeSent> {
  return apiPost('/auth/email/code', { email, purpose })
}

export function signupWithEmail(email: string, password: string, code: string): Promise<Session> {
  return apiPost('/auth/email/signup', { email, password, code })
}

export function loginWithEmail(email: string, password: string): Promise<Session> {
  return apiPost('/auth/email/login', { email, password })
}

// 비밀번호를 바꾸면 다른 기기의 로그인은 모두 끊기고, 이 요청에 새 세션이 온다.
export function resetPassword(email: string, password: string, code: string): Promise<Session> {
  return apiPost('/auth/email/reset', { email, password, code })
}

// Revokes the refresh token server-side. Never rejects — signing out locally
// must not depend on the network.
export function logout(refreshToken: string | null): Promise<void> {
  return apiPost<void>('/auth/logout', { refresh_token: refreshToken }).catch(() => undefined)
}
