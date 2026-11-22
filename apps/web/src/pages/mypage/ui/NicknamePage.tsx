import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Camera, ChevronLeft } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deleteAvatar, useAuth, updateNickname, uploadAvatar } from '@/entities/session'
import type { User } from '@/entities/session'
import { apiUrl } from '@/shared/config'
import { resizeAvatar } from '@/shared/lib/resizeAvatar'
import { Avatar } from '@/shared/ui/Avatar'
import { useToast } from '@/shared/ui/toast'
import { ArtistClaimSection } from '@/features/artist-claim'
import { AuthGate } from './AuthGate'
import styles from './MyPage.module.css'

export function NicknamePage() {
  return (
    <AuthGate>
      <Editor />
    </AuthGate>
  )
}

function Editor() {
  const { user } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const [draft, setDraft] = useState('')

  // Seed once the session resolves (and if it changes elsewhere).
  useEffect(() => {
    if (user?.nickname != null) setDraft(user.nickname)
  }, [user?.nickname])

  const save = useMutation({
    mutationFn: updateNickname,
    onSuccess: (u) => {
      qc.setQueryData(['me'], u)
      toast('닉네임을 바꿨습니다')
    },
    // 409(중복)면 서버 문구를 그대로 보여준다.
    onError: (e: Error) => toast(e.message || '닉네임 변경에 실패했습니다', 'error'),
  })

  const trimmed = draft.trim()
  const canSave = trimmed !== '' && trimmed !== user?.nickname && !save.isPending

  return (
    <div className={styles.page}>
      <Link to="/me" className={styles.back}>
        <ChevronLeft size={16} aria-hidden />
        마이페이지
      </Link>
      <h1 className={styles.title}>프로필 수정</h1>
      <AvatarSection />
      <section className={styles.card}>
        <label className={styles.label} htmlFor="nickname">
          닉네임
        </label>
        <div className={styles.nickRow}>
          <input
            id="nickname"
            className={styles.input}
            value={draft}
            maxLength={20}
            disabled={save.isPending}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              // !isComposing: 한글 IME는 조합 종료 Enter로 keydown이 한 번 더 뜬다 → 중복 저장 방지
              if (e.key === 'Enter' && !e.nativeEvent.isComposing && canSave) save.mutate(trimmed)
            }}
          />
          <button
            type="button"
            className={styles.save}
            disabled={!canSave}
            onClick={() => save.mutate(trimmed)}
          >
            {save.isPending ? '저장 중…' : '저장'}
          </button>
        </div>
      </section>
      <ArtistClaimSection />
    </div>
  )
}

function AvatarSection() {
  const { user } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const fileInput = useRef<HTMLInputElement>(null)

  const onSaved = (u: User, message: string) => {
    qc.setQueryData(['me'], u)
    // 공개 프로필 헤더는 ['user', id]에 따로 캐시돼 있어 같이 비워야 새 사진이 보인다.
    qc.invalidateQueries({ queryKey: ['user', u.id] })
    toast(message)
  }

  const upload = useMutation({
    mutationFn: async (file: File) => uploadAvatar(await resizeAvatar(file)),
    onSuccess: (u) => onSaved(u, '프로필 사진을 바꿨습니다'),
    onError: (e: Error) => toast(e.message || '프로필 사진 변경에 실패했습니다', 'error'),
  })
  const remove = useMutation({
    mutationFn: deleteAvatar,
    onSuccess: (u) => onSaved(u, '프로필 사진을 지웠습니다'),
    onError: (e: Error) => toast(e.message || '프로필 사진 삭제에 실패했습니다', 'error'),
  })
  const busy = upload.isPending || remove.isPending
  const pickFile = () => fileInput.current?.click()

  return (
    <section className={styles.card}>
      <span className={styles.label}>프로필 사진</span>
      <div className={styles.avatarRow}>
        <button
          type="button"
          className={styles.avatarPick}
          disabled={busy}
          onClick={pickFile}
          aria-label={user?.avatar_url ? '프로필 사진 바꾸기' : '프로필 사진 올리기'}
        >
          <Avatar src={user?.avatar_url ? apiUrl(user.avatar_url) : null} name={user?.nickname || '사용자'} size={72} />
          <span className={styles.avatarPickBadge} aria-hidden>
            <Camera size={13} />
          </span>
        </button>
        <div className={styles.avatarActions}>
          <button type="button" className={styles.save} disabled={busy} onClick={pickFile}>
            {upload.isPending ? '올리는 중…' : user?.avatar_url ? '사진 바꾸기' : '사진 올리기'}
          </button>
          {user?.avatar_url && (
            <button type="button" className={styles.secondary} disabled={busy} onClick={() => remove.mutate()}>
              {remove.isPending ? '지우는 중…' : '지우기'}
            </button>
          )}
        </div>
        <input
          ref={fileInput}
          type="file"
          accept="image/*"
          hidden
          onChange={(e) => {
            const file = e.target.files?.[0]
            // 같은 파일을 다시 골라도 change가 뜨도록 비운다.
            e.target.value = ''
            if (file) upload.mutate(file)
          }}
        />
      </div>
      <p className={styles.hint}>가운데를 정사각형으로 잘라 저장합니다.</p>
    </section>
  )
}
