import type { Album, Track } from '@/entities/album'

/** 곡 수와 총 재생 시간(분). 트랙이 동기화 전이면 total_tracks를 쓰고, 길이가 하나라도 비면 minutes는 null. */
export function albumStats(album: Album, tracks: Track[]): { count: number; minutes: number | null } {
  const count = tracks.length || album.total_tracks || 0
  const timed = tracks.length > 0 && tracks.every((t) => t.duration_ms != null)
  const totalMs = tracks.reduce((sum, t) => sum + (t.duration_ms ?? 0), 0)
  return { count, minutes: timed ? Math.max(1, Math.round(totalMs / 60000)) : null }
}
