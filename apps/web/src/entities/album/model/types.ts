// Mirrors the Go `Album` struct (apps/api/albums.go).
export interface AlbumArtist {
  id: string
  name: string | null
  display_name: string | null // 관리자가 등록한 한글명 (없으면 name으로 표시)
  image_url: string | null
  genres: string[] | null
  spotify_url: string | null
}

// Mirrors the Go `Track` struct (apps/api/tracks.go).
export interface Track {
  id: string
  disc_number: number
  track_number: number
  name: string
  display_name: string | null // 관리자가 등록한 한글명 (없으면 name으로 표시)
  duration_ms: number | null
  explicit: boolean
  spotify_url: string | null
  artists: { id: string; name: string }[] // 피처링 포함, Spotify 크레딧 순서
}

export interface Album {
  id: string
  name: string
  // 한글 표시 이름 — 관리자만 수정 가능. Spotify는 유통사가 등록한 (대개 영문)
  // 이름만 주기 때문에 한글 제목은 여기서만 나온다. null이면 name으로 표시.
  display_name: string | null
  artists: AlbumArtist[] // every credited artist, ordered; from the album_artists join
  release_date: string | null
  year: number | null
  album_type: string | null
  total_tracks: number | null
  image_url: string | null
  spotify_url: string | null
  type_label: string | null // 싱글 | EP | 정규 (server-computed)
  rating_avg: number | null // 평균 별점 0..5 (아무도 평가 안 했으면 null)
  rating_count: number
  comment_count: number // 삭제되지 않은 댓글 수 — 카드에서 별점 옆에 노출

  deleted_at: string | null // soft-delete timestamp; only admins receive deleted rows
}
