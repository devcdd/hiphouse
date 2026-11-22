import { useSearchParams } from 'react-router-dom'
import { AlbumFeed } from '@/widgets/album-feed'
import { ArtistGrid } from '@/widgets/artist-grid'
import styles from './SearchPage.module.css'
import { useDocumentTitle } from '@/shared/lib/useDocumentTitle'

type Tab = 'album' | 'artist'
const TABS: { key: Tab; label: string }[] = [
  { key: 'album', label: '앨범' },
  { key: 'artist', label: '아티스트' },
]

export function SearchPage() {
  const [params, setParams] = useSearchParams()
  const q = params.get('q') ?? ''
  const tab: Tab = params.get('type') === 'artist' ? 'artist' : 'album'
  useDocumentTitle(q ? `'${q}' 검색` : '검색')

  const setTab = (t: Tab) => setParams({ q, type: t }, { replace: true })

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>
        “{q}” 검색 결과
      </h1>

      <div className={styles.tabs} role="tablist">
        {TABS.map((t) => (
          <button
            key={t.key}
            role="tab"
            aria-selected={t.key === tab}
            className={t.key === tab ? `${styles.tab} ${styles.active}` : styles.tab}
            onClick={() => setTab(t.key)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {q === '' ? (
        <p className={styles.state}>검색어를 입력하세요.</p>
      ) : tab === 'album' ? (
        // 서버가 이름이 잘 맞은 순서로 먼저 세운다. sort는 같은 순위 안의 순서를 정한다.
        // 평가가 많은 앨범이 실제로 찾는 앨범일 확률이 높아서 인기순으로 둔다.
        <AlbumFeed params={{ q, sort: 'popular' }} />
      ) : (
        <ArtistGrid q={q} />
      )}
    </div>
  )
}
