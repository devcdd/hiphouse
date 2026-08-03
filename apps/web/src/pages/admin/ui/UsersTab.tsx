import { useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { Heart, MessageSquare, Search, Star, UserPlus, X } from 'lucide-react'
import { Avatar } from '@/shared/ui/Avatar'
import { useDebouncedValue } from '@/shared/lib/useDebouncedValue'
import { fetchAdminUsers, USERS_PAGE_SIZE } from '../api/usersAdminApi'
import styles from './AdminPage.module.css'

const nf = new Intl.NumberFormat('ko-KR')
const df = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium' })

// 가입 최신순 회원 목록. 보기 전용 — 계정 조작(정지·삭제)은 여기에 없다.
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
