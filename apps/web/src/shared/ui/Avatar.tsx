import styles from './Avatar.module.css'

interface Props {
  src?: string | null
  name: string
  /** Any CSS width — a number is px. Height always follows as a 1:1 square. */
  size?: number | string
  /** 'circle' (default) or 'square' — a rounded square, used on the artist detail header. */
  shape?: 'circle' | 'square'
  className?: string
}

// One artist avatar for every surface (card / detail header / admin rows).
// The clip lives on the box, never on the <img>, so a non-square source is
// cropped to the shape instead of rendering distorted.
export function Avatar({ src, name, size = '100%', shape = 'circle', className }: Props) {
  const classes = [styles.avatar, shape === 'square' && styles.square, className]
    .filter(Boolean)
    .join(' ')
  return (
    <div className={classes} style={{ width: size }}>
      {src ? <img src={src} alt={name} loading="lazy" /> : <span className={styles.initial}>{name.slice(0, 1)}</span>}
    </div>
  )
}
