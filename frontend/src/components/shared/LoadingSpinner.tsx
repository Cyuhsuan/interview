import styles from './shared.module.css'

export function LoadingSpinner({ label }: { label: string }) {
  return (
    <div className={styles.loadingRow} role="status">
      <span className={styles.spinner} aria-hidden="true" />
      <span>{label}</span>
    </div>
  )
}
