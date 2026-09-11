package service

import (
	"log/slog"
	"testing"

	"github.com/oralhistory/oralhistory/internal/constants"
	"github.com/oralhistory/oralhistory/internal/dto"
	"github.com/oralhistory/oralhistory/internal/model"
	"github.com/oralhistory/oralhistory/internal/repository"
	"github.com/oralhistory/oralhistory/internal/util"
)

type fakeProjectRepo struct {
	projects  map[uint]*model.Project
	updated   *model.Project
	forUpdate bool
	err       error
}

func (f *fakeProjectRepo) Create(project *model.Project) error {
	if f.err != nil {
		return f.err
	}
	f.projects[project.ID] = project
	return nil
}
func (f *fakeProjectRepo) FindByID(id uint) (*model.Project, error) {
	if p, ok := f.projects[id]; ok {
		return p, nil
	}
	return nil, repository.ErrNotFound
}
func (f *fakeProjectRepo) FindByIDForUpdate(id uint) (*model.Project, error) {
	f.forUpdate = true
	return f.FindByID(id)
}
func (f *fakeProjectRepo) List(page, pageSize int, status string, creatorID uint) ([]model.Project, int64, error) {
	return nil, 0, nil
}
func (f *fakeProjectRepo) ListByUser(userID uint, page, pageSize int) ([]model.Project, int64, error) {
	return nil, 0, nil
}
func (f *fakeProjectRepo) Update(project *model.Project) error {
	if f.err != nil {
		return f.err
	}
	f.updated = project
	f.projects[project.ID] = project
	return nil
}
func (f *fakeProjectRepo) UpdateStatus(project *model.Project) error {
	return f.Update(project)
}
func (f *fakeProjectRepo) Delete(id uint) error                         { return nil }
func (f *fakeProjectRepo) Count() (int64, error)                        { return 0, nil }
func (f *fakeProjectRepo) CountByCreator(creatorID uint) (int64, error) { return 0, nil }

func TestProjectServiceTransitionStatus(t *testing.T) {
	cases := []struct {
		name      string
		from      string
		to        string
		actorRole string
		actorID   uint
		wantErr   bool
	}{
		{name: "draft to in_progress", from: constants.ProjectStatusDraft, to: constants.ProjectStatusInProgress, actorRole: constants.RoleInterviewer, actorID: 1},
		{name: "in_progress to completed", from: constants.ProjectStatusInProgress, to: constants.ProjectStatusCompleted, actorRole: constants.RoleInterviewer, actorID: 1},
		{name: "completed to archived", from: constants.ProjectStatusCompleted, to: constants.ProjectStatusArchived, actorRole: constants.RoleArchivist, actorID: 3},
		{name: "draft to completed is invalid", from: constants.ProjectStatusDraft, to: constants.ProjectStatusCompleted, actorRole: constants.RoleInterviewer, actorID: 1, wantErr: true},
		{name: "archived cannot change", from: constants.ProjectStatusArchived, to: constants.ProjectStatusDraft, actorRole: constants.RoleAdmin, actorID: 9, wantErr: true},
		{name: "unknown status rejected", from: constants.ProjectStatusDraft, to: "unknown", actorRole: constants.RoleInterviewer, actorID: 1, wantErr: true},
		{name: "other interviewer cannot transition", from: constants.ProjectStatusDraft, to: constants.ProjectStatusInProgress, actorRole: constants.RoleInterviewer, actorID: 2, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeProjectRepo{projects: map[uint]*model.Project{
				1: {ID: 1, Title: "测试项目", Status: tc.from, CreatedBy: 1},
			}}
			svc := NewProjectService(repo, slog.Default())
			caseActor := &model.User{ID: tc.actorID, Username: "actor", Role: tc.actorRole}
			got, err := svc.TransitionStatus(caseActor, 1, tc.to)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				var appErr *util.AppError
				if !asAppError(err, &appErr) {
					t.Fatalf("expected app error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Status != tc.to {
				t.Fatalf("status = %s, want %s", got.Status, tc.to)
			}
			if !repo.forUpdate {
				t.Fatalf("expected SELECT FOR UPDATE path used")
			}
		})
	}
}

func TestProjectServiceCreate(t *testing.T) {
	repo := &fakeProjectRepo{projects: map[uint]*model.Project{}}
	svc := NewProjectService(repo, slog.Default())
	actor := &model.User{ID: 2, Username: "interviewer2", Role: constants.RoleInterviewer}
	req := &dto.CreateProjectRequest{
		Title:           "老城记忆",
		IntervieweeName: "王奶奶",
		BirthYear:       1938,
		Background:      "纺织厂退休工人",
	}
	project, err := svc.Create(actor, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project.Status != constants.ProjectStatusDraft {
		t.Fatalf("default status = %s, want draft", project.Status)
	}
	if project.CreatedBy != actor.ID {
		t.Fatalf("created_by = %d, want %d", project.CreatedBy, actor.ID)
	}
}

func TestProjectServiceCreateArchivistDenied(t *testing.T) {
	repo := &fakeProjectRepo{projects: map[uint]*model.Project{}}
	svc := NewProjectService(repo, slog.Default())
	actor := &model.User{ID: 3, Username: "archivist1", Role: constants.RoleArchivist}
	req := &dto.CreateProjectRequest{
		Title:           "不该创建的项目",
		IntervieweeName: "李爷爷",
		BirthYear:       1930,
	}
	if _, err := svc.Create(actor, req); err == nil {
		t.Fatalf("archivist must not create projects, got nil")
	}
}

func TestProjectServiceOwnership(t *testing.T) {
	cases := []struct {
		name      string
		role      string
		actorID   uint
		createdBy uint
		canWrite  bool
	}{
		{"owner interviewer", constants.RoleInterviewer, 1, 1, true},
		{"other interviewer denied", constants.RoleInterviewer, 2, 1, false},
		{"admin always", constants.RoleAdmin, 9, 1, true},
		{"archivist cannot edit content", constants.RoleArchivist, 3, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actor := &model.User{ID: tc.actorID, Role: tc.role}
			project := &model.Project{ID: 1, CreatedBy: tc.createdBy, Status: constants.ProjectStatusInProgress}
			if got := canManageProjectContent(actor, project); got != tc.canWrite {
				t.Fatalf("canManageProjectContent=%v want %v", got, tc.canWrite)
			}
		})
	}
}

func TestProjectArchivedWriteRejected(t *testing.T) {
	repo := &fakeProjectRepo{projects: map[uint]*model.Project{
		1: {ID: 1, Title: "已归档项目", Status: constants.ProjectStatusArchived, CreatedBy: 1},
	}}
	svc := NewProjectService(repo, slog.Default())
	owner := &model.User{ID: 1, Username: "owner", Role: constants.RoleInterviewer}
	if _, err := svc.Update(owner, 1, &dto.UpdateProjectRequest{Title: "改名"}); err == nil {
		t.Fatalf("owner update on archived project must be rejected")
	}
	if err := svc.Delete(owner, 1); err == nil {
		t.Fatalf("owner delete on archived project must be rejected")
	}
}

func asAppError(err error, target **util.AppError) bool {
	appErr, ok := err.(*util.AppError)
	if ok {
		*target = appErr
	}
	return ok
}
