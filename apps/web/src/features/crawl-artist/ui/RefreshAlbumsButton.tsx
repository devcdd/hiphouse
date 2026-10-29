import { RefreshCw } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { useCrawlArtist } from '@/features/crawl-artist/model/useCrawlArtist'
import styles from './RefreshAlbumsButton.module.css'

// Admin-only shortcut: same crawl as the admin 크롤링 탭, but right where the
// missing albums are noticed. Renders nothing for everyone else.
export function RefreshAlbumsButton({ artistId }: { artistId: string }) {
  const { isAdmin } = useAuth()
  const crawl = useCrawlArtist()

  if (!isAdmin) return null

  return (
    <div className={styles.wrap}>
      <button
        type="button"
        className={styles.btn}
        disabled={crawl.isPending}
        onClick={() => crawl.mutate({ artistId })}
      >
        <RefreshCw size={14} strokeWidth={2.2} className={crawl.isPending ? styles.spin : undefined} />
        {crawl.isPending ? '갱신 중…' : '앨범 목록 갱신'}
      </button>
    </div>
  )
}
