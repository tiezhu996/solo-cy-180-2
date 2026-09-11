// 用户管理页（仅管理员）：角色只能在这里由管理员调整，公开注册恒为采访员。
import { useCallback, useEffect, useState } from 'react'
import DataTable from '../../components/DataTable'
import { useAuth } from '../../hooks/useAuth'
import { usePagination } from '../../hooks/usePagination'
import { deleteUser, listUsers, updateUserRole } from '../../api/user'
import { ROLE_ADMIN, ROLE_OPTIONS, ROLE_TEXT } from '../../constants'
import { formatDateTime } from '../../utils/format'
import type { User } from '../../api/types'

export default function UsersPage() {
  const { user } = useAuth([ROLE_ADMIN])
  const { page, pageSize, total, setTotal, setPage } = usePagination(1, 20)
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const fetchUsers = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await listUsers({ page, page_size: pageSize })
      setUsers(res.list)
      setTotal(res.total)
    } catch (e) {
      setError(e instanceof Error ? e.message : '用户列表加载失败')
    } finally {
      setLoading(false)
    }
  }, [page, pageSize, setTotal])

  useEffect(() => {
    fetchUsers()
  }, [fetchUsers])

  const flash = (msg: string) => {
    setMessage(msg)
    setTimeout(() => setMessage(''), 3000)
  }

  const handleRoleChange = async (id: number, role: string) => {
    try {
      await updateUserRole(id, role)
      setUsers((list) => list.map((u) => (u.id === id ? { ...u, role: role as User['role'] } : u)))
      flash('角色已更新')
    } catch (e) {
      setError(e instanceof Error ? e.message : '角色更新失败')
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteUser(id)
      setUsers((list) => list.filter((u) => u.id !== id))
      flash('用户已删除')
    } catch (e) {
      setError(e instanceof Error ? e.message : '删除失败')
    }
  }

  return (
    <div className="page">
      <div className="page-header">
        <h2>用户与角色管理</h2>
        <span className="muted">公开注册仅获得「采访员」，角色由管理员在此调整</span>
      </div>
      {message && <div className="toast success">{message}</div>}
      {error && <div className="toast error">{error}</div>}
      <DataTable<User>
        loading={loading}
        rows={users}
        rowKey={(u) => u.id}
        columns={[
          { key: 'id', title: 'ID', render: (u) => u.id },
          { key: 'username', title: '用户名', render: (u) => u.username },
          { key: 'display_name', title: '昵称', render: (u) => u.display_name },
          { key: 'email', title: '邮箱', render: (u) => u.email || '-' },
          {
            key: 'role',
            title: '角色',
            render: (u) => (
              <select
                value={u.role}
                disabled={u.id === user?.id}
                onChange={(e) => handleRoleChange(u.id, e.target.value)}
              >
                {ROLE_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}（{ROLE_TEXT[opt.value]}）
                  </option>
                ))}
              </select>
            ),
          },
          { key: 'created_at', title: '注册时间', render: (u) => formatDateTime(u.created_at) },
          {
            key: 'actions',
            title: '操作',
            render: (u) =>
              u.role === ROLE_ADMIN || u.id === user?.id ? (
                <span className="muted">不可删除</span>
              ) : (
                <button className="btn btn-danger btn-small" onClick={() => handleDelete(u.id)}>
                  删除
                </button>
              ),
          },
        ]}
        emptyText="暂无用户"
      />
      <div className="pagination">
        <button className="btn btn-plain btn-small" disabled={page <= 1} onClick={() => setPage(page - 1)}>
          上一页
        </button>
        <span>
          第 {page} / {Math.max(1, Math.ceil(total / pageSize))} 页，共 {total} 条
        </span>
        <button
          className="btn btn-plain btn-small"
          disabled={page >= Math.ceil(total / pageSize)}
          onClick={() => setPage(page + 1)}
        >
          下一页
        </button>
      </div>
    </div>
  )
}
