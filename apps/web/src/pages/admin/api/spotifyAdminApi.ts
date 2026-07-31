import { apiGet } from '@/shared/api/client'

import type { SpotifyKey } from '@/features/crawl-artist'

export interface SpotifyArtistHit {
  id: string
  name: string
  image_url: string | null
  followers: number
  albums_in_db: number // how many albums WE already have — free local lookup
}

export function searchSpotifyArtists(q: string, key: SpotifyKey): Promise<SpotifyArtistHit[]> {
  return apiGet<SpotifyArtistHit[]>('/admin/spotify/artists', { q, key })
}

export interface SpotifyAlbumHit {
  id: string
  name: string
  image_url: string | null
  release_date: string | null
  album_type: string | null
  total_tracks: number
  artists: string // 크레딧 아티스트명 쉼표 연결
  in_db: boolean
  deleted: boolean // DB에 있지만 소프트 삭제된 상태
}

// 앨범명 검색. Spotify는 앨범 단위 장르가 없어서 결과가 장르 무관 — 관리자가 직접 골라 담는다.
export function searchSpotifyAlbums(q: string, key: SpotifyKey): Promise<SpotifyAlbumHit[]> {
  return apiGet<SpotifyAlbumHit[]>('/admin/spotify/albums', { q, key })
}
