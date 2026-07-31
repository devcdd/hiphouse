// Mirrors the Go `Artist` struct (apps/api/artists.go).
export interface Artist {
  id: string
  name: string | null
  display_name: string | null // 관리자가 등록한 한글명 (없으면 name으로 표시)
  image_url: string | null
  genres: string[] | null
  spotify_url: string | null
  aliases: string[] | null // 연관검색어 — admin-curated search keywords, matched alongside name
  followers: number | null // Spotify 팔로워 수 — 크롤러/enrich가 채움, 수집 전이면 null (화면 미표시)
  releases_watch: boolean // 신보 감시 대상 — 신보 체크가 이 플래그 켜진 아티스트만 확인 (관리자 토글)
  follower_count: number // 서비스 내부 팔로워 수 — follows 테이블 집계
}
