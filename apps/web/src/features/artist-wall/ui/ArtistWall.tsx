import { useLayoutEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { MessagesSquare, SendHorizontal, Trash2 } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { startKakaoLogin } from '@/features/auth'
import { WALL_MAX_LEN } from '@/features/artist-wall/api/wallApi'
import { useAddWallPost, useRemoveWallPost, useWall } from '@/features/artist-wall/model/useWall'
import styles from './ArtistWall.module.css'

const timeFmt = new Intl.DateTimeFormat('ko-KR', { hour: '2-digit', minute: '2-digit' })
const dateFmt = new Intl.DateTimeFormat('ko-KR', { month: 'numeric', day: 'numeric' })
const NEAR_BOTTOM_PX = 80

function stamp(iso: string) {
  const d = new Date(iso)
  const today = new Date().toDateString() === d.toDateString()
  return today ? timeFmt.format(d) : `${dateFmt.format(d)} ${timeFmt.format(d)}`
}

// 아티스트 담벼락. 채팅창처럼 아래가 최신이고, 위로 올리면 과거 메시지를 더 불러온다.
export function ArtistWall({ artistId }: { artistId: string }) {
  const { user, isAuthed, isAdmin } = useAuth()
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
    if (el.scrollTop === 0 && hasNextPage && !isFetchingNextPage) fetchNextPage()
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!text || add.isPending) return
    prev.current.nearBottom = true // 내가 쓴 글은 항상 보이게
    add.mutate(text, { onSuccess: () => setBody('') })
  }

  return (
    <section className={styles.wall} aria-label="담벼락">
      <h2 className={styles.title}>
        <MessagesSquare size={17} strokeWidth={2.2} aria-hidden />
        담벼락
      </h2>

      <div
        ref={scroller}
        className={styles.feed}
        onScroll={onScroll}
        role="log"
        aria-live="polite"
        aria-busy={isLoading || isFetchingNextPage}
      >
        {hasNextPage && (
          <button
            type="button"
            className={styles.more}
            disabled={isFetchingNextPage}
            onClick={() => {
              const el = scroller.current
              if (el) Object.assign(prev.current, { height: el.scrollHeight, top: el.scrollTop })
              fetchNextPage()
            }}
          >
            {isFetchingNextPage ? '불러오는 중…' : '이전 메시지'}
          </button>
        )}
        {isLoading && <p className={styles.state}>불러오는 중…</p>}
        {error && <p className={styles.state}>불러오기 실패</p>}
        {!isLoading && !error && posts.length === 0 && (
          <p className={styles.state}>아직 아무도 글을 남기지 않았어요. 첫 메시지를 남겨 보세요.</p>
        )}
        <ul className={styles.list}>
          {posts.map((p) => {
            const mine = p.user_id === user?.id
            return (
              <li key={p.id} className={mine ? `${styles.line} ${styles.mine}` : styles.line}>
                <Link to={`/users/${encodeURIComponent(p.user_id)}`} className={styles.nick}>
                  {p.nickname || '익명'}
                </Link>
                <span className={styles.body}>{p.body}</span>
                <time className={styles.time} dateTime={p.created_at}>
                  {stamp(p.created_at)}
                </time>
                {(mine || isAdmin) && (
                  <button
                    type="button"
                    className={styles.remove}
                    disabled={remove.isPending}
                    aria-label="메시지 삭제"
                    onClick={() => remove.mutate(p.id)}
                  >
                    <Trash2 size={12} strokeWidth={2.2} />
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      </div>

      {isAuthed ? (
        <form className={styles.form} onSubmit={submit}>
          <input
            className={styles.input}
            value={body}
            maxLength={WALL_MAX_LEN}
            onChange={(e) => setBody(e.target.value)}
            placeholder="한마디 남기기"
            aria-label="담벼락 메시지"
          />
          <span className={styles.counter}>
            {body.length}/{WALL_MAX_LEN}
          </span>
          <button type="submit" className={styles.send} disabled={!text || add.isPending} aria-label="보내기">
            <SendHorizontal size={16} strokeWidth={2.2} />
          </button>
        </form>
      ) : (
        <button type="button" className={styles.login} onClick={startKakaoLogin}>
          로그인하고 담벼락에 글 남기기
        </button>
      )}
    </section>
  )
}
