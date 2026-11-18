import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react'
import { AlbumCard, AlbumCardSkeleton, useOnThisDay } from '@/entities/album'
import { useAuth } from '@/entities/session'
import { PromoImageButton } from '@/features/album-promo-image'
import styles from './OnThisDay.module.css'

// 접은 날짜를 저장한다. 앨범이 날마다 바뀌니 다음 날에는 다시 펼쳐서 보여 준다.
const COLLAPSED_KEY = 'hiphouse:on-this-day-collapsed'

// 상세에 갔다 돌아오면 위젯이 새로 그려지므로 가로 스크롤 위치를 모듈에 남겨 둔다.
// 새로고침이나 날짜가 바뀐 뒤에는 처음부터 보여 주는 게 맞아서 저장소에는 쓰지 않는다.
let savedScroll = { date: '', left: 0 }

function readCollapsed(date: string): boolean {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === date
  } catch {
    return false
  }
}

function writeCollapsed(date: string, collapsed: boolean) {
  try {
    if (collapsed) localStorage.setItem(COLLAPSED_KEY, date)
    else localStorage.removeItem(COLLAPSED_KEY)
  } catch {
    // 저장소를 못 써도 이번 화면에서는 접고 펼 수 있다.
  }
}

function localDate(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

function yearsAgo(releaseDate: string | null, thisYear: number): number {
  return releaseDate ? thisYear - Number(releaseDate.slice(0, 4)) : 0
}

export function OnThisDay() {
  const today = new Date()
  const date = localDate(today)
  const { data, isLoading } = useOnThisDay(date)
  const { isAdmin } = useAuth()
  const [collapsed, setCollapsed] = useState(() => readCollapsed(date))
  const track = useRef<HTMLDivElement>(null)
  const [edge, setEdge] = useState({ start: true, end: false })

  const updateEdge = () => {
    const el = track.current
    if (!el) return
    setEdge({ start: el.scrollLeft <= 1, end: el.scrollLeft + el.clientWidth >= el.scrollWidth - 1 })
  }
  useEffect(updateEdge, [data, collapsed])

  // 카드가 그려진 직후, 화면에 보이기 전에 되돌려야 처음 위치가 깜빡이지 않는다.
  useLayoutEffect(() => {
    const el = track.current
    if (!el || !data || savedScroll.date !== date) return
    el.scrollLeft = savedScroll.left
    updateEdge()
  }, [data, date])

  if (!isLoading && (!data || data.length === 0)) return null

  const toggle = (next: boolean) => {
    setCollapsed(next)
    writeCollapsed(date, next)
  }

  const scroll = (dir: 1 | -1) => {
    const el = track.current
    if (el) el.scrollBy({ left: dir * el.clientWidth * 0.8, behavior: 'smooth' })
  }

  const onScroll = () => {
    const el = track.current
    if (el) savedScroll = { date, left: el.scrollLeft }
    updateEdge()
  }

  const thumbs = (data ?? []).filter((a) => a.image_url).slice(0, 4)

  return (
    <section className={styles.section} data-collapsed={collapsed} aria-label="오늘의 발매 기념일">
      <button
        type="button"
        className={styles.bar}
        onClick={() => toggle(!collapsed)}
        aria-expanded={!collapsed}
        aria-controls="on-this-day-track"
        title={collapsed ? undefined : '오늘 하루 접어 두기'}
      >
        {thumbs.length > 0 && (
          <span className={styles.thumbs} aria-hidden>
            {thumbs.map((a) => (
              <img key={a.id} src={a.image_url ?? ''} alt="" className={styles.thumb} />
            ))}
          </span>
        )}
        <span className={styles.barTitle}>오늘의 발매 기념일</span>
        <span className={styles.barCount}>
          <span className={styles.barDate}>
            {today.getMonth() + 1}월 {today.getDate()}일{data && ' · '}
          </span>
          {data && `${data.length}장`}
        </span>
        <ChevronDown size={18} strokeWidth={2.2} className={styles.barIcon} />
      </button>
      <div className={styles.collapse} inert={collapsed}>
        <div className={styles.collapseInner}>
          <hr className={styles.divider} />
          <div className={styles.trackWrap}>
            <div ref={track} id="on-this-day-track" className={styles.track} onScroll={onScroll}>
              {isLoading
                ? Array.from({ length: 6 }, (_, i) => (
                    <div key={i} className={styles.item}>
                      <span className={styles.tagSkeleton} />
                      <AlbumCardSkeleton />
                    </div>
                  ))
                : data?.map((a) => {
                    const years = yearsAgo(a.release_date, today.getFullYear())
                    return (
                      <div key={a.id} className={styles.item}>
                        <span className={styles.tag}>{years}년 전 오늘</span>
                        <AlbumCard
                          album={a}
                          artAction={
                            isAdmin ? <PromoImageButton album={a} options={{ anniversaryYears: years }} /> : undefined
                          }
                        />
                      </div>
                    )
                  })}
            </div>
            {!edge.start && (
              <button
                type="button"
                className={`${styles.navBtn} ${styles.prev}`}
                onClick={() => scroll(-1)}
                aria-label="이전"
              >
                <ChevronLeft size={18} strokeWidth={2.2} />
              </button>
            )}
            {!edge.end && (
              <button
                type="button"
                className={`${styles.navBtn} ${styles.next}`}
                onClick={() => scroll(1)}
                aria-label="다음"
              >
                <ChevronRight size={18} strokeWidth={2.2} />
              </button>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}
