import { useQuery } from '@tanstack/react-query'
import { fetchAlbumTracks } from '@/entities/album/api/albumApi'

// 트랙리스트는 기본 접힘 — enabled로 처음 펼칠 때 불러온다.
export function useAlbumTracks(albumId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['album-tracks', albumId],
    queryFn: () => fetchAlbumTracks(albumId),
    enabled: enabled && albumId !== '',
  })
}
