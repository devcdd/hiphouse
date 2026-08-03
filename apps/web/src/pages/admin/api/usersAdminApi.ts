import { apiGet } from '@/shared/api/client'

// Mirrors the Go `AdminUser` struct (apps/api/users.go).
export interface AdminUser {
  id: string
  nickname: string
  role: string // 'admin' | 'user'
  created_at: string
  rating_count: number
  comment_count: number
  favorite_count: number
  follow_count: number
}

export const USERS_PAGE_SIZE = 50

// 가입 최신순. q는 닉네임/카카오 ID 부분 일치.
export function fetchAdminUsers(q: string, offset: number): Promise<AdminUser[]> {
  return apiGet<AdminUser[]>('/admin/users', { q, limit: USERS_PAGE_SIZE, offset })
}
