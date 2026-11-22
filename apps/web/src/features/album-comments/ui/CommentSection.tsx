import { useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Pencil, Trash2 } from 'lucide-react'
import { useAuth } from '@/entities/session'
import { useStartSignIn } from '@/features/auth'
import { apiUrl } from '@/shared/config'
import { Avatar } from '@/shared/ui/Avatar'
import type { Comment, CommentThread } from '@/features/album-comments/api/commentApi'
import {
  useAddComment,
  useComments,
  useEditComment,
  useMoreReplies,
  useRemoveComment,
} from '@/features/album-comments/model/useComments'
import { CommentForm } from './CommentForm'
import styles from './CommentSection.module.css'

const fmt = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' })
// CommentSection.module.css의 --avatar-indent(아바타 + 간격), 답글 세로선 위치(.reply)와 맞춘다.
const AVATAR_SIZE = 24

export function CommentSection({ albumId }: { albumId: string }) {
  const { user, isAuthed, isAdmin } = useAuth()
  const startSignIn = useStartSignIn()
  const { threads, count, isLoading, hasNextPage, fetchNextPage, isFetchingNextPage } = useComments(albumId)
  const add = useAddComment(albumId)
  const edit = useEditComment(albumId)
  const remove = useRemoveComment(albumId)
  // Only one comment is editable at a time.
  const [editingId, setEditingId] = useState<number | null>(null)
  // Only one reply box is open at a time — id of the comment being replied to.
  const [replyTo, setReplyTo] = useState<number | null>(null)
  // 답글 더 보기를 누른 스레드. 답글을 단 스레드도 넣어서 방금 쓴 답글이 미리보기 뒤에 숨지 않게 한다.
  const [expanded, setExpanded] = useState<ReadonlySet<number>>(new Set())
  const expand = (id: number) => setExpanded((prev) => new Set(prev).add(id))

  const isMine = (c: Comment) => !c.deleted && c.user_id === user?.id
  // Admins can remove a comment but never rewrite it — that's the author's alone.
  const canDelete = (c: Comment) => !c.deleted && (isAdmin || c.user_id === user?.id)

  const row = (c: Comment, isReply: boolean) => (
    <li key={c.id} className={isReply ? styles.reply : styles.comment}>
      <div className={styles.body}>
        <div className={styles.meta}>
          {/* 삭제된 댓글은 작성자를 드러내지 않으므로 사진과 링크 없이 빈 원만 둔다. */}
          {c.deleted ? (
            <Avatar name="" size={AVATAR_SIZE} />
          ) : (
            <>
              <Link
                to={`/users/${encodeURIComponent(c.user_id)}`}
                className={styles.avatarLink}
                tabIndex={-1}
                aria-hidden
              >
                <Avatar src={c.avatar_url ? apiUrl(c.avatar_url) : null} name={c.nickname} size={AVATAR_SIZE} />
              </Link>
              <Link to={`/users/${encodeURIComponent(c.user_id)}`} className={styles.nickname}>
                {c.nickname}
              </Link>
              <time dateTime={c.created_at}>{fmt.format(new Date(c.created_at))}</time>
            </>
          )}
          {c.edited && <span className={styles.edited}>수정됨</span>}
          <span className={styles.actions}>
            {isMine(c) && (
              <button
                type="button"
                className={styles.iconBtn}
                disabled={edit.isPending}
                aria-label="댓글 수정"
                onClick={() => setEditingId(editingId === c.id ? null : c.id)}
              >
                <Pencil size={13} strokeWidth={2.2} />
              </button>
            )}
            {canDelete(c) && (
              <button
                type="button"
                className={styles.iconBtn}
                disabled={remove.isPending}
                aria-label="댓글 삭제"
                onClick={() => remove.mutate(c.id)}
              >
                <Trash2 size={13} strokeWidth={2.2} />
              </button>
            )}
          </span>
        </div>
        <div className={styles.content}>
          {editingId === c.id ? (
            <div className={styles.editForm}>
              <CommentForm
                autoFocus
                initialBody={c.body}
                submitLabel="수정"
                pending={edit.isPending}
                onCancel={() => setEditingId(null)}
                onSubmit={(body) => {
                  edit.mutate({ id: c.id, body })
                  setEditingId(null)
                }}
              />
            </div>
          ) : (
            <p className={c.deleted ? styles.removed : styles.text}>{c.deleted ? '삭제된 댓글입니다' : c.body}</p>
          )}
          {!isReply && isAuthed && !c.deleted && replyTo !== c.id && editingId !== c.id && (
            <button type="button" className={styles.replyBtn} onClick={() => setReplyTo(c.id)}>
              답글
            </button>
          )}
        </div>
      </div>
    </li>
  )

  return (
    <section className={styles.section}>
      <h2 className={styles.title}>
        댓글 {count > 0 && <span className={styles.count}>{count}</span>}
      </h2>

      {isAuthed ? (
        <div className={styles.composer}>
          <Avatar src={user?.avatar_url ? apiUrl(user.avatar_url) : null} name={user?.nickname || '나'} size={32} />
          <div className={styles.composerForm}>
            <CommentForm onSubmit={(body) => add.mutate({ body })} pending={add.isPending} />
          </div>
        </div>
      ) : (
        <button type="button" className={styles.login} onClick={startSignIn}>
          로그인하고 댓글 남기기
        </button>
      )}

      {isLoading && <p className={styles.state}>불러오는 중…</p>}
      {!isLoading && threads.length === 0 && <p className={styles.state}>첫 댓글을 남겨보세요.</p>}

      <ul className={styles.list}>
        {threads.map((comment) => (
          <li key={comment.id}>
            <ul className={styles.thread}>
              {row(comment, false)}
              <ThreadReplies
                albumId={albumId}
                thread={comment}
                expanded={expanded.has(comment.id)}
                onExpand={() => expand(comment.id)}
                renderReply={(r) => row(r, true)}
              />
            </ul>
            {replyTo === comment.id && (
              <div className={styles.replyForm}>
                <CommentForm
                  autoFocus
                  placeholder={`${comment.nickname}님에게 답글`}
                  pending={add.isPending}
                  onCancel={() => setReplyTo(null)}
                  onSubmit={(body) => {
                    add.mutate({ body, parentId: comment.id })
                    expand(comment.id)
                    setReplyTo(null)
                  }}
                />
              </div>
            )}
          </li>
        ))}
      </ul>

      {hasNextPage && (
        <button type="button" className={styles.more} disabled={isFetchingNextPage} onClick={() => fetchNextPage()}>
          {isFetchingNextPage ? '불러오는 중…' : '댓글 더 보기'}
        </button>
      )}
    </section>
  )
}

function ThreadReplies({
  albumId,
  thread,
  expanded,
  onExpand,
  renderReply,
}: {
  albumId: string
  thread: CommentThread
  expanded: boolean
  onExpand: () => void
  renderReply: (c: Comment) => ReactNode
}) {
  const after = thread.replies.at(-1)?.id ?? null
  const more = useMoreReplies(albumId, thread.id, after, expanded)
  const previewIds = new Set(thread.replies.map((r) => r.id))
  const shown = [...thread.replies, ...more.replies.filter((r) => !previewIds.has(r.id))]
  const remaining = Math.max(thread.reply_count - shown.length, 0)
  const canLoad = expanded ? more.isLoading || more.hasNextPage : remaining > 0
  const busy = more.isLoading || more.isFetchingNextPage

  return (
    <>
      {shown.map(renderReply)}
      {canLoad && (
        <li className={styles.moreReplies}>
          <button
            type="button"
            className={styles.replyBtn}
            disabled={busy}
            onClick={() => (expanded ? more.fetchNextPage() : onExpand())}
          >
            {busy ? '불러오는 중…' : `답글 ${remaining}개 더 보기`}
          </button>
        </li>
      )}
    </>
  )
}
