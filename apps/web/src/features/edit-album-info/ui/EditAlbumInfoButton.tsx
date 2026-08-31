import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { SquarePen } from 'lucide-react'
import { updateAlbumInfo, type Album } from '@/entities/album'
import { useAuth } from '@/entities/session'
import styles from './EditAlbumInfoButton.module.css'

const TYPE_OPTIONS = [
  { value: '', label: '미지정' },
  { value: 'album', label: '정규 (album)' },
  { value: 'single', label: '싱글/EP (single)' },
  { value: 'compilation', label: '컴필레이션' },
]

const toForm = (a: Album) => ({
  name: a.name,
  release_date: a.release_date ?? '',
  album_type: a.album_type ?? '',
  total_tracks: a.total_tracks == null ? '' : String(a.total_tracks),
})

// 관리자 전용 메타 편집. Spotify 발매일이 실제와 다른 앨범이 있어 원제/발매일/
// 유형/트랙 수를 직접 고친다. 저장 후엔 서버가 재동기화 덮어쓰기를 막아준다.
export function EditAlbumInfoButton({ album }: { album: Album }) {
  const { isAdmin } = useAuth()
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState(() => toForm(album))
  const set = (k: keyof ReturnType<typeof toForm>) => (e: { target: { value: string } }) =>
    setForm((f) => ({ ...f, [k]: e.target.value }))

  const save = useMutation({
    mutationFn: () =>
      updateAlbumInfo(album.id, {
        name: form.name.trim(),
        release_date: form.release_date.trim() || null,
        album_type: form.album_type || null,
        total_tracks: form.total_tracks === '' ? null : Number(form.total_tracks),
      }),
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
          setForm(toForm(album))
          setEditing(true)
        }}
      >
        <SquarePen size={14} strokeWidth={2.2} />
        정보 수정
      </button>
    )

  return (
    <form
      className={styles.form}
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <label className={styles.field}>
        <span>원제</span>
        <input className={styles.input} value={form.name} onChange={set('name')} required autoFocus />
      </label>
      <label className={styles.field}>
        <span>발매일</span>
        <input
          className={styles.input}
          value={form.release_date}
          onChange={set('release_date')}
          placeholder="YYYY-MM-DD"
          pattern="\d{4}(-\d{2}(-\d{2})?)?"
          title="YYYY, YYYY-MM 또는 YYYY-MM-DD"
          inputMode="numeric"
        />
      </label>
      <label className={styles.field}>
        <span>유형</span>
        <select className={styles.input} value={form.album_type} onChange={set('album_type')}>
          {TYPE_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
      <label className={styles.field}>
        <span>트랙 수</span>
        <input
          className={styles.input}
          type="number"
          min={1}
          value={form.total_tracks}
          onChange={set('total_tracks')}
          inputMode="numeric"
        />
      </label>
      <div className={styles.actions}>
        <button type="submit" className={styles.save} disabled={save.isPending}>
          {save.isPending ? '저장 중…' : '저장'}
        </button>
        <button type="button" className={styles.cancel} onClick={() => setEditing(false)}>
          취소
        </button>
        {save.isError && <span className={styles.error}>{save.error.message}</span>}
      </div>
    </form>
  )
}
