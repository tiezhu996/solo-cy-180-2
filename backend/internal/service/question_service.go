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

// QuestionService 采访问题业务接口。
type QuestionService interface {
	Create(actor *model.User, projectID uint, req *dto.CreateQuestionRequest) (*model.Question, error)
	ListByProject(actor *model.User, projectID uint) ([]model.Question, error)
	Update(actor *model.User, id uint, req *dto.UpdateQuestionRequest) (*model.Question, error)
	Delete(actor *model.User, id uint) error
	CountByProject(projectID uint) (int64, error)
}

type questionService struct {
	questionRepo repository.QuestionRepository
	projectRepo  repository.ProjectRepository
	logger       *slog.Logger
}

// NewQuestionService 构造问题服务。
func NewQuestionService(questionRepo repository.QuestionRepository, projectRepo repository.ProjectRepository, logger *slog.Logger) QuestionService {
	return &questionService{questionRepo: questionRepo, projectRepo: projectRepo, logger: logger}
}

// loadWritableProject 取项目并校验「采集内容写权限 + 归档写保护」。
func (s *questionService) loadWritableProject(actor *model.User, projectID uint, entity, action string) (*model.Project, error) {
	project, err := s.projectRepo.FindByID(projectID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("项目 %d 不存在", projectID), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询项目 %d 失败", projectID), err)
	}
	if !canManageProjectContent(actor, project) {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, entity, action))
		return nil, roleWriteError(actor, entity, action)
	}
	if err := ensureWritable(actor, project, entity, action); err != nil {
		return nil, err
	}
	return project, nil
}

func (s *questionService) Create(actor *model.User, projectID uint, req *dto.CreateQuestionRequest) (*model.Question, error) {
	if _, err := s.loadWritableProject(actor, projectID, "采访问题", "添加"); err != nil {
		return nil, err
	}
	question := &model.Question{
		ProjectID: projectID,
		Content:   req.Content,
		SortOrder: req.SortOrder,
	}
	if err := s.questionRepo.Create(question); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("向项目 %d 添加问题失败", projectID), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogQuestionCreate, actor.Username, projectID, question.Content))
	return question, nil
}

func (s *questionService) ListByProject(actor *model.User, projectID uint) ([]model.Question, error) {
	// 读也受归属约束：采访员只能读自己负责项目的问题。
	if _, err := func() (*model.Project, error) {
		project, err := s.projectRepo.FindByID(projectID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("项目 %d 不存在", projectID), err)
			}
			return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询项目 %d 失败", projectID), err)
		}
		if actor.Role == constants.RoleInterviewer && project.CreatedBy != actor.ID {
			return nil, roleWriteError(actor, "项目", "访问")
		}
		return project, nil
	}(); err != nil {
		return nil, err
	}
	questions, err := s.questionRepo.ListByProject(projectID)
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询项目 %d 问题列表失败", projectID), err)
	}
	return questions, nil
}

func (s *questionService) Update(actor *model.User, id uint, req *dto.UpdateQuestionRequest) (*model.Question, error) {
	question, err := s.questionRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("问题 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询问题 %d 失败", id), err)
	}
	if _, err := s.loadWritableProject(actor, question.ProjectID, "采访问题", "修改"); err != nil {
		return nil, err
	}
	if req.Content != "" {
		question.Content = req.Content
	}
	if req.SortOrder != 0 {
		question.SortOrder = req.SortOrder
	}
	if err := s.questionRepo.Update(question); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("更新问题 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogQuestionUpdate, actor.Username, question.ID, question.Content))
	return question, nil
}

func (s *questionService) Delete(actor *model.User, id uint) error {
	question, err := s.questionRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(constants.CodeNotFound, fmt.Sprintf("问题 %d 不存在", id), err)
		}
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询问题 %d 失败", id), err)
	}
	if _, err := s.loadWritableProject(actor, question.ProjectID, "采访问题", "删除"); err != nil {
		return err
	}
	if err := s.questionRepo.Delete(id); err != nil {
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("删除问题 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogQuestionDelete, actor.Username, id))
	return nil
}

func (s *questionService) CountByProject(projectID uint) (int64, error) {
	return s.questionRepo.CountByProject(projectID)
}
