import { apiGet, apiPost } from '@/shared/api/client'

// Which Spotify app credentials the server should use. Any <PREFIX>_SPOTIFY_CLIENT_*
// env pair on the server is a valid key ("first", "second", "third", ...);
// fetchSpotifyKeys reports the configured ones. '' = server default pair.
export type SpotifyKey = string

export function fetchSpotifyKeys(): Promise<SpotifyKey[]> {
  return apiGet<SpotifyKey[]>('/admin/spotify/keys')
}

export interface CrawlResult {
  albums: number
  saved: boolean
  artist_name?: string | null
  artists?: number
  enriched?: number
  tracks_synced?: number // 트랙까지 새로 받아온 앨범 수 (미동기화분만)
}

// Admin: pull an artist's Spotify discography into the DB.
// key '' = the server's default credentials pair.
export function crawlArtist(artistId: string, key = ''): Promise<CrawlResult> {
  return apiPost<CrawlResult>('/admin/spotify/crawl', { artist_id: artistId, key })
}

export interface CrawlAlbumResult {
  album_id: string
  album_name: string
  saved: boolean
  artists?: number
  enriched?: number
  tracks_synced?: number
  deleted?: boolean // 행은 있으나 소프트 삭제 상태 — 삭제된 앨범 탭에서 복구해야 노출됨
}

// Admin: 앨범 검색에서 고른 앨범 1개를 DB에 추가.
export function crawlAlbum(albumId: string, key = ''): Promise<CrawlAlbumResult> {
  return apiPost<CrawlAlbumResult>('/admin/spotify/crawl-album', { album_id: albumId, key })
}
