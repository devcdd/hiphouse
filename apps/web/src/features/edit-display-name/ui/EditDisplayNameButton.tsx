import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Pencil } from 'lucide-react'
import { updateAlbumDisplayName, updateTrackDisplayName } from '@/entities/album'
import { updateArtistDisplayName } from '@/entities/artist'
import { useAuth } from '@/entities/session'
import styles from './EditDisplayNameButton.module.css'

// 관리자 전용 한글명 편집. Spotify는 유통사가 등록한 이름(대개 영문) 하나만 주기
// 때문에 한글 표기는 여기서 직접 넣는다. 비우고 저장하면 해제되어 원본명으로 돌아간다.
// 관리자가 아니면 아무것도 렌더하지 않는다.
export function EditDisplayNameButton({
  kind,
  id,
  name,
  displayName,
  albumId,
  full = false,
  onEditingChange,
}: {
  kind: 'album' | 'artist' | 'track'
  id: string
  name: string // Spotify 원본명 — placeholder 및 해제 시 표시될 이름
  displayName: string | null
  albumId?: string // kind='track'일 때 트랙이 속한 앨범 id (tracks PK가 album_id+id 복합)
  full?: boolean // 관리자 카드처럼 폼이 한 줄 전체를 쓰는 자리에서 다른 편집 폼과 같은 모양으로
  onEditingChange?: (editing: boolean) => void // 부모가 폼 열림 동안 다른 도구를 숨기는 용도
}) {
  const { isAdmin } = useAuth()
  const qc = useQueryClient()
  const [editing, setEditingState] = useState(false)
  const setEditing = (v: boolean) => {
    setEditingState(v)
    onEditingChange?.(v)
  }
  const [text, setText] = useState(displayName ?? '')

  const save = useMutation({
    mutationFn: () =>
      kind === 'album'
        ? updateAlbumDisplayName(id, text.trim())
        : kind === 'artist'
          ? updateArtistDisplayName(id, text.trim())
          : updateTrackDisplayName(albumId ?? '', id, text.trim()),
    onSuccess: () => {
      setEditing(false)
      if (kind === 'track') {
        qc.invalidateQueries({ queryKey: ['album-tracks', albumId] })
        return
      }
      qc.invalidateQueries({ queryKey: [kind, id] })
      qc.invalidateQueries({ queryKey: [`${kind}s`] })
      // 앨범 카드에도 아티스트명이 찍히므로 아티스트 이름이 바뀌면 앨범 목록도 갱신.
      if (kind === 'artist') qc.invalidateQueries({ queryKey: ['albums'] })
    },
  })

  if (!isAdmin) return null

  if (!editing)
    return (
      <button
        type="button"
        className={styles.btn}
        onClick={() => {
          setText(displayName ?? '')
          setEditing(true)
        }}
      >
        <Pencil size={14} strokeWidth={2.2} />
        {displayName ? '한글명 수정' : '한글명 추가'}
      </button>
    )

  const submit = (
    <button
      type="submit"
      className={styles.save}
      disabled={save.isPending || text.trim() === (displayName ?? '')}
    >
      {save.isPending ? '저장 중…' : '저장'}
    </button>
  )
  const input = (
    <input
      className={styles.input}
      value={text}
      onChange={(e) => setText(e.target.value)}
      placeholder={name}
      title="비우고 저장하면 원본 이름으로 돌아갑니다"
      aria-label="한글 표시 이름"
      autoFocus
    />
  )

  return (
    <form
      className={full ? styles.formFull : styles.form}
      onKeyDown={(e) => e.key === 'Escape' && setEditing(false)}
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      {full ? (
        <>
          {input}
          {save.isError && <span className={styles.error}>저장 실패</span>}
          <button type="button" className={styles.cancelFull} onClick={() => setEditing(false)}>
            취소
          </button>
          {submit}
        </>
      ) : (
        <>
          {input}
          {submit}
          <button type="button" className={styles.cancel} onClick={() => setEditing(false)}>
            취소
          </button>
          {save.isError && <span className={styles.error}>실패</span>}
        </>
      )}
    </form>
  )
}
