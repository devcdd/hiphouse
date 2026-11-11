import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Users, X } from 'lucide-react'
import { updateAlbumArtists, type Album } from '@/entities/album'
import { ArtistCombobox } from '@/entities/artist'
import { useAuth } from '@/entities/session'
import { Avatar } from '@/shared/ui/Avatar'
import { displayName } from '@/shared/lib/displayName'
import styles from './EditAlbumArtistsButton.module.css'

type Credit = { id: string; name: string | null; display_name: string | null; image_url: string | null }

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
  const [editing, setEditingState] = useState(false)
  const setEditing = (v: boolean) => {
    setEditingState(v)
    onEditingChange?.(v)
  }
  const [credits, setCredits] = useState<Credit[]>(album.artists)
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
      onKeyDown={(e) => e.key === 'Escape' && setEditing(false)}
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

      <ArtistCombobox
        autoFocus
        excludeIds={credits.map((c) => c.id)}
        onPick={(a) => setCredits((cs) => [...cs, a])}
      />

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
