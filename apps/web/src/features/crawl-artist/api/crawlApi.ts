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
  existing?: number // Apple 크롤: 이미 DB에 있어 건너뛴 앨범 수
}

// Admin: pull an artist's Spotify discography into the DB.
// key '' = the server's default credentials pair.
// appearsOn=true면 참여·컴필레이션 앨범(쇼미더머니류)까지 포함 — 노이즈가 많아 기본은 꺼짐.
export function crawlArtist(artistId: string, key = '', appearsOn = false): Promise<CrawlResult> {
  return apiPost<CrawlResult>('/admin/spotify/crawl', { artist_id: artistId, key, appears_on: appearsOn })
}

export interface CrawlAlbumResult {
  album_id: string
  album_name: string
  saved: boolean
  artists?: number
  enriched?: number
  tracks_synced?: number
  deleted?: boolean // 행은 있으나 소프트 삭제 상태 — 삭제된 앨범 탭에서 복구해야 노출됨
  existing?: boolean // Apple 추가: 이미 DB에 있어 저장하지 않음
}

// Admin: 앨범 검색에서 고른 앨범 1개를 DB에 추가.
export function crawlAlbum(albumId: string, key = ''): Promise<CrawlAlbumResult> {
  return apiPost<CrawlAlbumResult>('/admin/spotify/crawl-album', { album_id: albumId, key })
}

// Spotify 계정이 막힌 동안 관리자 크롤링은 Apple Music으로 한다. appleArtistId(검색 결과) 또는
// artistId(DB id — 그 아티스트의 apple_id를 씀) 중 하나를 넘긴다. 이미 있는 앨범은 건너뛴다.
export function crawlAppleArtist(
  target: { appleArtistId: string } | { artistId: string },
  appearsOn = false,
): Promise<CrawlResult> {
  const id = 'appleArtistId' in target ? { apple_artist_id: target.appleArtistId } : { artist_id: target.artistId }
  return apiPost<CrawlResult>('/admin/apple/crawl', { ...id, appears_on: appearsOn })
}

export function crawlAppleAlbum(appleAlbumId: string): Promise<CrawlAlbumResult> {
  return apiPost<CrawlAlbumResult>('/admin/apple/crawl-album', { album_id: appleAlbumId })
}
