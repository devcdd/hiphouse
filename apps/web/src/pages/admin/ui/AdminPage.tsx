import { useSearchParams } from 'react-router-dom'
import { Swiper, SwiperSlide } from 'swiper/react'
import { FreeMode, Mousewheel } from 'swiper/modules'
import { useAuth } from '@/entities/session'
import { DeletedAlbumsTab } from './DeletedAlbumsTab'
import { AliasManagerTab } from './AliasManagerTab'
import { CrawlTab } from './CrawlTab'
import { TracksTab } from './TracksTab'
import { ReleasesTab } from './ReleasesTab'
import { StatsTab } from './StatsTab'
import { ReportsTab } from './ReportsTab'
import { RenameRequestsTab } from './RenameRequestsTab'
import 'swiper/css'
import 'swiper/css/free-mode'
import 'swiper/css/mousewheel'
import styles from './AdminPage.module.css'

type Tab = 'deleted' | 'aliases' | 'crawl' | 'tracks' | 'releases' | 'stats' | 'reports' | 'rename'
const TABS: { key: Tab; label: string }[] = [
  { key: 'crawl', label: '크롤링' },
  { key: 'releases', label: '신보 체크' },
  { key: 'tracks', label: '트랙 동기화' },
  { key: 'aliases', label: '연관검색어' },
  { key: 'deleted', label: '삭제된 앨범' },
  { key: 'reports', label: '힙합 아님 신고' },
  { key: 'rename', label: '이름 변경 요청' },
  { key: 'stats', label: '통계' },
]

export function AdminPage() {
  const { isAdmin, isLoading } = useAuth()
  const [params, setParams] = useSearchParams()
  const raw = params.get('tab')
  const tab: Tab =
    raw === 'aliases' ||
    raw === 'deleted' ||
    raw === 'stats' ||
    raw === 'reports' ||
    raw === 'tracks' ||
    raw === 'releases' ||
    raw === 'rename'
      ? raw
      : 'crawl'

  if (isLoading) return <p className={styles.state}>확인 중…</p>
  if (!isAdmin) return <p className={styles.state}>관리자 전용 페이지입니다.</p>

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>관리자</h1>

      {/* 연도 필터와 같은 가로 스트립 — 탭 7개는 좁은 화면에서 줄이 터진다.
          initialSlide로 URL에 박힌 탭이 처음부터 보이게 한다. */}
      <Swiper
        modules={[FreeMode, Mousewheel]}
        freeMode={{ enabled: true, momentum: true }}
        mousewheel={{ forceToAxis: true }}
        grabCursor
        slidesPerView="auto"
        spaceBetween={8}
        initialSlide={TABS.findIndex((t) => t.key === tab)}
        className={styles.tabs}
        role="tablist"
        aria-label="관리자 탭"
      >
        {TABS.map((t) => (
          <SwiperSlide key={t.key} className={styles.slide}>
            <button
              role="tab"
              aria-selected={t.key === tab}
              className={t.key === tab ? `${styles.tab} ${styles.active}` : styles.tab}
              onClick={() => setParams({ tab: t.key }, { replace: true })}
            >
              {t.label}
            </button>
          </SwiperSlide>
        ))}
      </Swiper>

      {tab === 'crawl' ? (
        <CrawlTab />
      ) : tab === 'releases' ? (
        <ReleasesTab />
      ) : tab === 'tracks' ? (
        <TracksTab />
      ) : tab === 'deleted' ? (
        <DeletedAlbumsTab />
      ) : tab === 'stats' ? (
        <StatsTab />
      ) : tab === 'reports' ? (
        <ReportsTab />
      ) : tab === 'rename' ? (
        <RenameRequestsTab />
      ) : (
        <AliasManagerTab />
      )}
    </div>
  )
}
