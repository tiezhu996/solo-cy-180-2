package service

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/oralhistory/oralhistory/internal/constants"
	"github.com/oralhistory/oralhistory/internal/dto"
	"github.com/oralhistory/oralhistory/internal/model"
	"github.com/oralhistory/oralhistory/internal/repository"
	"github.com/oralhistory/oralhistory/internal/util"
)

// ProjectService 采访项目业务接口。
type ProjectService interface {
	Create(actor *model.User, req *dto.CreateProjectRequest) (*model.Project, error)
	Get(actor *model.User, id uint) (*model.Project, error)
	// List 采访员只能看到自己负责的项目；管理员/档案员可看全部。
	List(actor *model.User, page, pageSize int, status string) ([]model.Project, int64, error)
	ListMine(actorID uint, page, pageSize int) ([]model.Project, int64, error)
	Update(actor *model.User, id uint, req *dto.UpdateProjectRequest) (*model.Project, error)
	TransitionStatus(actor *model.User, id uint, status string) (*model.Project, error)
	Delete(actor *model.User, id uint) error
	Stats(actor *model.User) (map[string]any, error)
}

type projectService struct {
	projectRepo repository.ProjectRepository
	logger      *slog.Logger
}

// NewProjectService 构造项目服务。
func NewProjectService(projectRepo repository.ProjectRepository, logger *slog.Logger) ProjectService {
	return &projectService{projectRepo: projectRepo, logger: logger}
}

func (s *projectService) Create(actor *model.User, req *dto.CreateProjectRequest) (*model.Project, error) {
	// 只有管理员与采访员能创建项目；档案员不能维护项目资料。
	if actor.Role != constants.RoleAdmin && actor.Role != constants.RoleInterviewer {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "project", "create"))
		return nil, roleWriteError(actor, "项目", "创建")
	}
	status := req.Status
	if status == "" {
		status = constants.ProjectStatusDraft
	}
	if !constants.ValidProjectStatus(status) {
		return nil, util.NewAppError(constants.CodeValidation, fmt.Sprintf("项目状态 %s 不合法", status), nil)
	}
	project := &model.Project{
		Title:           req.Title,
		IntervieweeName: req.IntervieweeName,
		BirthYear:       req.BirthYear,
		Background:      req.Background,
		Status:          status,
		CreatedBy:       actor.ID,
	}
	if err := s.projectRepo.Create(project); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("创建项目 %s 失败", req.Title), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogProjectCreate, actor.Username, project.Title, project.IntervieweeName, project.Status))
	return project, nil
}

// loadProjectForRead 取项目并做读权限校验（采访员只能读自己负责的项目）。
func (s *projectService) loadProjectForRead(actor *model.User, id uint) (*model.Project, error) {
	project, err := s.projectRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("项目 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询项目 %d 失败", id), err)
	}
	if actor.Role == constants.RoleInterviewer && project.CreatedBy != actor.ID {
		s.logger.Warn(fmt.Sprintf(constants.LogOwnerDenied, actor.Username, project.ID, project.CreatedBy, "read"))
		return nil, roleWriteError(actor, "项目", "访问")
	}
	return project, nil
}

func (s *projectService) Get(actor *model.User, id uint) (*model.Project, error) {
	return s.loadProjectForRead(actor, id)
}

func (s *projectService) List(actor *model.User, page, pageSize int, status string) ([]model.Project, int64, error) {
	if status != "" && !constants.ValidProjectStatus(status) {
		return nil, 0, util.NewAppError(constants.CodeValidation, fmt.Sprintf("项目状态 %s 不合法", status), nil)
	}
	// 采访员的项目列表强制收敛为「我负责的项目」。
	var creatorID uint
	if actor.Role == constants.RoleInterviewer {
		creatorID = actor.ID
	}
	projects, total, err := s.projectRepo.List(page, pageSize, status, creatorID)
	if err != nil {
		return nil, 0, util.NewAppError(constants.CodeInternal, "项目列表查询失败", err)
	}
	return projects, total, nil
}

func (s *projectService) ListMine(actorID uint, page, pageSize int) ([]model.Project, int64, error) {
	projects, total, err := s.projectRepo.ListByUser(actorID, page, pageSize)
	if err != nil {
		return nil, 0, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询用户 %d 项目列表失败", actorID), err)
	}
	return projects, total, nil
}

func (s *projectService) Update(actor *model.User, id uint, req *dto.UpdateProjectRequest) (*model.Project, error) {
	project, err := s.projectRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("项目 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询项目 %d 失败", id), err)
	}
	// 仅管理员可改任意项目；采访员只能改自己负责的项目；档案员不能改项目资料。
	if !canManageProjectContent(actor, project) {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "project", "update"))
		return nil, roleWriteError(actor, "项目资料", "修改")
	}
	if err := ensureWritable(actor, project, "项目资料", "修改"); err != nil {
		return nil, err
	}
	if req.Title != "" {
		project.Title = req.Title
	}
	if req.IntervieweeName != "" {
		project.IntervieweeName = req.IntervieweeName
	}
	if req.BirthYear != 0 {
		project.BirthYear = req.BirthYear
	}
	if req.Background != "" {
		project.Background = req.Background
	}
	if err := s.projectRepo.Update(project); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("更新项目 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogProjectUpdate, actor.Username, project.ID, project.Title, project.Status))
	return project, nil
}

func (s *projectService) TransitionStatus(actor *model.User, id uint, status string) (*model.Project, error) {
	if !constants.ValidProjectStatus(status) {
		return nil, util.NewAppError(constants.CodeValidation, fmt.Sprintf("项目状态 %s 不合法", status), nil)
	}
	// 并发场景使用 SELECT ... FOR UPDATE 锁定行。
	project, err := s.projectRepo.FindByIDForUpdate(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("项目 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询项目 %d 失败", id), err)
	}
	// 已归档是终态：任何角色（含管理员）都不得再做状态流转。
	if err := ensureWritable(actor, project, "项目", "状态流转"); err != nil {
		return nil, err
	}
	// 归档动作单独授权（管理员/档案员可归档任意项目，采访员只能归档自己的项目）；
	// 其余状态流转按「能否维护项目采集内容」授权。
	if status == constants.ProjectStatusArchived {
		if !canArchiveProject(actor, project) {
			s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "project", "archive"))
			return nil, roleWriteError(actor, "项目", "归档")
		}
	} else if !canManageProjectContent(actor, project) {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "project", "status"))
		return nil, roleWriteError(actor, "项目", "状态流转")
	}
	if !constants.CanTransitionProject(project.Status, status) {
		return nil, util.NewAppError(constants.CodeProjectStatus,
			fmt.Sprintf("项目 %d 状态不允许从 %s 流转到 %s", id, project.Status, status), nil)
	}
	from := project.Status
	project.Status = status
	if err := s.projectRepo.UpdateStatus(project); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("项目 %d 状态更新失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogProjectStatus, actor.Username, project.ID, from, status))
	return project, nil
}

func (s *projectService) Delete(actor *model.User, id uint) error {
	project, err := s.projectRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(constants.CodeNotFound, fmt.Sprintf("项目 %d 不存在", id), err)
		}
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询项目 %d 失败", id), err)
	}
	if !canManageProjectContent(actor, project) {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "project", "delete"))
		return roleWriteError(actor, "项目", "删除")
	}
	if err := ensureWritable(actor, project, "项目", "删除"); err != nil {
		return err
	}
	if err := s.projectRepo.Delete(id); err != nil {
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("删除项目 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogProjectDelete, actor.Username, id))
	return nil
}

func (s *projectService) Stats(actor *model.User) (map[string]any, error) {
	var creatorID uint
	if actor.Role == constants.RoleInterviewer {
		creatorID = actor.ID
	}
	total, err := s.projectRepo.CountByCreator(creatorID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternal, "项目统计失败", err)
	}
	return map[string]any{"project_total": total}, nil
}
