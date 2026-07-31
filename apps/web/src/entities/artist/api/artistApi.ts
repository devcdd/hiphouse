import { apiDelete, apiGet, apiPost, apiPut } from '@/shared/api/client'
import type { Artist } from '@/entities/artist/model/types'

export const ARTIST_PAGE_SIZE = 40

export function fetchArtist(id: string): Promise<Artist> {
  return apiGet<Artist>(`/artists/${encodeURIComponent(id)}`)
}

export function fetchArtists(params: { q?: string; offset: number }): Promise<Artist[]> {
  return apiGet<Artist[]>('/artists', { q: params.q, limit: ARTIST_PAGE_SIZE, offset: params.offset })
}

// Replace just the 연관검색어 (admin-only; other fields stay crawler-owned).
export function updateArtistAliases(id: string, aliases: string[]): Promise<Artist> {
  return apiPut<Artist>(`/artists/${encodeURIComponent(id)}/aliases`, { aliases })
}

// Admin: 한글 표시 이름만 교체. 빈 문자열이면 해제되어 Spotify 원본명으로 돌아간다.
export function updateArtistDisplayName(id: string, displayName: string): Promise<void> {
  return apiPut<void>(`/artists/${encodeURIComponent(id)}/display-name`, { display_name: displayName })
}

// Admin: 신보 감시 플래그만 교체 — 신보 체크 탭이 확인하는 대상을 결정한다.
export function updateArtistReleasesWatch(id: string, watch: boolean): Promise<void> {
  return apiPut<void>(`/artists/${encodeURIComponent(id)}/releases-watch`, { watch })
}

export type DeleteMode = 'soft' | 'hard'

// Admin: 'soft' keeps the artist and only soft-deletes the albums they are the
// sole credit on; 'hard' removes the artist and every album they appear on.
// Returns how many albums were affected.
export function deleteArtist(id: string, mode: DeleteMode): Promise<{ mode: DeleteMode; albums: number }> {
  return apiDelete<{ mode: DeleteMode; albums: number }>(`/artists/${encodeURIComponent(id)}?mode=${mode}`)
}

// Admin: fold mergedIds into masterId — credits move over, names/aliases become
// the master's aliases, duplicates are deleted.
export function mergeArtists(masterId: string, mergedIds: string[]): Promise<Artist> {
  return apiPost<Artist>('/artists/merge', { master_id: masterId, merged_ids: mergedIds })
}
