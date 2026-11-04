import { useEffect, useRef } from 'react'
import { Swiper, SwiperSlide } from 'swiper/react'
import { FreeMode, Mousewheel } from 'swiper/modules'
import type { Swiper as SwiperClass } from 'swiper'
import type { YearOption } from '../model/types'
import 'swiper/css'
import 'swiper/css/free-mode'
import 'swiper/css/mousewheel'
import styles from './YearFilter.module.css'

interface Props {
  years: YearOption[]
  value: YearOption
  onChange: (year: YearOption) => void
}

// Horizontal year strip on Swiper: drag + flick-momentum (freeMode), horizontal
// wheel, grab cursor. Swiper also suppresses the click that ends a drag.
export function YearFilter({ years, value, onChange }: Props) {
  const swiperRef = useRef<SwiperClass | null>(null)

  // Bring the selected year into view when it changes (e.g. cleared from URL).
  // Only when it actually changes — re-running this on unrelated renders yanks
  // the strip back to the selected chip mid-swipe.
  const scrolledTo = useRef<YearOption | null>(null)
  useEffect(() => {
    if (scrolledTo.current === value) return
    const i = years.indexOf(value)
    if (i < 0) return
    scrolledTo.current = value
    swiperRef.current?.slideTo(i)
  }, [value, years])

  return (
    <Swiper
      onSwiper={(s) => {
        swiperRef.current = s
      }}
      modules={[FreeMode, Mousewheel]}
      freeMode={{ enabled: true, momentum: true }}
      mousewheel={{ forceToAxis: true }}
      // 기본값엔 button이 있어, 포커스된 칩을 잡고 끌면 Swiper가 시작 위치를 0으로 잡아 맨 앞으로 튄다(swiper 14.0.6).
      focusableElements="input, select, option, textarea, video, label"
      grabCursor
      slidesPerView="auto"
      spaceBetween={8}
      className={styles.tabs}
      role="tablist"
      aria-label="연도 필터"
    >
      {years.map((y) => (
        <SwiperSlide key={y} className={styles.slide}>
          <button
            role="tab"
            aria-selected={y === value}
            className={y === value ? `${styles.tab} ${styles.active}` : styles.tab}
            onClick={() => onChange(y)}
          >
            {y}
          </button>
        </SwiperSlide>
      ))}
    </Swiper>
  )
}
