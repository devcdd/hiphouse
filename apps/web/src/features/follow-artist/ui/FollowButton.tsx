import { UserCheck, UserPlus } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { useStartSignIn } from '@/features/auth'
import { useFollowIds, useToggleFollow } from '@/features/follow-artist/model/useFollows'
import styles from './FollowButton.module.css'

export function FollowButton({ artistId }: { artistId: string }) {
  const { isAuthed } = useAuth()
  const startSignIn = useStartSignIn()
  const followIds = useFollowIds(isAuthed)
  const toggle = useToggleFollow()

  if (!isAuthed) {
    return (
      <button type="button" className={styles.btn} onClick={startSignIn}>
        <UserPlus size={15} strokeWidth={2.2} />
        로그인하고 팔로우
      </button>
    )
  }

  const following = followIds.has(artistId)
  return (
    <button
      type="button"
      className={following ? `${styles.btn} ${styles.on}` : styles.btn}
      aria-pressed={following}
      disabled={toggle.isPending}
      onClick={() => toggle.mutate({ id: artistId, following })}
    >
      {following ? <UserCheck size={15} strokeWidth={2.2} /> : <UserPlus size={15} strokeWidth={2.2} />}
      {following ? '팔로잉' : '팔로우'}
    </button>
  )
}
