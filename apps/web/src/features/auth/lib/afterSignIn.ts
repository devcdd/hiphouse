import type { User } from '@/entities/session'

// 닉네임을 아직 본인이 정하지 않았으면 인사 + 닉네임 설정부터 보낸다(끝나면 next로).
export function afterSignInPath(user: User, next: string): string {
  return user.nickname_set ? next : `/welcome?next=${encodeURIComponent(next)}`
}
