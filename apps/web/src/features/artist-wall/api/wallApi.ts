import { apiDelete, apiGet, apiPost } from '@/shared/api/client'

export interface WallPost {
  id: number
  user_id: string
  nickname: string
  body: string
  created_at: string
  artist_id: string | null // 작성자가 인증된 아티스트 본인이면 그 아티스트
  artist_name: string | null
}

export interface WallQuota {
  limit: number
  used: number
  unlimited: boolean // 이 담벼락 주인 아티스트로 인증된 본인은 제한이 없다
}

export const WALL_PAGE_SIZE = 50 // mirrors the API's wallPageSize
export const WALL_MAX_LEN = 50 // mirrors the API's maxWallPostLen

// 오래된 순으로 한 페이지. before를 주면 그 id보다 오래된 페이지.
export const fetchWallPosts = (artistId: string, before?: number): Promise<WallPost[]> =>
  apiGet<WallPost[]>(`/artists/${encodeURIComponent(artistId)}/wall`, { before })

export const addWallPost = (artistId: string, body: string) =>
  apiPost<WallPost>(`/artists/${encodeURIComponent(artistId)}/wall`, { body })

export const removeWallPost = (id: number) => apiDelete(`/wall/${id}`)

export const fetchWallQuota = (artistId: string) =>
  apiGet<WallQuota>(`/artists/${encodeURIComponent(artistId)}/wall/quota`)

export const fetchWallCount = (artistId: string) =>
  apiGet<{ count: number }>(`/artists/${encodeURIComponent(artistId)}/wall/count`)
