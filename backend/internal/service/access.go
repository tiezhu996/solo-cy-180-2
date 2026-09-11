package service

import (
	"fmt"

	"github.com/oralhistory/oralhistory/internal/constants"
	"github.com/oralhistory/oralhistory/internal/model"
	"github.com/oralhistory/oralhistory/internal/util"
)

// 本文件集中承载三角色写权限规则（RBAC + 数据归属 + 归档写保护）。
//
//	管理员 admin：全局管理，可写任意项目及其子资源。
//	采访员 interviewer：只能写自己负责（created_by = 自己）的项目、问题、录音、节点。
//	档案员 archivist：不能改项目资料；可整理录音摘要与时间轴节点；可归档项目。
//	项目一旦进入 archived，除管理员外一律禁止继续写入。

// roleWriteError 构造角色越权错误，信息中带实体名、动作、角色名、用户名。
func roleWriteError(actor *model.User, entity, action string) error {
	return util.NewAppError(constants.CodeForbidden,
		fmt.Sprintf("%s：角色 %s（用户 %s）不允许对%s执行「%s」操作",
			constants.MsgForbidden, actor.Role, actor.Username, entity, action), nil)
}

// archivedWriteError 构造归档后写入冲突错误。
func archivedWriteError(actor *model.User, projectID uint, entity, action string) error {
	return util.NewAppError(constants.CodeArchivedWrite,
		fmt.Sprintf("项目 %d 已归档，禁止继续写入：角色 %s（用户 %s）不能对%s执行「%s」操作",
			projectID, actor.Role, actor.Username, entity, action), nil)
}

// crossProjectError 构造跨项目写入冲突错误。
func crossProjectError(actor *model.User, projectID, childProjectID uint, entity string) error {
	return util.NewAppError(constants.CodeCrossProject,
		fmt.Sprintf("跨项目写入被拒绝：%s属于项目 %d，不属于目标项目 %d（用户 %s）",
			entity, childProjectID, projectID, actor.Username), nil)
}

// canManageProjectContent 判断角色能否写入某项目的采集内容（项目资料/问题/录音采集）。
// 管理员可写任意项目；采访员只能写自己负责的项目；档案员不能写采集内容。
func canManageProjectContent(actor *model.User, project *model.Project) bool {
	switch actor.Role {
	case constants.RoleAdmin:
		return true
	case constants.RoleInterviewer:
		return project.CreatedBy == actor.ID
	default:
		return false
	}
}

// canCurateProject 判断角色能否在项目内做档案整理（录音摘要、时间轴节点）。
// 管理员全局可整理；采访员只能整理自己负责的项目；档案员可整理任意项目。
func canCurateProject(actor *model.User, project *model.Project) bool {
	switch actor.Role {
	case constants.RoleAdmin:
		return true
	case constants.RoleArchivist:
		return true
	case constants.RoleInterviewer:
		return project.CreatedBy == actor.ID
	default:
		return false
	}
}

// canArchiveProject 判断角色能否把项目流转到归档态。
// 管理员、档案员可归档任意项目；采访员只能归档自己负责的项目。
func canArchiveProject(actor *model.User, project *model.Project) bool {
	switch actor.Role {
	case constants.RoleAdmin, constants.RoleArchivist:
		return true
	case constants.RoleInterviewer:
		return project.CreatedBy == actor.ID
	default:
		return false
	}
}

// ensureWritable 校验项目未归档。归档是 WORM 终态：任何角色（含管理员）都不得
// 再写入，以保证「归档后禁止继续写入」这一硬性业务约束；如确需纠错只能从数据库处理。
func ensureWritable(actor *model.User, project *model.Project, entity, action string) error {
	if project.Status == constants.ProjectStatusArchived {
		return archivedWriteError(actor, project.ID, entity, action)
	}
	return nil
}
