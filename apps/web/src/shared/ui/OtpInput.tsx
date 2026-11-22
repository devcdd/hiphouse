import { useState, type SyntheticEvent } from 'react'
import styles from './OtpInput.module.css'

interface Props {
  id?: string
  value: string
  onChange: (value: string) => void
  length?: number
  autoFocus?: boolean
  'aria-label'?: string
}

// 숫자 칸을 따로 그리지만 실제 입력은 투명한 input 하나가 받는다.
// 칸마다 input을 두면 붙여 넣기와 iOS·Android의 코드 자동 입력(one-time-code)이 첫 칸에만 들어간다.
export function OtpInput({ id, value, onChange, length = 6, autoFocus, 'aria-label': ariaLabel }: Props) {
  const [focused, setFocused] = useState(false)
  const active = Math.min(value.length, length - 1)

  // 칸 중간을 눌러도 커서를 끝에 둬서, 입력과 지우기가 항상 마지막 칸에서 일어나게 한다.
  const keepCaretAtEnd = (e: SyntheticEvent<HTMLInputElement>) => {
    const el = e.currentTarget
    el.setSelectionRange(el.value.length, el.value.length)
  }

  return (
    <div className={styles.wrap}>
      <input
        id={id}
        className={styles.input}
        inputMode="numeric"
        autoComplete="one-time-code"
        maxLength={length}
        value={value}
        onChange={(e) => onChange(e.target.value.replace(/\D/g, '').slice(0, length))}
        onFocus={(e) => {
          setFocused(true)
          keepCaretAtEnd(e)
        }}
        onBlur={() => setFocused(false)}
        onSelect={keepCaretAtEnd}
        autoFocus={autoFocus}
        aria-label={ariaLabel}
      />
      <div className={styles.boxes} style={{ gridTemplateColumns: `repeat(${length}, 1fr)` }} aria-hidden>
        {Array.from({ length }, (_, i) => {
          const isActive = focused && i === active
          return (
            <span
              key={i}
              className={`${styles.box} ${value[i] ? styles.filled : ''} ${isActive ? styles.active : ''}`}
            >
              {value[i] ?? (isActive && <span className={styles.caret} />)}
            </span>
          )
        })}
      </div>
    </div>
  )
}
