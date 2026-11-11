import { apiDelete, apiGet, apiPut } from '@/shared/api/client'

export type ClaimStatus = 'pending' | 'approved' | 'rejected'

// Mirrors the Go `ArtistClaim` struct (apps/api/claims.go).
export interface ArtistClaim {
  user_id: string
  nickname: string
  artist_id: string
  artist_name: string | null
  artist_display_name: string | null
  artist_image_url: string | null
  status: ClaimStatus
  note: string
  created_at: string
  reviewed_at: string | null
}

export const CLAIM_NOTE_MAX = 200 // mirrors the API's maxClaimNoteLen

export const fetchMyClaim = () => apiGet<ArtistClaim | null>('/me/artist-claim')

export const submitMyClaim = (artistId: string, note: string) =>
  apiPut<ArtistClaim>('/me/artist-claim', { artist_id: artistId, note })

export const withdrawMyClaim = () => apiDelete('/me/artist-claim')

export const fetchClaims = (status: ClaimStatus) => apiGet<ArtistClaim[]>('/admin/artist-claims', { status })

// 관리자 검수. artistId를 주면 신청자가 고른 아티스트 대신 그 아티스트로 매핑한다.
export const reviewClaim = (userId: string, status: 'approved' | 'rejected', artistId?: string) =>
  apiPut<ArtistClaim>(`/admin/artist-claims/${encodeURIComponent(userId)}`, { status, artist_id: artistId ?? null })

// 신청 행의 아티스트를 화면 이름 규칙(한글명 > 원본명 > id)대로.
export const claimArtistName = (c: Pick<ArtistClaim, 'artist_id' | 'artist_name' | 'artist_display_name'>) =>
  c.artist_display_name || c.artist_name || c.artist_id
