import styles from './TypeFilter.module.css'

interface Props {
  value: boolean
  onChange: (awarded: boolean) => void
}

// 수상작만 보기 토글. 유형 필터와 직교하므로 별도 버튼, 디자인은 동일.
export function AwardedFilter({ value, onChange }: Props) {
  return (
    <button
      type="button"
      aria-pressed={value}
      className={value ? `${styles.tab} ${styles.active}` : styles.tab}
      onClick={() => onChange(!value)}
    >
      수상작
    </button>
  )
}
