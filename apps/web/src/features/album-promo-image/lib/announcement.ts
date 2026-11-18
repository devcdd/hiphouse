import type { Album, Track } from '@/entities/album'
import { displayName } from '@/shared/lib/displayName'
import { albumStats } from './albumStats'

const SITE = 'https://hiphouse.kr'

// 인스타그램 게시글 본문. 곡 수나 재생 시간을 모르면(트랙 동기화 전) 그 항목만 뺀다.
export function announcementText(album: Album, tracks: Track[]): string {
  const artist = album.artists.map(displayName).join(', ')
  const { count, minutes } = albumStats(album, tracks)
  const stats = [count > 0 ? `${count}곡` : '', minutes != null ? `${minutes}분` : ''].filter(Boolean).join(' · ')

  return [
    `${artist} – ${displayName(album)}`,
    ...(stats ? [stats] : []),
    '',
    '힙합 앨범 커뮤니티, 힙집에서 평가를 남겨주세요.',
    `${SITE}/albums/${encodeURIComponent(album.id)}`,
  ].join('\n')
}
