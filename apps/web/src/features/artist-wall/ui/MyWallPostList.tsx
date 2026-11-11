import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { Avatar } from '@/shared/ui/Avatar'
import { useToast } from '@/shared/ui/toast'
import { useInfiniteScroll } from '@/shared/lib/useInfiniteScroll'
import { fetchMyWallPosts, MY_WALL_PAGE_SIZE, removeWallPost } from '@/features/artist-wall/api/wallApi'
import styles from './MyWallPostList.module.css'

const df = new Intl.DateTimeFormat('ko-KR', { dateStyle: 'medium', timeStyle: 'short' })

// 내가 여러 아티스트 담벼락에 남긴 글을 최신순으로 모아 본다.
export function MyWallPostList() {
  const qc = useQueryClient()
  const toast = useToast()
  const { data, isLoading, error, hasNextPage, fetchNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ['my-wall'],
    queryFn: ({ pageParam }) => fetchMyWallPosts(pageParam),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => (last.length < MY_WALL_PAGE_SIZE ? undefined : last[last.length - 1]?.id),
  })
  const posts = useMemo(() => data?.pages.flat() ?? [], [data])
  const sentinel = useInfiniteScroll<HTMLDivElement>(fetchNextPage, !!hasNextPage && !isFetchingNextPage)

  const remove = useMutation({
    mutationFn: removeWallPost,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['my-wall'] })
      qc.invalidateQueries({ queryKey: ['wall'] })
      toast('글을 삭제했습니다')
    },
    onError: () => toast('삭제에 실패했습니다', 'error'),
  })

  if (isLoading) return <p className={styles.state}>불러오는 중…</p>
  if (error) return <p className={styles.state}>불러오기 실패</p>
  if (posts.length === 0) return <p className={styles.state}>아직 담벼락에 남긴 글이 없습니다.</p>

  return (
    <>
      <ul className={styles.list}>
        {posts.map((p) => {
          const name = p.artist_name || p.artist_id
          return (
            <li key={p.id} className={styles.item}>
              <Link to={`/artists/${encodeURIComponent(p.artist_id)}`} className={styles.artist}>
                <Avatar src={p.artist_image_url} name={name} size={36} />
              </Link>
              <div className={styles.main}>
                <div className={styles.meta}>
                  <Link to={`/artists/${encodeURIComponent(p.artist_id)}`} className={styles.artistName}>
                    {name}
                  </Link>
                  <time dateTime={p.created_at}>{df.format(new Date(p.created_at))}</time>
                </div>
                <p className={styles.body}>{p.body}</p>
              </div>
              <button
                type="button"
                className={styles.remove}
                disabled={remove.isPending}
                aria-label="글 삭제"
                onClick={() => remove.mutate(p.id)}
              >
                <Trash2 size={14} strokeWidth={2.2} />
              </button>
            </li>
          )
        })}
      </ul>
      {hasNextPage && <div ref={sentinel} aria-hidden />}
      {isFetchingNextPage && <p className={styles.state}>불러오는 중…</p>}
    </>
  )
}
