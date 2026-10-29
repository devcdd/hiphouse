import { apiGet } from '@/shared/api/client'

export interface AppleArtistHit {
  id: string // Apple 아티스트 id
  name: string
  image_url: string | null
  genres: string[]
  artist_id: string | null // 연결된 DB 아티스트 id — 없으면 아직 미등록
  albums_in_db: number
}

export function searchAppleArtists(q: string): Promise<AppleArtistHit[]> {
  return apiGet<AppleArtistHit[]>('/admin/apple/artists', { q })
}

export interface AppleAlbumHit {
  id: string // Apple 앨범 id
  name: string
  image_url: string | null
  release_date: string | null
  album_type: 'album' | 'ep' | 'single'
  total_tracks: number
  artists: string
  in_db: boolean
  deleted: boolean // DB에 있지만 소프트 삭제된 상태
}

export function searchAppleAlbums(q: string): Promise<AppleAlbumHit[]> {
  return apiGet<AppleAlbumHit[]>('/admin/apple/albums', { q })
}
