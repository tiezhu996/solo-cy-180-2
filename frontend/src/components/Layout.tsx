// 应用主布局：顶部导航 + 内容区。承担登录态路由守卫与角色化导航显隐。
import { useEffect } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuthStore } from '../stores/authStore'
import { ROLE_ADMIN, ROLE_TEXT } from '../constants'

export default function Layout() {
  const { user, initialized, loadMe, logout, hasRole } = useAuthStore()
  const navigate = useNavigate()

  useEffect(() => {
    if (!initialized) {
      loadMe()
    }
  }, [initialized, loadMe])

  useEffect(() => {
    if (initialized && !user) {
      navigate('/login', { replace: true })
    }
  }, [initialized, user, navigate])

  if (!initialized || !user) {
    return <div className="page">加载中…</div>
  }

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">
          <span className="brand-logo">🎙️</span>
          <span>口述历史采集工具</span>
        </div>
        <nav className="nav">
          <NavLink to="/" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')} end>
            采访项目
          </NavLink>
          <NavLink to="/interview" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
            采访工作台
          </NavLink>
          {hasRole(ROLE_ADMIN) && (
            <>
              <NavLink to="/users" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
                用户管理
              </NavLink>
              <NavLink to="/audit" className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}>
                审计日志
              </NavLink>
            </>
          )}
        </nav>
        <div className="user-box">
          <span className="user-role">{ROLE_TEXT[user.role] || user.role}</span>
          <span className="user-name">{user.display_name || user.username}</span>
          <button className="btn btn-plain btn-small" onClick={handleLogout}>
            退出
          </button>
        </div>
      </header>
      <main className="main">
        <Outlet />
      </main>
    </div>
  )
}
