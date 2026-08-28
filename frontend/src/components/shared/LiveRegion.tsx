import styles from './shared.module.css'

export function LiveRegion({
  message,
  assertive = false,
}: {
  message: string
  assertive?: boolean
}) {
  return (
    <div
      className={styles.visuallyHidden}
      aria-live={assertive ? 'assertive' : 'polite'}
      aria-atomic="true"
    >
      {message}
    </div>
  )
}
