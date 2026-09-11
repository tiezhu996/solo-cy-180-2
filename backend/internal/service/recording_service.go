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

// RecordingService 录音片段业务接口。
type RecordingService interface {
	Create(actor *model.User, req *dto.CreateRecordingRequest) (*model.Recording, error)
	Get(actor *model.User, id uint) (*model.Recording, error)
	// List 同时服务「按项目」与「按问题」两个接口，复用同一 service 方法。
	List(actor *model.User, projectID, questionID uint) ([]model.Recording, error)
	Update(actor *model.User, id uint, req *dto.UpdateRecordingRequest) (*model.Recording, error)
	UpdateSummary(actor *model.User, id uint, summary string) (*model.Recording, error)
	AttachAudio(actor *model.User, id uint, audioKey string, duration int) (*model.Recording, error)
	Delete(actor *model.User, id uint) error
	CountByProject(projectID uint) (int64, error)
}

type recordingService struct {
	recordingRepo repository.RecordingRepository
	projectRepo   repository.ProjectRepository
	questionRepo  repository.QuestionRepository
	logger        *slog.Logger
}

// NewRecordingService 构造录音服务。
func NewRecordingService(recordingRepo repository.RecordingRepository, projectRepo repository.ProjectRepository, questionRepo repository.QuestionRepository, logger *slog.Logger) RecordingService {
	return &recordingService{recordingRepo: recordingRepo, projectRepo: projectRepo, questionRepo: questionRepo, logger: logger}
}

// loadProjectForRead 取项目并做读归属校验。
func (s *recordingService) loadProjectForRead(actor *model.User, projectID uint) (*model.Project, error) {
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
}

func (s *recordingService) Create(actor *model.User, req *dto.CreateRecordingRequest) (*model.Recording, error) {
	project, err := s.loadProjectForRead(actor, req.ProjectID)
	if err != nil {
		return nil, err
	}
	// 录音采集属于项目采集内容：管理员可写任意项目，采访员只能写自己负责的项目，档案员不能采集。
	if !canManageProjectContent(actor, project) {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "recording", "create"))
		return nil, roleWriteError(actor, "录音", "采集")
	}
	if err := ensureWritable(actor, project, "录音", "采集"); err != nil {
		return nil, err
	}
	question, err := s.questionRepo.FindByID(req.QuestionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("问题 %d 不存在", req.QuestionID), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询问题 %d 失败", req.QuestionID), err)
	}
	// 录音关联的提问必须属于同一项目，禁止把录音挂到别的项目的问题上。
	if question.ProjectID != req.ProjectID {
		s.logger.Warn(fmt.Sprintf(constants.LogCrossProject, actor.Username, req.ProjectID, question.ProjectID, "question#"+fmt.Sprint(question.ID)))
		return nil, crossProjectError(actor, req.ProjectID, question.ProjectID,
			fmt.Sprintf("问题 %d", question.ID))
	}
	recording := &model.Recording{
		ProjectID:       req.ProjectID,
		QuestionID:      req.QuestionID,
		DurationSeconds: req.DurationSeconds,
		Summary:         req.Summary,
		Status:          constants.RecordingStatusRecording,
		CreatedBy:       actor.ID,
	}
	if err := s.recordingRepo.Create(recording); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("创建问题 %d 的录音失败", req.QuestionID), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogRecordingUpload, actor.Username, recording.ProjectID, recording.QuestionID, recording.DurationSeconds, recording.Status))
	return recording, nil
}

func (s *recordingService) Get(actor *model.User, id uint) (*model.Recording, error) {
	recording, err := s.recordingRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("录音 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询录音 %d 失败", id), err)
	}
	if _, err := s.loadProjectForRead(actor, recording.ProjectID); err != nil {
		return nil, err
	}
	return recording, nil
}

func (s *recordingService) List(actor *model.User, projectID, questionID uint) ([]model.Recording, error) {
	// 按问题查询时，先解析问题所属项目以统一做归属校验。
	if projectID == 0 && questionID > 0 {
		question, err := s.questionRepo.FindByID(questionID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("问题 %d 不存在", questionID), err)
			}
			return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询问题 %d 失败", questionID), err)
		}
		projectID = question.ProjectID
	}
	if _, err := s.loadProjectForRead(actor, projectID); err != nil {
		return nil, err
	}
	var (
		recordings []model.Recording
		err        error
	)
	if projectID > 0 {
		recordings, err = s.recordingRepo.ListByProject(projectID)
	} else {
		recordings, err = s.recordingRepo.ListByQuestion(questionID)
	}
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternal, "录音列表查询失败", err)
	}
	return recordings, nil
}

func (s *recordingService) Update(actor *model.User, id uint, req *dto.UpdateRecordingRequest) (*model.Recording, error) {
	recording, err := s.recordingRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("录音 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询录音 %d 失败", id), err)
	}
	project, err := s.loadProjectForRead(actor, recording.ProjectID)
	if err != nil {
		return nil, err
	}
	// 通用更新（时长/状态）归采集内容；改摘要走专用整理权限。
	hasManage := canManageProjectContent(actor, project)
	hasCurate := canCurateProject(actor, project)
	if !hasManage && !hasCurate {
		return nil, roleWriteError(actor, "录音", "更新")
	}
	if err := ensureWritable(actor, project, "录音", "更新"); err != nil {
		return nil, err
	}
	if hasManage {
		if req.DurationSeconds > 0 {
			recording.DurationSeconds = req.DurationSeconds
		}
		if req.Status != "" {
			if !constants.ValidRecordingStatus(req.Status) {
				return nil, util.NewAppError(constants.CodeValidation, fmt.Sprintf("录音状态 %s 不合法", req.Status), nil)
			}
			if !constants.CanTransitionRecording(recording.Status, req.Status) {
				return nil, util.NewAppError(constants.CodeRecordingStatus,
					fmt.Sprintf("录音 %d 状态不允许从 %s 流转到 %s", id, recording.Status, req.Status), nil)
			}
			recording.Status = req.Status
		}
	}
	if req.Summary != "" {
		if !hasCurate {
			return nil, roleWriteError(actor, "录音摘要", "整理")
		}
		recording.Summary = req.Summary
	}
	if err := s.recordingRepo.Update(recording); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("更新录音 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogRecordingStatus, actor.Username, recording.ID, recording.Status, recording.Status))
	return recording, nil
}

func (s *recordingService) UpdateSummary(actor *model.User, id uint, summary string) (*model.Recording, error) {
	recording, err := s.recordingRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("录音 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询录音 %d 失败", id), err)
	}
	project, err := s.loadProjectForRead(actor, recording.ProjectID)
	if err != nil {
		return nil, err
	}
	// 摘要是档案整理动作：管理员、档案员可整理任意项目，采访员只能整理自己负责的项目。
	if !canCurateProject(actor, project) {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "recording", "summary"))
		return nil, roleWriteError(actor, "录音摘要", "整理")
	}
	if err := ensureWritable(actor, project, "录音摘要", "整理"); err != nil {
		return nil, err
	}
	recording.Summary = summary
	if err := s.recordingRepo.Update(recording); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("更新录音 %d 摘要失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogRecordingSummary, actor.Username, recording.ID, summary))
	return recording, nil
}

func (s *recordingService) AttachAudio(actor *model.User, id uint, audioKey string, duration int) (*model.Recording, error) {
	recording, err := s.recordingRepo.FindByIDForUpdate(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("录音 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询录音 %d 失败", id), err)
	}
	project, err := s.loadProjectForRead(actor, recording.ProjectID)
	if err != nil {
		return nil, err
	}
	if !canManageProjectContent(actor, project) {
		return nil, roleWriteError(actor, "录音", "上传")
	}
	if err := ensureWritable(actor, project, "录音", "上传"); err != nil {
		return nil, err
	}
	recording.AudioKey = audioKey
	if duration > 0 {
		recording.DurationSeconds = duration
	}
	if recording.Status == constants.RecordingStatusRecording || recording.Status == constants.RecordingStatusProcessing {
		recording.Status = constants.RecordingStatusReady
	}
	if err := s.recordingRepo.Update(recording); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("录音 %d 音频关联失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogRecordingUpload, actor.Username, recording.ProjectID, recording.QuestionID, recording.DurationSeconds, recording.Status))
	return recording, nil
}

func (s *recordingService) Delete(actor *model.User, id uint) error {
	recording, err := s.recordingRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(constants.CodeNotFound, fmt.Sprintf("录音 %d 不存在", id), err)
		}
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询录音 %d 失败", id), err)
	}
	project, err := s.loadProjectForRead(actor, recording.ProjectID)
	if err != nil {
		return err
	}
	if !canManageProjectContent(actor, project) {
		return roleWriteError(actor, "录音", "删除")
	}
	if err := ensureWritable(actor, project, "录音", "删除"); err != nil {
		return err
	}
	if err := s.recordingRepo.Delete(id); err != nil {
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("删除录音 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogRecordingDelete, actor.Username, id))
	return nil
}

func (s *recordingService) CountByProject(projectID uint) (int64, error) {
	return s.recordingRepo.CountByProject(projectID)
}
