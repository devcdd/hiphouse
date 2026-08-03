import { apiGet } from '@/shared/api/client'
import type { Album } from '@/entities/album'

export interface PublicUser {
  id: string
  nickname: string
  created_at: string
  profile_public: boolean // false = 본인이 프로필을 비공개로 설정 → 아래 집계는 전부 0/null
  rating_count: number
  rating_avg: number | null // 별점 0..5, 평가가 없으면 null
  comment_count: number
}

// 앨범 카드 + 이 프로필 주인이 준 점수(하프스타 1..10).
export type RatedAlbum = Album & { score: number }

export function fetchPublicUser(id: string): Promise<PublicUser> {
  return apiGet<PublicUser>(`/users/${encodeURIComponent(id)}`)
}

export function fetchPublicRatedAlbums(id: string): Promise<RatedAlbum[]> {
  return apiGet<RatedAlbum[]>(`/users/${encodeURIComponent(id)}/ratings`, { limit: 60 })
}
