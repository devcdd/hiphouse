import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { GitMerge, Search, Trash2, X } from 'lucide-react'
import {
  deleteArtist,
  mergeArtists,
  updateArtistAliases,
  updateArtistDisplayName,
  updateArtistReleasesWatch,
  useArtistSearch,
  type Artist,
  type DeleteMode,
} from '@/entities/artist'
import { Avatar } from '@/shared/ui/Avatar'
import { useToast } from '@/shared/ui/toast'
import { displayName } from '@/shared/lib/displayName'
import { useDebouncedValue } from '@/shared/lib/useDebouncedValue'
import { useInfiniteScroll } from '@/shared/lib/useInfiniteScroll'
import styles from './AdminPage.module.css'

// 연관검색어 관리 + 아티스트 삭제/병합. Rows are checkbox-selectable; a floating
// bottom bar shows the selection and merges it into a chosen master artist.
export function AliasManagerTab() {
  const qc = useQueryClient()
  const [input, setInput] = useState('')
  const q = useDebouncedValue(input.trim(), 300) // instant search — no submit button
  const { data, fetchNextPage, hasNextPage, isFetching, isLoading, error } = useArtistSearch(q)
  const artists = useMemo(() => data?.pages.flat() ?? [], [data])
  const sentinel = useInfiniteScroll<HTMLDivElement>(fetchNextPage, hasNextPage && !isFetching)

  // Selection: id -> display name (kept so the merge bar can label options
  // even after the list refetches or the query changes).
  const [selected, setSelected] = useState<Map<string, string>>(new Map())
  const [masterId, setMasterId] = useState('')
  const selectedIds = [...selected.keys()]
  const master = selected.has(masterId) ? masterId : (selectedIds[0] ?? '')

  const toggle = (a: Artist) =>
    setSelected((prev) => {
      const next = new Map(prev)
      if (next.has(a.id)) next.delete(a.id)
      else next.set(a.id, displayName(a))
      return next
    })
  const clear = () => {
    setSelected(new Map())
    setMasterId('')
  }

  const merge = useMutation({
    mutationFn: () =>
      mergeArtists(
        master,
        selectedIds.filter((id) => id !== master),
      ),
    onSuccess: () => {
      clear()
      qc.invalidateQueries({ queryKey: ['artists'] })
      qc.invalidateQueries({ queryKey: ['albums'] }) // credits moved
    },
  })

  return (
    <div className={styles.aliasWrap}>
      <div className={styles.searchForm}>
        <Search size={15} className={styles.searchIcon} aria-hidden />
        <input
          className={styles.searchInput}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="아티스트 검색 (이름, 한글명 또는 연관검색어)"
          aria-label="아티스트 검색"
        />
        {input && (
          <button type="button" className={styles.clearBtn} onClick={() => setInput('')} aria-label="검색어 지우기">
            <X size={14} />
          </button>
        )}
      </div>

      {error && <p className={styles.state}>불러오기 실패: {String(error)}</p>}
      {isLoading && <p className={styles.state}>불러오는 중…</p>}

      <div className={styles.rows}>
        {artists.map((a) => (
          <AliasRow
            key={a.id}
            artist={a}
            checked={selected.has(a.id)}
            onToggle={() => toggle(a)}
            onDeleted={(id) =>
              setSelected((prev) => {
                if (!prev.has(id)) return prev
                const next = new Map(prev)
                next.delete(id)
                return next
              })
            }
          />
        ))}
      </div>
      {hasNextPage && <div ref={sentinel} className={styles.sentinel} />}
      {!isLoading && !error && artists.length === 0 && <p className={styles.state}>아티스트가 없습니다.</p>}

      {selectedIds.length > 0 && (
        <div className={styles.mergeBar} role="toolbar" aria-label="선택된 아티스트 작업">
          <span className={styles.mergeCount}>{selectedIds.length}개 선택됨</span>
          {selectedIds.length >= 2 && (
            <>
              <label className={styles.mergeLabel}>
                마스터
                <select
                  className={styles.mergeSelect}
                  value={master}
                  onChange={(e) => setMasterId(e.target.value)}
                >
                  {selectedIds.map((id) => (
                    <option key={id} value={id}>
                      {selected.get(id)}
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                className={styles.mergeBtn}
                disabled={merge.isPending}
                onClick={() => merge.mutate()}
              >
                <GitMerge size={14} />
                {merge.isPending ? '병합 중…' : '합치기'}
              </button>
            </>
          )}
          {merge.isError && <span className={styles.mergeError}>실패</span>}
          <button type="button" className={styles.mergeClear} onClick={clear} aria-label="선택 해제">
            <X size={15} />
          </button>
        </div>
      )}
    </div>
  )
}

function AliasRow({
  artist,
  checked,
  onToggle,
  onDeleted,
}: {
  artist: Artist
  checked: boolean
  onToggle: () => void
  onDeleted: (id: string) => void
}) {
  const qc = useQueryClient()
  const toast = useToast()
  const saved = (artist.aliases ?? []).join(', ')
  const savedName = artist.display_name ?? ''
  const [text, setText] = useState(saved)
  const [name, setName] = useState(savedName) // 한글 표시 이름 (비우면 해제)
  const [confirming, setConfirming] = useState(false)
  const shown = displayName(artist)
  const dirty = text !== saved || name.trim() !== savedName

  // 한 번의 저장으로 한글명 + 연관검색어를 함께 반영 — 바뀐 것만 요청한다.
  const save = useMutation({
    mutationFn: async () => {
      if (name.trim() !== savedName) await updateArtistDisplayName(artist.id, name.trim())
      return text === saved
        ? null
        : updateArtistAliases(
            artist.id,
            text.split(',').map((s) => s.trim()).filter(Boolean),
          )
    },
    onSuccess: (updated) => {
      if (updated) setText((updated.aliases ?? []).join(', '))
      qc.invalidateQueries({ queryKey: ['artists'] })
      qc.invalidateQueries({ queryKey: ['albums'] }) // 앨범 카드에도 아티스트명이 찍힘
    },
  })

  // 저장 버튼과 묶지 않는다 — 켜고 끄는 즉시 신보 체크 대상이 바뀌어야 한다.
  const watch = useMutation({
    mutationFn: (on: boolean) => updateArtistReleasesWatch(artist.id, on),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['artists'] })
      qc.invalidateQueries({ queryKey: ['releases-status'] })
    },
    onError: () => toast('신보 감시 변경에 실패했습니다', 'error'),
  })

  const del = useMutation({
    mutationFn: (mode: DeleteMode) => deleteArtist(artist.id, mode),
    onSuccess: ({ mode, albums }) => {
      setConfirming(false)
      if (mode === 'hard') onDeleted(artist.id)
      qc.invalidateQueries({ queryKey: ['artists'] })
      qc.invalidateQueries({ queryKey: ['albums'] })
      toast(mode === 'hard' ? `아티스트와 앨범 ${albums}장을 삭제했습니다` : `단독 앨범 ${albums}장을 삭제했습니다`)
    },
    onError: () => toast('삭제에 실패했습니다', 'error'),
  })

  return (
    <form
      className={styles.row}
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <input
        type="checkbox"
        className={styles.check}
        checked={checked}
        onChange={onToggle}
        aria-label={`${shown} 선택`}
      />
      <Avatar src={artist.image_url} name={shown} size={40} />
      <div className={styles.rowInfo}>
        <span className={styles.rowName} title={shown}>
          {shown}
        </span>
        <span className={styles.rowId}>{artist.id}</span>
      </div>
      <label className={styles.checkLabel} title="신보 체크 탭이 확인하는 대상인지 여부">
        <input
          type="checkbox"
          className={styles.check}
          checked={artist.releases_watch}
          disabled={watch.isPending}
          onChange={(e) => watch.mutate(e.target.checked)}
        />
        신보
      </label>
      <input
        className={`${styles.aliasInput} ${styles.displayInput}`}
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder={`한글명 (비우면 ${artist.name ?? artist.id})`}
        aria-label="한글 표시 이름"
      />
      <input
        className={styles.aliasInput}
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="연관검색어 (쉼표로 구분: 블랙넛, 블넛)"
        aria-label="연관검색어"
      />
      <button type="submit" className={styles.save} disabled={!dirty || save.isPending}>
        {save.isPending ? '저장 중…' : '저장'}
      </button>
      {save.isError && <span className={styles.rowError}>실패</span>}
      <button
        type="button"
        className={styles.rowDelete}
        disabled={del.isPending}
        aria-label={`${shown} 삭제`}
        onClick={() => setConfirming(true)}
      >
        <Trash2 size={15} />
      </button>

      {confirming && (
        <DeleteArtistDialog
          name={shown}
          pending={del.isPending}
          onPick={(mode) => del.mutate(mode)}
          onClose={() => setConfirming(false)}
        />
      )}
    </form>
  )
}

// Two very different outcomes, so they get two labelled buttons instead of a
// yes/no confirm.
function DeleteArtistDialog({
  name,
  pending,
  onPick,
  onClose,
}: {
  name: string
  pending: boolean
  onPick: (mode: DeleteMode) => void
  onClose: () => void
}) {
  const dialog = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    dialog.current?.showModal()
  }, [])

  return (
    <dialog
      ref={dialog}
      className={styles.confirm}
      onClose={onClose}
      onClick={(e) => e.target === dialog.current && onClose()}
    >
      <div className={styles.confirmSheet}>
        <p className={styles.confirmTitle}>“{name}” 삭제</p>

        <button type="button" className={styles.confirmSoft} disabled={pending} onClick={() => onPick('soft')}>
          단독 앨범만 삭제
          <span>아티스트는 남기고, 이 아티스트만 크레딧된 앨범을 삭제 목록으로 보냅니다. 복구 가능.</span>
        </button>

        <button type="button" className={styles.confirmHard} disabled={pending} onClick={() => onPick('hard')}>
          아티스트와 모든 앨범 삭제
          <span>참여한 앨범을 전부 완전 삭제합니다. 피처링으로만 참여한 다른 아티스트의 앨범도 사라지며, 즐겨찾기·별점·댓글까지 함께 지워집니다. 복구 불가.</span>
        </button>

        <button type="button" className={styles.confirmCancel} onClick={onClose}>
          취소
        </button>
      </div>
    </dialog>
  )
}
