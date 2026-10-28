export function StatusBadge({ status }: { status: number }) {
  return status === 1 ? (
    <span className="badge badge-on">启用</span>
  ) : (
    <span className="badge badge-off">停用</span>
  )
}
