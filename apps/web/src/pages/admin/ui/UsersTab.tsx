import { useState } from 'react'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Heart, MessageSquare, Search, Star, Trash2, UserPlus, X } from 'lucide-react'
import { Avatar } from '@/shared/ui/Avatar'
import { useDebouncedValue } from '@/shared/lib/useDebouncedValue'
import { formatDeleteAfter, useAuth } from '@/entities/session'
import { useToast } from '@/shared/ui/toast'
import { fetchAdminUsers, purgeAdminUser, USERS_PAGE_SIZE } from '../api/usersAdminApi'
import type { AdminUser } from '../api/usersAdminApi'
import styles from './AdminPage.module.css'

const nf = new Intl.NumberFormat('ko-KR')
const df = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium' })

// 가입 최신순 회원 목록. 조작은 즉시 영구 삭제 하나뿐이다(탈퇴 유예 기간을 건너뛴다).
export function UsersTab() {
  const [input, setInput] = useState('')
  const q = useDebouncedValue(input.trim(), 300)

  const { data, isLoading, error, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ['admin-users', q],
    queryFn: ({ pageParam }) => fetchAdminUsers(q, pageParam),
    initialPageParam: 0,
    getNextPageParam: (last, pages) =>
      last.length < USERS_PAGE_SIZE ? undefined : pages.length * USERS_PAGE_SIZE,
  })
  const users = data?.pages.flat() ?? []
  const { user: me } = useAuth()
  const qc = useQueryClient()
  const toast = useToast()
  const purge = useMutation({
    mutationFn: (u: AdminUser) => purgeAdminUser(u.id),
    onSuccess: (_r, u) => {
      qc.invalidateQueries({ queryKey: ['admin-users'] })
      toast(`${u.nickname || u.id} 회원을 삭제했습니다`)
    },
    onError: (e: Error) => toast(e.message || '회원 삭제에 실패했습니다', 'error'),
  })
  const confirmPurge = (u: AdminUser) => {
    if (window.confirm(`${u.nickname || u.id} 회원과 활동 기록을 지금 영구 삭제할까요? 되돌릴 수 없습니다.`)) purge.mutate(u)
  }

  return (
    <div className={styles.aliasWrap}>
      <div className={styles.searchForm}>
        <Search size={15} className={styles.searchIcon} aria-hidden />
        <input
          className={styles.searchInput}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="닉네임 또는 ID로 검색"
          aria-label="회원 검색"
        />
        {input && (
          <button type="button" className={styles.clearBtn} onClick={() => setInput('')} aria-label="검색어 지우기">
            <X size={14} />
          </button>
        )}
      </div>

      {isLoading && <p className={styles.state}>불러오는 중…</p>}
      {error && <p className={styles.state}>불러오기 실패: {String(error)}</p>}
      {!isLoading && !error && users.length === 0 && <p className={styles.state}>회원이 없습니다.</p>}

      <div className={styles.rows}>
        {users.map((u) => (
          <div key={u.id} className={styles.row}>
            <Avatar name={u.nickname || u.id} size={40} />
            <div className={styles.rowInfo}>
              <span className={styles.rowName} title={u.nickname}>
                {u.nickname || '(닉네임 없음)'}
                {u.role === 'admin' && <span className={styles.roleBadge}>관리자</span>}
                {u.delete_after && (
                  <span className={styles.withdrawBadge}>{formatDeleteAfter(u.delete_after)} 삭제 예정</span>
                )}
              </span>
              <span className={styles.rowId} title={u.id}>
                {u.id} · {df.format(new Date(u.created_at))} 가입
              </span>
            </div>
            <div className={styles.userCounts}>
              <span title="별점">
                <Star size={13} strokeWidth={2.4} aria-hidden />
                {nf.format(u.rating_count)}
              </span>
              <span title="댓글">
                <MessageSquare size={13} strokeWidth={2.4} aria-hidden />
                {nf.format(u.comment_count)}
              </span>
              <span title="즐겨찾기">
                <Heart size={13} strokeWidth={2.4} aria-hidden />
                {nf.format(u.favorite_count)}
              </span>
              <span title="팔로우">
                <UserPlus size={13} strokeWidth={2.4} aria-hidden />
                {nf.format(u.follow_count)}
              </span>
            </div>
            {u.id !== me?.id && (
              <button
                type="button"
                className={styles.purgeBtn}
                disabled={purge.isPending}
                onClick={() => confirmPurge(u)}
                title="즉시 영구 삭제"
                aria-label={`${u.nickname || u.id} 즉시 영구 삭제`}
              >
                <Trash2 size={15} />
              </button>
            )}
          </div>
        ))}
      </div>

      {hasNextPage && (
        <button type="button" className={styles.moreBtn} disabled={isFetchingNextPage} onClick={() => fetchNextPage()}>
          {isFetchingNextPage ? '불러오는 중…' : '더 보기'}
        </button>
      )}
    </div>
  )
}
