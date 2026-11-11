import { useId, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Users, X } from 'lucide-react'
import { updateAlbumArtists, type Album } from '@/entities/album'
import { fetchArtists } from '@/entities/artist'
import { useAuth } from '@/entities/session'
import { Avatar } from '@/shared/ui/Avatar'
import { displayName } from '@/shared/lib/displayName'
import { useDebouncedValue } from '@/shared/lib/useDebouncedValue'
import styles from './EditAlbumArtistsButton.module.css'

type Credit = { id: string; name: string | null; display_name: string | null; image_url: string | null }

const MAX_SUGGESTIONS = 8

// 관리자 전용 크레딧 교체. 이미 DB에 있는 아티스트만 고를 수 있고, 목록 순서가 표시 순서가 된다.
export function EditAlbumArtistsButton({
  album,
  onEditingChange,
}: {
  album: Album
  onEditingChange?: (editing: boolean) => void // 부모가 폼 열림 동안 다른 도구를 숨기는 용도
}) {
  const { isAdmin } = useAuth()
  const qc = useQueryClient()
  const listId = useId()
  const [editing, setEditingState] = useState(false)
  const setEditing = (v: boolean) => {
    setEditingState(v)
    onEditingChange?.(v)
  }
  const [credits, setCredits] = useState<Credit[]>(album.artists)
  const [input, setInput] = useState('')
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const q = useDebouncedValue(input.trim(), 200)

  const search = useQuery({
    queryKey: ['artists', 'suggest', q],
    queryFn: () => fetchArtists({ q, offset: 0 }),
    enabled: editing && q !== '',
    staleTime: 30_000,
  })
  const taken = new Set(credits.map((c) => c.id))
  const suggestions = (search.data ?? []).filter((a) => !taken.has(a.id)).slice(0, MAX_SUGGESTIONS)
  const showList = open && q !== ''

  const pick = (a: Credit) => {
    setCredits((cs) => [...cs, a])
    setInput('')
    setActive(0)
  }

  const save = useMutation({
    mutationFn: () => updateAlbumArtists(album.id, credits.map((c) => c.id)),
    onSuccess: () => {
      setEditing(false)
      qc.invalidateQueries({ queryKey: ['album', album.id] })
      qc.invalidateQueries({ queryKey: ['albums'] })
    },
  })

  if (!isAdmin) return null

  if (!editing)
    return (
      <button
        type="button"
        className={styles.btn}
        onClick={() => {
          setCredits(album.artists)
          setInput('')
          save.reset()
          setEditing(true)
        }}
      >
        <Users size={14} strokeWidth={2.2} />
        아티스트 변경
      </button>
    )

  const unchanged =
    credits.length === album.artists.length && credits.every((c, i) => c.id === album.artists[i].id)

  return (
    <form
      className={styles.form}
      onSubmit={(e) => {
        e.preventDefault()
        if (credits.length > 0) save.mutate()
      }}
    >
      <span className={styles.label}>크레딧 아티스트</span>
      <ul className={styles.chips}>
        {credits.map((c) => (
          <li key={c.id} className={styles.chip}>
            <Avatar src={c.image_url} name={displayName(c)} size={22} />
            <span className={styles.chipName}>{displayName(c)}</span>
            <button
              type="button"
              className={styles.chipRemove}
              onClick={() => setCredits((cs) => cs.filter((x) => x.id !== c.id))}
              aria-label={`${displayName(c)} 제거`}
            >
              <X size={13} strokeWidth={2.4} />
            </button>
          </li>
        ))}
        {credits.length === 0 && <li className={styles.empty}>아티스트를 한 명 이상 추가하세요</li>}
      </ul>

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
            if (e.key === 'Escape') {
              setOpen(false)
            } else if (e.key === 'ArrowDown' && suggestions.length > 0) {
              e.preventDefault()
              setOpen(true)
              setActive((i) => (i + 1) % suggestions.length)
            } else if (e.key === 'ArrowUp' && suggestions.length > 0) {
              e.preventDefault()
              setActive((i) => (i - 1 + suggestions.length) % suggestions.length)
            } else if (e.key === 'Enter' && showList) {
              e.preventDefault() // 드롭다운이 열려 있을 땐 저장 대신 선택
              if (suggestions[active]) pick(suggestions[active])
            }
          }}
          placeholder="아티스트 검색 (이름, 한글명 또는 연관검색어)"
          aria-label="추가할 아티스트 검색"
          role="combobox"
          aria-expanded={showList}
          aria-controls={listId}
          aria-autocomplete="list"
          autoFocus
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

      <div className={styles.actions}>
        {save.isError && <span className={styles.error}>{save.error.message}</span>}
        <button type="button" className={styles.cancel} onClick={() => setEditing(false)}>
          취소
        </button>
        <button type="submit" className={styles.save} disabled={credits.length === 0 || unchanged || save.isPending}>
          {save.isPending ? '저장 중…' : '저장'}
        </button>
      </div>
    </form>
  )
}
