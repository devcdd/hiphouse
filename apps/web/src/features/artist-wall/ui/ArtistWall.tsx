import { useCallback, useId, useLayoutEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { BadgeCheck, ChevronDown, MessagesSquare, SendHorizontal, Trash2 } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { startKakaoLogin } from '@/features/auth'
import { Avatar } from '@/shared/ui/Avatar'
import { useInfiniteScroll } from '@/shared/lib/useInfiniteScroll'
import { WALL_MAX_LEN, type WallPost } from '@/features/artist-wall/api/wallApi'
import {
  useAddWallPost,
  useRemoveWallPost,
  useWall,
  useWallCount,
  useWallQuota,
} from '@/features/artist-wall/model/useWall'
import styles from './ArtistWall.module.css'

const timeFmt = new Intl.DateTimeFormat('ko-KR', { hour: '2-digit', minute: '2-digit' })
const dateFmt = new Intl.DateTimeFormat('ko-KR', { month: 'numeric', day: 'numeric' })
const NEAR_BOTTOM_PX = 80

function stamp(iso: string) {
  const d = new Date(iso)
  const today = new Date().toDateString() === d.toDateString()
  return today ? timeFmt.format(d) : `${dateFmt.format(d)} ${timeFmt.format(d)}`
}

type WallArtist = { id: string; name: string; image_url: string | null }

// 아티스트 담벼락. 기본은 접혀 있고 헤더에는 글 수만 보인다. 펼쳐야 글을 불러온다.
export function ArtistWall({ artist }: { artist: WallArtist }) {
  const [open, setOpen] = useState(false)
  const count = useWallCount(artist.id).data?.count
  const panelId = useId()

  return (
    <section className={styles.wall} aria-label="담벼락">
      <button
        type="button"
        className={styles.toggle}
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((v) => !v)}
      >
        <MessagesSquare size={17} strokeWidth={2.2} aria-hidden />
        <span className={styles.title}>담벼락</span>
        <span className={styles.end}>
          {count != null && <span className={styles.count}>{count.toLocaleString()}개</span>}
          <ChevronDown
            size={16}
            strokeWidth={2.4}
            className={open ? `${styles.chevron} ${styles.chevronOpen}` : styles.chevron}
            aria-hidden
          />
        </span>
      </button>
      {/* 펼칠 때마다 새로 마운트해서 스크롤이 항상 최신 글(맨 아래)에서 시작한다 */}
      {open && (
        <div id={panelId} className={styles.panel}>
          <WallBody artist={artist} />
        </div>
      )}
    </section>
  )
}

// 채팅창처럼 아래가 최신이고, 위로 올리면 과거 메시지를 더 불러온다.
// 담벼락 주인 아티스트로 인증된 사람의 글은 왼쪽, 나머지 사람의 글은 오른쪽 말풍선이다.
function WallBody({ artist }: { artist: WallArtist }) {
  const artistId = artist.id
  const { user, isAuthed, isAdmin } = useAuth()
  const quota = useWallQuota(artistId, isAuthed).data
  const remaining = quota && !quota.unlimited ? Math.max(quota.limit - quota.used, 0) : null
  const blocked = remaining === 0
  const { posts, isLoading, error, hasNextPage, fetchNextPage, isFetchingNextPage } = useWall(artistId)
  const add = useAddWallPost(artistId)
  const remove = useRemoveWallPost(artistId)
  const [body, setBody] = useState('')
  const text = body.trim()

  const scroller = useRef<HTMLDivElement>(null)
  // 직전 렌더의 스크롤 상태. 과거 페이지가 위에 붙어도 보던 위치를 지키고,
  // 바닥 근처에 있었을 때만 새 메시지를 따라 내려간다.
  const prev = useRef({ firstId: 0, lastId: 0, height: 0, top: 0, nearBottom: true })

  useLayoutEffect(() => {
    const el = scroller.current
    if (!el || posts.length === 0) return
    const firstId = posts[0].id
    const lastId = posts[posts.length - 1].id
    const p = prev.current
    if (p.firstId && firstId < p.firstId && lastId === p.lastId) {
      el.scrollTop = el.scrollHeight - p.height + p.top
    } else if (lastId !== p.lastId && (p.nearBottom || !p.lastId)) {
      el.scrollTop = el.scrollHeight
    }
    prev.current = { ...p, firstId, lastId }
  }, [posts])

  const onScroll = () => {
    const el = scroller.current
    if (!el) return
    prev.current.height = el.scrollHeight
    prev.current.top = el.scrollTop
    prev.current.nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_PX
  }

  // 위쪽 sentinel이 보이면 과거 페이지를 붙인다. 첫 렌더는 맨 아래로 스크롤된 뒤라 바로 불리지 않는다.
  const loadOlder = useCallback(() => {
    const el = scroller.current
    if (el) Object.assign(prev.current, { height: el.scrollHeight, top: el.scrollTop })
    fetchNextPage()
  }, [fetchNextPage])
  const sentinel = useInfiniteScroll<HTMLDivElement>(
    loadOlder,
    !!hasNextPage && !isFetchingNextPage && posts.length > 0,
    { root: scroller, rootMargin: '120px 0px 0px 0px' },
  )

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!text || add.isPending || blocked) return
    prev.current.nearBottom = true // 내가 쓴 글은 항상 보이게
    add.mutate(text, { onSuccess: () => setBody('') })
  }

  return (
    <>
      <div
        ref={scroller}
        className={styles.feed}
        onScroll={onScroll}
        role="log"
        aria-live="polite"
        aria-busy={isLoading || isFetchingNextPage}
      >
        {hasNextPage && <div ref={sentinel} className={styles.sentinel} aria-hidden />}
        {isFetchingNextPage && <p className={styles.loadingOlder}>이전 글 불러오는 중…</p>}
        {isLoading && <p className={styles.state}>불러오는 중…</p>}
        {error && <p className={styles.state}>불러오기 실패</p>}
        {!isLoading && !error && posts.length === 0 && (
          <p className={styles.state}>아직 아무도 글을 남기지 않았어요. 첫 메시지를 남겨 보세요.</p>
        )}
        {posts.length > 0 && (
          <ul className={styles.list}>
            {posts.map((p, i) => (
              <Bubble
                key={p.id}
                post={p}
                owner={p.artist_id === artistId}
                ownerImage={artist.image_url}
                // 같은 사람이 이어 쓰면 이름은 첫 말풍선에만 단다.
                continued={i > 0 && posts[i - 1].user_id === p.user_id}
                mine={p.user_id === user?.id}
                canDelete={p.user_id === user?.id || isAdmin}
                deleting={remove.isPending}
                onDelete={() => remove.mutate(p.id)}
              />
            ))}
          </ul>
        )}
      </div>

      {isAuthed ? (
        <form className={styles.form} onSubmit={submit}>
          <input
            className={styles.input}
            value={body}
            maxLength={WALL_MAX_LEN}
            onChange={(e) => setBody(e.target.value)}
            placeholder={blocked ? '오늘은 더 남길 수 없어요. 내일 다시 만나요' : '한마디 남기기'}
            disabled={blocked}
            aria-label="담벼락 메시지"
          />
          <span className={styles.counter}>
            {body.length}/{WALL_MAX_LEN}
          </span>
          <button type="submit" className={styles.send} disabled={!text || add.isPending || blocked} aria-label="보내기">
            <SendHorizontal size={16} strokeWidth={2.2} />
          </button>
        </form>
      ) : null}
      {isAuthed && remaining !== null && (
        <p className={styles.quota}>
          {blocked
            ? `오늘 남길 수 있는 글을 다 썼어요 (하루 ${quota!.limit}번)`
            : `오늘 이 담벼락에 ${remaining}번 더 남길 수 있어요 (하루 ${quota!.limit}번)`}
        </p>
      )}
      {!isAuthed && (
        <button type="button" className={styles.login} onClick={startKakaoLogin}>
          로그인하고 담벼락에 글 남기기
        </button>
      )}
    </>
  )
}

function Bubble({
  post,
  owner,
  ownerImage,
  continued,
  mine,
  canDelete,
  deleting,
  onDelete,
}: {
  post: WallPost
  owner: boolean
  ownerImage: string | null
  continued: boolean
  mine: boolean
  canDelete: boolean
  deleting: boolean
  onDelete: () => void
}) {
  const verified = post.artist_id != null
  const name = verified ? post.artist_name || post.nickname : post.nickname || '익명'
  const nameHref = verified
    ? `/artists/${encodeURIComponent(post.artist_id!)}`
    : `/users/${encodeURIComponent(post.user_id)}`
  const cls = [styles.msg, owner ? styles.left : styles.right, continued && styles.continued]
    .filter(Boolean)
    .join(' ')

  return (
    <li className={cls}>
      {owner && (
        <span className={styles.avatar} aria-hidden>
          {!continued && <Avatar src={ownerImage} name={name} size={32} />}
        </span>
      )}
      <div className={styles.stack}>
        {!continued && (
          <Link
            to={nameHref}
            className={verified ? `${styles.nick} ${styles.verified}` : styles.nick}
            title={verified ? `인증된 아티스트 (${post.nickname})` : undefined}
          >
            {name}
            {verified && <BadgeCheck size={13} strokeWidth={2.4} aria-label="인증된 아티스트" />}
          </Link>
        )}
        <div className={styles.bubbleRow}>
          <p className={[styles.bubble, owner && styles.ownerBubble, mine && !owner && styles.mineBubble].filter(Boolean).join(' ')}>
            {post.body}
          </p>
          <div className={styles.side}>
            <time className={styles.time} dateTime={post.created_at}>
              {stamp(post.created_at)}
            </time>
            {canDelete && (
              <button type="button" className={styles.remove} disabled={deleting} aria-label="메시지 삭제" onClick={onDelete}>
                <Trash2 size={12} strokeWidth={2.2} />
              </button>
            )}
          </div>
        </div>
      </div>
    </li>
  )
}
