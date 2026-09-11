# 口述历史采集工具（OralHistory）

一个全栈的口述历史采集工具：登录后创建采访项目，填写受访者姓名、出生年份与背景简介；采访过程中支持录音并自动关联到对应问题；结束后在时间轴上标注关键节点，并为每段录音撰写一句话摘要；项目页按时间线展示全部采访片段，可点击播放录音并查看摘要。

## 快速启动（Docker Compose，推荐）

```bash
cd /path/to/cy-180
docker compose up -d
```

启动完成后：

| 服务 | 地址 |
| --- | --- |
| 前端 | http://localhost:8180 |
| 后端 API | http://localhost:9180 |
| 健康检查 | http://localhost:9180/healthz |
| OpenAPI 文档 | backend/api/openapi.yaml（Swagger UI 可导入 https://editor.swagger.io） |
| MySQL | localhost:10180 |

> Redis 与 MinIO 仅在 Compose 内网中供后端使用，默认不映射到宿主机；三个对外端口固定为 **前端 8180 / 后端 9180 / 数据库 10180**。

默认管理员账号：`admin / admin123456`（应用首次启动自动创建）。**公开注册只能得到「采访员」角色**，「档案员 / 管理员」只能由管理员在「用户管理」页（`PUT /api/v1/users/:id/role`）调整。

停止并清理数据：

```bash
docker compose down -v --remove-orphans
```

## 主要功能

- 用户注册/登录（JWT 认证 + RBAC 角色：管理员 / 采访员 / 档案员）
- 采访项目管理：创建、编辑、状态流转（草稿 → 进行中 → 已完成 → 已归档）、删除
- 采访问题管理：为项目添加问题清单，作为录音提纲
- 录音管理：浏览器端录音 → 上传 MinIO → 自动关联到对应问题 → 一句话摘要
- 时间轴：按项目/录音标注关键节点，项目页按时间线展示所有片段并支持播放
- 操作审计日志（仅管理员）、全局错误处理与请求追踪（request_id）

## 角色与权限模型（RBAC + 数据归属 + 归档写保护）

- **公开注册**：任何人都可注册，但注册结果恒为「采访员」，请求体中即使伪造 `role=admin` 也会被忽略；只有管理员能调用 `PUT /users/:id/role` 改角色。
- **管理员 admin**：全局管理。可查看/操作任意项目、问题、录音、节点，可管理用户角色、查看审计日志。
- **采访员 interviewer**：只能处理**自己负责**（`projects.created_by = 自己`）的项目及其问题、录音、节点；看不到、改不到他人项目。
- **档案员 archivist**：可查看任意项目、整理**录音摘要**与**时间轴节点**、可归档项目；但**不能**创建/修改/删除项目资料、不能添加问题、不能采集/删除录音。
- **同项目约束**：录音关联的提问必须属于同一项目，时间轴节点必须对应同项目的录音，违反返回 `409 / 40906 跨项目写入`。
- **归档写保护**：项目进入 `archived` 后为只读终态，任何角色（含管理员）都不能再写入问题/录音/摘要/节点/状态/删除，违反返回 `409 / 40905`；读取与播放不受影响。

| 操作 | 管理员 | 采访员（本人项目） | 采访员（他人项目） | 档案员 |
| --- | --- | --- | --- | --- |
| 改用户角色 / 用户管理 / 审计日志 | ✅ | ❌ 403 | ❌ 403 | ❌ 403 |
| 创建 / 改 / 删项目、加问题、采集删除录音 | ✅ | ✅ | ❌ 403 | ❌ 403 |
| 查看项目 / 问题 / 录音 | ✅ 全部 | ✅ 本人 | ❌ 403 | ✅ 全部 |
| 整理录音摘要、标注时间轴节点 | ✅ | ✅ 本人 | ❌ 403 | ✅ 全部 |
| 归档项目 | ✅ | ✅ 本人 | ❌ 403 | ✅ |
| 归档后任何写入 | ❌ 409 | ❌ 409 | ❌ 409 | ❌ 409 |

## 技术栈

| 层 | 技术栈 |
| --- | --- |
| 前端 | 原前端框架保持不变（React 18 + Vite + TypeScript + Zustand + React Router + axios） |
| 后端 | Go 1.22 + Gin + GORM |
| 数据库 | MySQL 8.0 |
| 对象存储 | MinIO（录音文件） |
| 缓存/限流 | Redis |
| 认证 | JWT + RBAC |
| 日志 | log/slog（结构化，含 request_id） |
| 参数校验 | github.com/go-playground/validator/v10 |

## 目录结构

```
cy-180/
├── backend/
│   ├── cmd/server/main.go          # 入口：加载配置、装配依赖、启动服务
│   ├── internal/
│   │   ├── config/                 # 环境变量配置解析
│   │   ├── constants/              # 错误码/日志模板/文案/角色/项目状态/录音状态
│   │   ├── model/                  # 每个实体一个文件（user/project/question/recording/timeline_marker/audit_log）
│   │   ├── dto/                    # 每个实体一个 DTO 文件
│   │   ├── repository/             # 每个实体一个仓储文件（含哨兵错误）
│   │   ├── service/                # 每个实体一个服务文件 + 对象存储服务 + access.go（三角色/归属/归档鉴权）
│   │   ├── handler/                # 每个实体一个处理器文件
│   │   ├── router/                 # 每个实体一个路由注册文件
│   │   ├── middleware/             # auth/rbac/request_id/error_handler/recovery/rate_limit/audit/cors/request_log
│   │   └── util/                   # logger/jwt/app_error/formatters/password/response
│   ├── migrations/001_init.sql     # MySQL 初始化基线脚本
│   ├── api/openapi.yaml            # OpenAPI 3.0 接口文档
│   ├── Dockerfile                  # Go 多阶段构建
│   ├── go.mod / go.sum
├── frontend/
│   ├── src/
│   │   ├── api/                    # 每个实体一个 API 文件（auth/project/question/recording/timelineMarker/audit/user）
│   │   ├── components/             # StatusBadge/EmptyState/ConfirmDialog/DataTable/AudioPlayer/ProjectForm/Layout
│   │   ├── pages/                  # login/projects/interview/audit/users 页面目录
│   │   ├── stores/                 # 按实体拆分（auth/project/question/recording/timeline）
│   │   ├── hooks/                  # useAuth/usePagination
│   │   ├── utils/                  # request.ts（拦截器）/format.ts/permission.ts（前端权限规则）
│   │   ├── constants/              # 与后端对应的枚举
│   │   └── router/                 # 路由守卫
│   ├── Dockerfile                  # 前端多阶段构建 + Nginx 托管
│   └── nginx.conf                  # 前端路由 + /api 反向代理
├── database/init.sql               # 数据库初始化脚本
├── scripts/selftest.sh             # 三角色越权 / 跨项目 / 归档写保护 / 主流程接口自测
├── docker-compose.yml
├── .env / .env.example
└── README.md
```

## 本地开发（备选）

后端：

```bash
cd backend
go mod tidy
go run ./cmd/server        # 需要先准备 MySQL/Redis/MinIO 环境变量
go build ./...
```

前端：

```bash
cd frontend
npm install
npm run dev                # 默认 http://localhost:5173，/api 代理到 http://localhost:9180
```

## 环境变量说明

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| COMPOSE_PROJECT_NAME | oralhistory | 容器名前缀与网络命名 |
| FRONTEND_PORT | 8180 | 前端宿主机端口 |
| BACKEND_PORT | 9180 | 后端宿主机端口 |
| DB_PORT | 10180 | MySQL 宿主机端口 |
| DB_NAME / DB_USER / DB_PASSWORD | oralhistory_db / oralhistory_user / oralhistory_pwd | MySQL 配置 |
| DB_ROOT_PASSWORD | oralhistory_root_pwd | MySQL root 密码（healthcheck 使用） |
| JWT_SECRET | change_me_to_a_long_random_string | JWT 签名密钥（生产务必修改） |
| JWT_EXPIRE_HOURS | 72 | JWT 有效期（小时） |
| RUN_MODE | release | 运行模式 |
| MINIO_ACCESS_KEY / MINIO_SECRET_KEY / MINIO_BUCKET | minioadmin / minioadmin123 / oralhistory-audio | MinIO 配置（仅内网） |

## API 清单

统一前缀 `/api/v1`，统一响应 `{ "code": 0, "message": "ok", "data": ... }`，分页参数 `page` / `page_size`。

| 方法 | 路径 | 说明 | 权限 |
| --- | --- | --- | --- |
| GET | /healthz | 健康检查 | 公开 |
| POST | /api/v1/auth/register | 注册 | 公开 |
| POST | /api/v1/auth/login | 登录 | 公开 |
| GET | /api/v1/auth/me | 当前用户 | 登录 |
| GET | /api/v1/users | 用户列表 | 管理员 |
| PUT | /api/v1/users/:id/role | 更新用户角色 | 管理员 |
| DELETE | /api/v1/users/:id | 删除用户 | 管理员 |
| GET | /api/v1/projects | 项目列表（status 筛选；采访员仅本人） | 登录 |
| POST | /api/v1/projects | 创建项目 | 管理员/采访员 |
| GET | /api/v1/projects/mine | 我的项目 | 登录 |
| GET | /api/v1/projects/stats | 项目统计 | 登录 |
| GET | /api/v1/projects/:id | 项目详情（采访员仅本人） | 登录 |
| PUT | /api/v1/projects/:id | 更新项目资料 | 管理员/负责人采访员 |
| PUT | /api/v1/projects/:id/status | 项目状态流转（归档：管理员/档案员/负责人） | 登录 |
| DELETE | /api/v1/projects/:id | 删除项目 | 管理员/负责人采访员 |
| GET | /api/v1/projects/:id/questions | 问题列表 | 登录（采访员仅本人） |
| POST | /api/v1/projects/:id/questions | 添加问题 | 管理员/负责人采访员 |
| PUT | /api/v1/questions/:id | 更新问题 | 管理员/负责人采访员 |
| DELETE | /api/v1/questions/:id | 删除问题 | 管理员/负责人采访员 |
| GET | /api/v1/recordings?project_id= 或 ?question_id= | 录音列表（复用 RecordingService.List） | 登录（采访员仅本人） |
| POST | /api/v1/recordings | 创建录音（问题须同项目） | 管理员/负责人采访员 |
| GET | /api/v1/recordings/:id | 录音详情 | 登录（采访员仅本人） |
| PUT | /api/v1/recordings/:id | 更新录音 | 管理员/负责人采访员 |
| PUT | /api/v1/recordings/:id/summary | 更新一句话摘要 | 管理员/档案员/负责人采访员 |
| POST | /api/v1/recordings/:id/audio | 上传录音（multipart） | 管理员/负责人采访员 |
| GET | /api/v1/recordings/:id/audio | 播放音频流 | 登录（采访员仅本人） |
| DELETE | /api/v1/recordings/:id | 删除录音 | 管理员/负责人采访员 |
| GET | /api/v1/timeline-markers?project_id= 或 ?recording_id= | 时间轴节点（复用 TimelineMarkerService.List） | 登录（采访员仅本人） |
| POST | /api/v1/timeline-markers | 标注节点（录音须同项目） | 管理员/档案员/负责人采访员 |
| PUT | /api/v1/timeline-markers/:id | 更新节点 | 管理员/档案员/负责人采访员 |
| DELETE | /api/v1/timeline-markers/:id | 删除节点 | 管理员/档案员/负责人采访员 |
| GET | /api/v1/audit-logs | 审计日志 | 管理员 |

> 上述所有写接口在项目归档后统一返回 `409 / 40905`；跨项目挂接（录音引用别项目问题、节点引用别项目录音）返回 `409 / 40906`；越权访问他人项目返回 `403 / 40300`。

复用关系说明：`GET /api/v1/recordings?project_id=` 与 `GET /api/v1/recordings?question_id=` 复用 `RecordingService.List`；`GET /api/v1/timeline-markers?project_id=` 与 `GET /api/v1/timeline-markers?recording_id=` 复用 `TimelineMarkerService.List`；前端 `ProjectForm` 组件在项目列表页与项目详情页复用，`StatusBadge` / `AudioPlayer` 在多个页面复用。

## curl 调用示例

```bash
# 登录获取 token
curl -sS -X POST http://localhost:9180/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123456"}'
# => {"code":0,"message":"登录成功","data":{"token":"<TOKEN>","user":{...}}}

TOKEN='<上一步返回的 token>'

# 创建采访项目
curl -sS -X POST http://localhost:9180/api/v1/projects \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"老城记忆口述史","interviewee_name":"王奶奶","birth_year":1938,"background":"纺织厂退休工人"}'

# 项目列表
curl -sS "http://localhost:9180/api/v1/projects?page=1&page_size=10" -H "Authorization: Bearer $TOKEN"

# 添加采访问题
curl -sS -X POST http://localhost:9180/api/v1/projects/1/questions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"请您回忆一下童年时住过的老房子","sort_order":0}'

# 创建录音记录并上传音频
curl -sS -X POST http://localhost:9180/api/v1/recordings \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"project_id":1,"question_id":1,"duration_seconds":30}'
curl -sS -X POST http://localhost:9180/api/v1/recordings/1/audio \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@interview.webm" -F "duration_seconds=30"

# 撰写一句话摘要
curl -sS -X PUT http://localhost:9180/api/v1/recordings/1/summary \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"summary":"王奶奶回忆童年在胡同里捉迷藏的趣事"}'

# 标注时间轴节点
curl -sS -X POST http://localhost:9180/api/v1/timeline-markers \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"project_id":1,"recording_id":1,"timestamp_second":6,"label":"讲到胡同捉迷藏","note":"情绪激动"}'

# 项目状态流转
curl -sS -X PUT http://localhost:9180/api/v1/projects/1/status \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"in_progress"}'

# 审计日志（管理员）
curl -sS "http://localhost:9180/api/v1/audit-logs?page=1&page_size=10" -H "Authorization: Bearer $TOKEN"
```

## 枚举出现位置清单

### 1. 用户角色 Role（admin / interviewer / archivist）

后端出现位置：
- `backend/internal/constants/roles.go`（定义与校验）
- `backend/internal/model/user.go`（role 字段）
- `backend/internal/dto/user.go`（UserResponse；注册 DTO 不接受 role）
- `backend/internal/service/user_service.go`（注册强制 interviewer、UpdateRole 校验）
- `backend/internal/service/access.go`（三角色写权限矩阵：canManageProjectContent/canCurateProject/canArchiveProject）
- `backend/internal/handler/user_handler.go`（UpdateRole 的 oneof 校验）
- `backend/internal/router/user.go`、`backend/internal/router/audit.go`（RBAC 中间件使用）
- `backend/internal/middleware/rbac.go`（角色比对）
- `backend/internal/util/formatters.go`（RoleText）
- `backend/internal/constants/log_templates.go`（LogUserRegister / LogRoleDenied 等含 role 参数）
- `backend/internal/constants/error_codes.go`（CodeForbidden 等）

前端出现位置：
- `frontend/src/constants/index.ts`（ROLE_* / ROLE_TEXT / ROLE_OPTIONS）
- `frontend/src/utils/format.ts`（roleText）
- `frontend/src/utils/permission.ts`（角色能力判定 canManageContent/canCurate/canArchive/canCreateProject）
- `frontend/src/stores/authStore.ts`（hasRole）
- `frontend/src/components/Layout.tsx`（管理员菜单显隐）
- `frontend/src/pages/audit/AuditPage.tsx`、`frontend/src/pages/users/UsersPage.tsx`（useAuth 路由守卫）
- `frontend/src/api/types.ts`（Role 类型）

### 2. 项目状态 ProjectStatus（draft / in_progress / completed / archived）

后端出现位置：
- `backend/internal/constants/project_status.go`（定义、校验、状态机流转 CanTransitionProject）
- `backend/internal/model/project.go`（status 字段）
- `backend/internal/dto/project.go`（Create/UpdateStatus 的 oneof 校验）
- `backend/internal/service/project_service.go`（状态机校验与流转）
- `backend/internal/handler/project_handler.go`（状态流转接口）
- `backend/internal/repository/project_repository.go`（status 筛选）
- `backend/internal/util/formatters.go`（ProjectStatusText）
- `backend/internal/constants/log_templates.go`（LogProjectCreate/Update/Status 含 status）
- `backend/internal/constants/error_codes.go`（CodeProjectStatus）

前端出现位置：
- `frontend/src/constants/index.ts`（PROJECT_STATUS_* / PROJECT_STATUS_TEXT / PROJECT_STATUS_OPTIONS）
- `frontend/src/utils/format.ts`（projectStatusText）
- `frontend/src/components/StatusBadge.tsx`（状态徽标样式映射）
- `frontend/src/pages/projects/ProjectListPage.tsx`（筛选、流转按钮显隐）
- `frontend/src/pages/projects/ProjectDetailPage.tsx`（状态流转按钮）
- `frontend/src/api/types.ts`（ProjectStatus 类型）

### 3. 录音状态 RecordingStatus（recording / processing / ready / failed）

后端出现位置：
- `backend/internal/constants/recording_status.go`（定义、校验、状态机流转 CanTransitionRecording）
- `backend/internal/model/recording.go`（status 字段）
- `backend/internal/dto/recording.go`（Update 的 oneof 校验）
- `backend/internal/service/recording_service.go`（状态机校验、AttachAudio 置为 ready）
- `backend/internal/handler/recording_handler.go`（更新/上传接口）
- `backend/internal/util/formatters.go`（RecordingStatusText）
- `backend/internal/constants/log_templates.go`（LogRecordingUpload/Status 含 status）
- `backend/internal/constants/error_codes.go`（CodeRecordingStatus）

前端出现位置：
- `frontend/src/constants/index.ts`（RECORDING_STATUS_* / RECORDING_STATUS_TEXT）
- `frontend/src/utils/format.ts`（recordingStatusText）
- `frontend/src/components/StatusBadge.tsx`（录音状态徽标）
- `frontend/src/pages/projects/ProjectDetailPage.tsx`（时间线状态展示）
- `frontend/src/pages/interview/InterviewPage.tsx`（录音面板状态展示）
- `frontend/src/api/types.ts`（RecordingStatus 类型）

## Docker 部署说明

- 一条命令启动全部服务（前端、后端、MySQL、Redis、MinIO）：

  ```bash
  docker compose up -d
  ```

- 端口映射：前端 `${FRONTEND_PORT:-8180}:80`，后端 `${BACKEND_PORT:-9180}:9180`（容器内后端监听 9180），数据库 `${DB_PORT:-10180}:3306`。Redis、MinIO 仅供 Compose 内网访问，不占宿主机端口。
- 数据卷：`db_data`（MySQL）、`redis_data`（Redis）、`minio_data`（MinIO）均为命名卷，`docker compose down -v` 会清空数据。
- 健康检查与启动顺序（避免“卡在等待依赖”）：
  - 探针必须使用镜像内**实际存在**的二进制。MySQL 用 `mysqladmin ping`、Redis 用 `redis-cli ping`；MinIO 官方镜像只自带 `curl`/`mc` 而**不含 `wget`**，故其探针为 `curl -fsS http://127.0.0.1:9000/minio/health/ready`（误用 `wget` 会导致容器永远不 healthy、后端一直等待）。
  - 后端镜像 `alpine:3.20`、前端镜像 `nginx:1.27-alpine` 均自带 busybox `wget`，分别用 `wget -qO- http://127.0.0.1:9180/healthz`、`wget -qO- http://127.0.0.1:80/` 探测。
  - `depends_on`：后端等待 db/redis/minio `service_healthy`；前端等待后端 `service_healthy`。后端对 MinIO 的 bucket 探测另有最多 12 次、每次 2s 的有界重试（`internal/service/storage_service.go`），即便依赖刚 healthy 仍在初始化也不会崩溃重启。
- 常见问题：
  - 中文目录名下启动：本项目未使用绑定挂载到中文路径，任意目录名均可启动；如遇权限问题请检查 Docker Desktop 的文件共享设置。
  - 后端无法连接数据库：等待 `db` 服务 healthy（`docker compose ps` 查看），后端会自动等待。
  - 若某容器一直 `health: starting`：先核对其 healthcheck 用的命令在该镜像里是否存在（典型坑：给 MinIO 用了 `wget`）。
  - 录音无法上传：请确认浏览器已授权麦克风；上传大小限制在 nginx `client_max_body_size 200m`。
  - 修改 JWT_SECRET 后需重启后端使 token 失效。

## 接口自测

`scripts/selftest.sh` 覆盖三角色越权、跨项目写入与归档写保护，并跑通主流程（注册→项目→问题→录音上传→摘要→时间轴节点→归档）。后端启动后执行：

```bash
# 直连后端
bash scripts/selftest.sh http://localhost:9180
# 或经前端 Nginx 反代
bash scripts/selftest.sh http://localhost:8180
```

预期结尾输出 `自测结果：PASS=53 FAIL=0`。后端单元测试与构建：`cd backend && go test ./... && go build ./...`。

## License

MIT
