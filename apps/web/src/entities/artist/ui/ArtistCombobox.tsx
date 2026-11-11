import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchArtists } from '@/entities/artist/api/artistApi'
import type { Artist } from '@/entities/artist/model/types'
import { Avatar } from '@/shared/ui/Avatar'
import { displayName } from '@/shared/lib/displayName'
import { useDebouncedValue } from '@/shared/lib/useDebouncedValue'
import styles from './ArtistCombobox.module.css'

const MAX_SUGGESTIONS = 8

// DB에 있는 아티스트를 타이핑으로 찾아 고르는 입력창. 고르면 입력은 비워진다.
export function ArtistCombobox({
  onPick,
  excludeIds,
  placeholder = '아티스트 검색 (이름, 한글명 또는 연관검색어)',
  autoFocus,
}: {
  onPick: (artist: Artist) => void
  excludeIds?: Iterable<string>
  placeholder?: string
  autoFocus?: boolean
}) {
  const listId = useId()
  const [input, setInput] = useState('')
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const q = useDebouncedValue(input.trim(), 200)

  const search = useQuery({
    queryKey: ['artists', 'suggest', q],
    queryFn: () => fetchArtists({ q, offset: 0 }),
    enabled: q !== '',
    staleTime: 30_000,
  })
  const taken = new Set(excludeIds)
  const suggestions = (search.data ?? []).filter((a) => !taken.has(a.id)).slice(0, MAX_SUGGESTIONS)
  const showList = open && q !== ''

  const pick = (a: Artist) => {
    onPick(a)
    setInput('')
    setActive(0)
  }

  return (
    <div className={styles.combo}>
      <input
        className={styles.input}
        value={input}
        onChange={(e) => {
          setInput(e.target.value)
          setOpen(true)
          setActive(0)
        }}
        onFocus={() => setOpen(true)}
        // 항목 클릭은 onMouseDown에서 처리되므로 blur 뒤에 닫아도 선택이 먹힌다.
        onBlur={() => setOpen(false)}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return // 한글 조합 중 Enter·화살표는 IME 몫
          if (e.key === 'Escape') {
            // 드롭다운이 열려 있으면 그것만 닫고, 감싼 편집 폼까지는 ESC를 넘기지 않는다.
            if (showList) e.stopPropagation()
            setOpen(false)
          } else if (e.key === 'ArrowDown' && suggestions.length > 0) {
            e.preventDefault()
            setOpen(true)
            setActive((i) => (i + 1) % suggestions.length)
          } else if (e.key === 'ArrowUp' && suggestions.length > 0) {
            e.preventDefault()
            setActive((i) => (i - 1 + suggestions.length) % suggestions.length)
          } else if (e.key === 'Enter' && showList) {
            e.preventDefault() // 드롭다운이 열려 있을 땐 폼 제출 대신 선택
            if (suggestions[active]) pick(suggestions[active])
          }
        }}
        placeholder={placeholder}
        aria-label={placeholder}
        role="combobox"
        aria-expanded={showList}
        aria-controls={listId}
        aria-autocomplete="list"
        autoFocus={autoFocus}
      />
      {showList && (
        <ul className={styles.dropdown} id={listId} role="listbox">
          {search.isFetching && suggestions.length === 0 ? (
            <li className={styles.hint}>검색 중…</li>
          ) : suggestions.length === 0 ? (
            <li className={styles.hint}>일치하는 아티스트가 없습니다</li>
          ) : (
            suggestions.map((a, i) => (
              <li
                key={a.id}
                role="option"
                aria-selected={i === active}
                className={i === active ? `${styles.option} ${styles.optionActive}` : styles.option}
                onMouseEnter={() => setActive(i)}
                onMouseDown={(e) => {
                  e.preventDefault()
                  pick(a)
                }}
              >
                <Avatar src={a.image_url} name={displayName(a)} size={28} />
                <span className={styles.optionText}>
                  <span className={styles.optionName}>{displayName(a)}</span>
                  {a.display_name && a.name && <span className={styles.optionSub}>{a.name}</span>}
                </span>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  )
}
