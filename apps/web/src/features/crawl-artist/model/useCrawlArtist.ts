import { useMutation, useQueryClient } from '@tanstack/react-query'
import { crawlAppleArtist, type CrawlResult } from '@/features/crawl-artist/api/crawlApi'
import { useToast } from '@/shared/ui/toast'

// Re-crawls one artist and refreshes anything that renders albums.
export function useCrawlArtist(onDone?: (r: CrawlResult) => void) {
  const qc = useQueryClient()
  const toast = useToast()
  return useMutation({
    mutationFn: ({ artistId }: { artistId: string }) => crawlAppleArtist({ artistId }),
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ['albums'] })
      qc.invalidateQueries({ queryKey: ['years'] })
      qc.invalidateQueries({ queryKey: ['artists'] })
      toast(result.albums === 0 ? '새로 가져올 앨범이 없습니다' : `앨범 ${result.albums}장을 가져왔습니다`)
      onDone?.(result)
    },
    onError: (e) => toast(`갱신 실패: ${String(e)}`, 'error'),
  })
}
