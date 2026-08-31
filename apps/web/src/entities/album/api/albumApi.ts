import { apiGet, apiPut } from '@/shared/api/client'
import type { Album, Track } from '@/entities/album/model/types'

export const PAGE_SIZE = 40

export interface AlbumQuery {
  year?: number
  artistId?: string
  q?: string
  albumType?: string // 'single' | 'album'
  sort?: string // 'tracks' (default = recent)
  awarded?: boolean // 수상 경력이 있는 앨범만
  deletedOnly?: boolean // admin: only soft-deleted albums (server ignores for non-admins)
}

// One page of albums. All filters optional; default sort is newest year first.
export function fetchAlbums(params: AlbumQuery & { offset: number }): Promise<Album[]> {
  return apiGet<Album[]>('/albums', {
    year: params.year,
    artist_id: params.artistId,
    q: params.q,
    type: params.albumType,
    sort: params.sort,
    awarded: params.awarded ? '1' : undefined,
    deleted: params.deletedOnly ? 'only' : undefined,
    limit: PAGE_SIZE,
    offset: params.offset,
  })
}

export function fetchAlbum(id: string): Promise<Album> {
  return apiGet<Album>(`/albums/${encodeURIComponent(id)}`)
}

// Admin: 한글 표시 이름만 교체. 빈 문자열이면 해제되어 Spotify 원본명으로 돌아간다.
export function updateAlbumDisplayName(id: string, displayName: string): Promise<void> {
  return apiPut<void>(`/albums/${encodeURIComponent(id)}/display-name`, { display_name: displayName })
}

export interface AlbumInfoPatch {
  name: string
  release_date: string | null // YYYY | YYYY-MM | YYYY-MM-DD
  album_type: string | null // album | single | compilation
  total_tracks: number | null
}

// Admin: Spotify 메타 교정. 저장 후엔 재동기화가 이 앨범을 덮어쓰지 않는다.
export function updateAlbumInfo(id: string, body: AlbumInfoPatch): Promise<void> {
  return apiPut<void>(`/albums/${encodeURIComponent(id)}/info`, body)
}

// disc/track 순 정렬. 아직 동기화 전인 앨범은 빈 배열.
export function fetchAlbumTracks(albumId: string): Promise<Track[]> {
  return apiGet<Track[]>(`/albums/${encodeURIComponent(albumId)}/tracks`)
}

// Admin: 트랙 한글 표시 이름만 교체. 빈 문자열이면 해제.
export function updateTrackDisplayName(albumId: string, trackId: string, displayName: string): Promise<void> {
  return apiPut<void>(
    `/albums/${encodeURIComponent(albumId)}/tracks/${encodeURIComponent(trackId)}/display-name`,
    { display_name: displayName },
  )
}

// Distinct years present in the DB, newest first.
export function fetchYears(): Promise<number[]> {
  return apiGet<number[]>('/albums/years')
}
