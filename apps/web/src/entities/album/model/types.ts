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

export interface AlbumAward {
  id: number
  host: string // 주최 코드 (KHA, KMA …) — 라벨은 shared/lib/awardHosts
  name: string
  year: number | null
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
  apple_id: string | null // Apple Music 카탈로그 앨범 id — UPC 매칭으로 연결, 미연결이면 null
  label: string | null // Apple recordLabel (Spotify label 필드는 2026-02 개편으로 죽음)
  genres: string[] | null // Apple 앨범 장르 (루트 "음악" 제외)
  copyright: string | null // ℗ 한 줄 (Apple)
  content_rating: string | null // "explicit" | "clean" | null (Apple)
  editorial_notes: string | null // Apple 에디토리얼 노트 평문, 줄바꿈 포함
  type_label: string | null // 싱글 | EP | 정규 (server-computed)
  rating_avg: number | null // 평균 별점 0..5 (아무도 평가 안 했으면 null)
  rating_count: number
  comment_count: number // 삭제되지 않은 댓글 수 — 카드에서 별점 옆에 노출
  awards: AlbumAward[] // 수상 경력, 연도 내림차순 — 카드 하단 태그

  deleted_at: string | null // soft-delete timestamp; only admins receive deleted rows
}
