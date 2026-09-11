// 前端权限规则，与后端 internal/service/access.go 保持一致。
// 注意：按钮显隐仅为体验优化，真正的鉴权以后端接口返回 403/409 为准。
import { ROLE_ADMIN, ROLE_ARCHIVIST, ROLE_INTERVIEWER, PROJECT_STATUS_ARCHIVED } from '../constants'
import type { Project } from '../api/types'

export type Role = typeof ROLE_ADMIN | typeof ROLE_INTERVIEWER | typeof ROLE_ARCHIVIST

// 采访员只能处理自己负责（created_by 为自己）的项目。
export function isProjectOwner(role: string | undefined, userId: number | undefined, project: Project): boolean {
  return role === ROLE_ADMIN || (role === ROLE_INTERVIEWER && !!userId && project.created_by === userId)
}

// 项目采集内容：项目资料、问题、录音采集/删除。
export function canManageContent(role: string | undefined, userId: number | undefined, project: Project): boolean {
  if (role === ROLE_ADMIN) return true
  return role === ROLE_INTERVIEWER && !!userId && project.created_by === userId
}

// 档案整理：录音摘要、时间轴节点。档案员可整理任意项目。
export function canCurate(role: string | undefined, userId: number | undefined, project: Project): boolean {
  if (role === ROLE_ADMIN || role === ROLE_ARCHIVIST) return true
  return role === ROLE_INTERVIEWER && !!userId && project.created_by === userId
}

// 归档操作：管理员、档案员可归档任意项目；采访员只能归档自己的项目。
export function canArchive(role: string | undefined, userId: number | undefined, project: Project): boolean {
  if (role === ROLE_ADMIN || role === ROLE_ARCHIVIST) return true
  return role === ROLE_INTERVIEWER && !!userId && project.created_by === userId
}

// 归档后禁止继续写入（管理员也不可改档案内容）。
export function isArchived(project: Project): boolean {
  return project.status === PROJECT_STATUS_ARCHIVED
}

// 页面级：能否看到「新建项目 / 采访采集」入口。
export function canCreateProject(role: string | undefined): boolean {
  return role === ROLE_ADMIN || role === ROLE_INTERVIEWER
}

export function isAdmin(role: string | undefined): boolean {
  return role === ROLE_ADMIN
}
