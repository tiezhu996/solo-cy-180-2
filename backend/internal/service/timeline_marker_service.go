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

// TimelineMarkerService 时间轴节点业务接口。
type TimelineMarkerService interface {
	Create(actor *model.User, req *dto.CreateTimelineMarkerRequest) (*model.TimelineMarker, error)
	// List 同时服务「按项目」与「按录音」两个接口，复用同一 service 方法。
	List(actor *model.User, projectID, recordingID uint) ([]model.TimelineMarker, error)
	Update(actor *model.User, id uint, req *dto.UpdateTimelineMarkerRequest) (*model.TimelineMarker, error)
	Delete(actor *model.User, id uint) error
}

type timelineMarkerService struct {
	markerRepo    repository.TimelineMarkerRepository
	projectRepo   repository.ProjectRepository
	recordingRepo repository.RecordingRepository
	logger        *slog.Logger
}

// NewTimelineMarkerService 构造时间轴节点服务。
func NewTimelineMarkerService(markerRepo repository.TimelineMarkerRepository, projectRepo repository.ProjectRepository, recordingRepo repository.RecordingRepository, logger *slog.Logger) TimelineMarkerService {
	return &timelineMarkerService{markerRepo: markerRepo, projectRepo: projectRepo, recordingRepo: recordingRepo, logger: logger}
}

// loadProjectForRead 取项目并做读归属校验。
func (s *timelineMarkerService) loadProjectForRead(actor *model.User, projectID uint) (*model.Project, error) {
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

func (s *timelineMarkerService) Create(actor *model.User, req *dto.CreateTimelineMarkerRequest) (*model.TimelineMarker, error) {
	project, err := s.loadProjectForRead(actor, req.ProjectID)
	if err != nil {
		return nil, err
	}
	// 节点是档案整理动作：管理员、档案员可整理任意项目，采访员只能整理自己负责的项目。
	if !canCurateProject(actor, project) {
		s.logger.Warn(fmt.Sprintf(constants.LogRoleDenied, actor.Username, actor.Role, "timeline_marker", "create"))
		return nil, roleWriteError(actor, "时间轴节点", "标注")
	}
	if err := ensureWritable(actor, project, "时间轴节点", "标注"); err != nil {
		return nil, err
	}
	recording, err := s.recordingRepo.FindByID(req.RecordingID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("录音 %d 不存在", req.RecordingID), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询录音 %d 失败", req.RecordingID), err)
	}
	// 节点必须对应同项目的录音，禁止把节点挂到别的项目的录音上。
	if recording.ProjectID != req.ProjectID {
		s.logger.Warn(fmt.Sprintf(constants.LogCrossProject, actor.Username, req.ProjectID, recording.ProjectID, "recording#"+fmt.Sprint(recording.ID)))
		return nil, crossProjectError(actor, req.ProjectID, recording.ProjectID,
			fmt.Sprintf("录音 %d", recording.ID))
	}
	marker := &model.TimelineMarker{
		ProjectID:       req.ProjectID,
		RecordingID:     req.RecordingID,
		TimestampSecond: req.TimestampSecond,
		Label:           req.Label,
		Note:            req.Note,
		CreatedBy:       actor.ID,
	}
	if err := s.markerRepo.Create(marker); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("标注录音 %d 时间轴节点失败", req.RecordingID), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogMarkerCreate, actor.Username, marker.ProjectID, marker.RecordingID, marker.TimestampSecond, marker.Label))
	return marker, nil
}

func (s *timelineMarkerService) List(actor *model.User, projectID, recordingID uint) ([]model.TimelineMarker, error) {
	// 按录音查询时，先解析录音所属项目以统一做归属校验。
	if projectID == 0 && recordingID > 0 {
		recording, err := s.recordingRepo.FindByID(recordingID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("录音 %d 不存在", recordingID), err)
			}
			return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询录音 %d 失败", recordingID), err)
		}
		projectID = recording.ProjectID
	}
	if _, err := s.loadProjectForRead(actor, projectID); err != nil {
		return nil, err
	}
	var (
		markers []model.TimelineMarker
		err     error
	)
	if projectID > 0 {
		markers, err = s.markerRepo.ListByProject(projectID)
	} else {
		markers, err = s.markerRepo.ListByRecording(recordingID)
	}
	if err != nil {
		return nil, util.NewAppError(constants.CodeInternal, "时间轴节点查询失败", err)
	}
	return markers, nil
}

func (s *timelineMarkerService) Update(actor *model.User, id uint, req *dto.UpdateTimelineMarkerRequest) (*model.TimelineMarker, error) {
	marker, err := s.markerRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, util.NewAppError(constants.CodeNotFound, fmt.Sprintf("时间轴节点 %d 不存在", id), err)
		}
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询时间轴节点 %d 失败", id), err)
	}
	project, err := s.loadProjectForRead(actor, marker.ProjectID)
	if err != nil {
		return nil, err
	}
	if !canCurateProject(actor, project) {
		return nil, roleWriteError(actor, "时间轴节点", "修改")
	}
	if err := ensureWritable(actor, project, "时间轴节点", "修改"); err != nil {
		return nil, err
	}
	if req.TimestampSecond != 0 {
		marker.TimestampSecond = req.TimestampSecond
	}
	if req.Label != "" {
		marker.Label = req.Label
	}
	if req.Note != "" {
		marker.Note = req.Note
	}
	if err := s.markerRepo.Update(marker); err != nil {
		return nil, util.NewAppError(constants.CodeInternal, fmt.Sprintf("更新时间轴节点 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogMarkerUpdate, actor.Username, marker.ID, marker.Label))
	return marker, nil
}

func (s *timelineMarkerService) Delete(actor *model.User, id uint) error {
	marker, err := s.markerRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return util.NewAppError(constants.CodeNotFound, fmt.Sprintf("时间轴节点 %d 不存在", id), err)
		}
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("查询时间轴节点 %d 失败", id), err)
	}
	project, err := s.loadProjectForRead(actor, marker.ProjectID)
	if err != nil {
		return err
	}
	if !canCurateProject(actor, project) {
		return roleWriteError(actor, "时间轴节点", "删除")
	}
	if err := ensureWritable(actor, project, "时间轴节点", "删除"); err != nil {
		return err
	}
	if err := s.markerRepo.Delete(id); err != nil {
		return util.NewAppError(constants.CodeInternal, fmt.Sprintf("删除时间轴节点 %d 失败", id), err)
	}
	s.logger.Info(fmt.Sprintf(constants.LogMarkerDelete, actor.Username, id))
	return nil
}
