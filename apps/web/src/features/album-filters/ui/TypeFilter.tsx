import { useRef } from 'react'
import { TYPE_OPTIONS, type AlbumType } from '../model/types'
import styles from './TypeFilter.module.css'

interface Props {
  value: AlbumType[]
  onChange: (types: AlbumType[]) => void
}

export function TypeFilter({ value, onChange }: Props) {
  // '전체'를 다시 누르면 그 직전 선택으로 돌아가도록 지운 값을 기억해 둔다.
  const beforeAll = useRef<AlbumType[]>([])
  const isAll = value.length === 0

  const toggle = (k: AlbumType) =>
    onChange(value.includes(k) ? value.filter((v) => v !== k) : [...value, k])

  const toggleAll = () => {
    if (isAll) {
      if (beforeAll.current.length) onChange(beforeAll.current)
      beforeAll.current = []
      return
    }
    beforeAll.current = value
    onChange([])
  }

  return (
    <div className={styles.tabs} role="group" aria-label="앨범 유형">
      <button
        type="button"
        aria-pressed={isAll}
        className={isAll ? `${styles.tab} ${styles.active}` : styles.tab}
        onClick={toggleAll}
      >
        전체
      </button>
      {TYPE_OPTIONS.map((o) => (
        <button
          key={o.key}
          type="button"
          aria-pressed={value.includes(o.key)}
          className={value.includes(o.key) ? `${styles.tab} ${styles.active}` : styles.tab}
          onClick={() => toggle(o.key)}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}
