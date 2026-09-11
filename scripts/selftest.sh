#!/usr/bin/env bash
# 三角色越权 / 跨项目 / 归档写保护 + 业务主流程接口自测。
# 用法：bash scripts/selftest.sh [BASE_URL]
set -u
BASE="${1:-http://127.0.0.1:9180}"
API="$BASE/api/v1"
PASS=0; FAIL=0
declare -A TOKEN

jget() {
  # 路径形如 ['data']['token']，用 argv 传入以避免 shell 引号嵌套。
  python3 -c '
import sys, json
d = json.load(sys.stdin)
for part in sys.argv[1].strip()[1:-1].split("]["):
    part = part.strip().strip(chr(39))
    d = d[int(part)] if part.isdigit() else d[part]
print(d)
' "$1" 2>/dev/null
}
jcode() { python3 -c "import sys,json;print(json.load(sys.stdin)['code'])" 2>/dev/null; }

# http METHOD PATH TOKEN BODY
http() {
  local method="$1" path="$2" tok="$3" body="${4:-}"
  local auth=()
  [ -n "$tok" ] && [ -n "${TOKEN[$tok]:-}" ] && auth=(-H "Authorization: Bearer ${TOKEN[$tok]}")
  if [ -n "$body" ]; then
    curl -s -o /tmp/resp.json -w '%{http_code}' -X "$method" "$API$path" \
      "${auth[@]}" -H 'Content-Type: application/json' -d "$body"
  else
    curl -s -o /tmp/resp.json -w '%{http_code}' -X "$method" "$API$path" "${auth[@]}"
  fi
}

# check DESC EXPECTED_HTTP METHOD PATH TOKEN [BODY] [EXPECT_BODY_SUBSTR]
check() {
  local desc="$1" want="$2" method="$3" path="$4" tok="$5" body="${6:-}" sub="${7:-}"
  local code
  code="$(http "$method" "$path" "$tok" "$body")"
  local bcode; bcode="$(jcode < /tmp/resp.json)"
  if [ "$code" == "$want" ]; then
    if [ -n "$sub" ] && ! grep -qF "$sub" /tmp/resp.json; then
      echo "FAIL  $desc | http=$code 期望含 '$sub'"; cat /tmp/resp.json; echo; FAIL=$((FAIL+1))
    else
      echo "PASS  $desc (http=$code code=$bcode)"; PASS=$((PASS+1))
    fi
  else
    echo "FAIL  $desc | http=$code 期望 $want"; cat /tmp/resp.json; echo; FAIL=$((FAIL+1))
  fi
}

login() { # name user pass
  local code t
  code="$(curl -s -o /tmp/login.json -w '%{http_code}' -X POST "$API/auth/login" -H 'Content-Type: application/json' -d "{\"username\":\"$2\",\"password\":\"$3\"}")"
  t="$(jget "['data']['token']" < /tmp/login.json)"
  TOKEN[$1]="$t"
  echo "-- login $2 http=$code token_len=${#t}"
}

echo "================ 0. 健康检查 & 未认证 ================"
curl -s -o /dev/null -w 'healthz http=%{http_code}\n' "$BASE/healthz"
check "未带令牌访问项目列表被拒" 401 GET /projects any

echo "================ 1. 公开注册恒为采访员 ================"
TS=$(date +%s)
IV1="iv_$TS"; IV2="iv2_$TS"; ARC="arc_$TS"
check "公开注册 iv1" 200 POST /auth/register "" "{\"username\":\"$IV1\",\"password\":\"secret123\",\"display_name\":\"采访员甲\"}"
check "公开注册 iv2" 200 POST /auth/register "" "{\"username\":\"$IV2\",\"password\":\"secret123\",\"display_name\":\"采访员乙\"}"
check "公开注册 arc" 200 POST /auth/register "" "{\"username\":\"$ARC\",\"password\":\"secret123\",\"display_name\":\"档案员丙\"}"
# 即使请求体伪造 role=admin 也无效
curl -s -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"username\":\"evil_$TS\",\"password\":\"secret123\",\"display_name\":\"伪造者\",\"role\":\"admin\"}" -o /tmp/resp.json
ROLE=$(python3 -c "import sys,json;print(json.load(sys.stdin)['data']['role'])" < /tmp/resp.json)
if [ "$ROLE" == "interviewer" ]; then echo "PASS  伪造 role=admin 注册仍为 interviewer"; PASS=$((PASS+1)); else echo "FAIL 伪造角色得到 $ROLE"; FAIL=$((FAIL+1)); fi

login admin admin admin123456
login iv1 "$IV1" secret123
login iv2 "$IV2" secret123
login arc "$ARC" secret123

# 档案员角色只能由管理员赋予
ARCID=$(curl -s "$API/users?page=1&page_size=100" -H "Authorization: Bearer ${TOKEN[admin]}" \
  | python3 -c "import sys,json;[print(u['id']) for u in json.load(sys.stdin)['data']['list'] if u['username']=='$ARC']")
check "采访员无权改角色(403)" 403 PUT "/users/$ARCID/role" iv1 '{"role":"archivist"}'
check "非管理员无权列出用户(403)" 403 GET /users iv1
check "管理员把 arc 升为档案员" 200 PUT "/users/$ARCID/role" admin '{"role":"archivist"}'
login arc "$ARC" secret123

echo "================ 2. 主流程：项目→问题→录音摘要→节点 ================"
check "采访员甲创建项目 P1" 200 POST /projects iv1 \
  "{\"title\":\"渡江战役口述-$TS\",\"interviewee_name\":\"张爷爷\",\"birth_year\":1931,\"background\":\"原华东野战军战士\"}"
check "档案员不能创建项目(403)" 403 POST /projects arc \
  '{"title":"不该建的项目","interviewee_name":"某人","birth_year":1930}'

P1=$(jget "['data']['id']" < /tmp/resp.json 2>/dev/null)
# 上一条是 403，重新取 P1：用我的项目列表
P1=$(curl -s "$API/projects?page=1&page_size=5" -H "Authorization: Bearer ${TOKEN[iv1]}" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['list'][0]['id'])")
echo "P1=$P1"

check "项目进入进行中(draft->in_progress)" 200 PUT "/projects/$P1/status" iv1 '{"status":"in_progress"}'
check "采访员甲向 P1 添加问题 Q1" 200 POST "/projects/$P1/questions" iv1 '{"content":"请讲讲您参军的经过？","sort_order":0}'
Q1=$(curl -s "$API/projects/$P1/questions" -H "Authorization: Bearer ${TOKEN[iv1]}" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['list'][0]['id'])")
echo "Q1=$Q1"

check "采访员甲创建录音 R1（同项目问题）" 200 POST /recordings iv1 \
  "{\"project_id\":$P1,\"question_id\":$Q1,\"duration_seconds\":12}"
R1=$(curl -s "$API/recordings?project_id=$P1" -H "Authorization: Bearer ${TOKEN[iv1]}" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['list'][0]['id'])")
echo "R1=$R1"

# 上传一段假音频
head -c 2048 /dev/urandom > /tmp/audio.webm
AUD_HTTP=$(curl -s -o /tmp/resp.json -w '%{http_code}' -X POST "$API/recordings/$R1/audio" \
  -H "Authorization: Bearer ${TOKEN[iv1]}" -F "file=@/tmp/audio.webm;type=audio/webm" -F "duration_seconds=12")
[ "$AUD_HTTP" == "200" ] && { echo "PASS  录音文件上传并关联 (http=200)"; PASS=$((PASS+1)); } || { echo "FAIL 音频上传 http=$AUD_HTTP"; cat /tmp/resp.json; FAIL=$((FAIL+1)); }
ASTATUS=$(python3 -c "import sys,json;print(json.load(sys.stdin)['data']['status'])" < /tmp/resp.json)
[ "$ASTATUS" == "ready" ] && { echo "PASS  上传后录音状态变为 ready"; PASS=$((PASS+1)); } || { echo "FAIL 录音状态=$ASTATUS"; FAIL=$((FAIL+1)); }

check "档案员整理录音摘要" 200 PUT "/recordings/$R1/summary" arc '{"summary":"张爷爷回忆了连夜渡江的紧张时刻。"}'
check "采访员甲标注时间轴节点 M1(timestamp=0)" 200 POST /timeline-markers iv1 \
  "{\"project_id\":$P1,\"recording_id\":$R1,\"timestamp_second\":0,\"label\":\"开始回忆渡江\"}"
check "档案员标注时间轴节点 M2" 200 POST /timeline-markers arc \
  "{\"project_id\":$P1,\"recording_id\":$R1,\"timestamp_second\":6,\"label\":\"讲到登岸\"}"
check "时间线查询返回 2 个节点" 200 GET "/timeline-markers?project_id=$P1" arc

echo "================ 3. 越权：采访员只能处理自己负责的项目 ================"
check "采访员乙不能读甲的项目(403)" 403 GET "/projects/$P1" iv2
check "采访员乙看不到甲的项目(列表为空)" 200 GET "/projects?page=1&page_size=20" iv2
N=$(python3 -c "import sys,json;print(json.load(sys.stdin)['data']['total'])" < /tmp/resp.json)
[ "$N" == "0" ] && { echo "PASS  采访员乙项目列表不含他人项目 total=0"; PASS=$((PASS+1)); } || { echo "FAIL 乙看到 total=$N"; FAIL=$((FAIL+1)); }
check "采访员乙不能改甲的项目资料(403)" 403 PUT "/projects/$P1" iv2 '{"title":"被篡改"}'
check "采访员乙不能给甲的项目加问题(403)" 403 POST "/projects/$P1/questions" iv2 '{"content":"黑"}'
check "采访员乙不能读甲的项目问题(403)" 403 GET "/projects/$P1/questions" iv2
check "采访员乙不能在甲的项目建录音(403)" 403 POST /recordings iv2 \
  "{\"project_id\":$P1,\"question_id\":$Q1,\"duration_seconds\":1}"
check "采访员乙不能看甲的录音列表(403)" 403 GET "/recordings?project_id=$P1" iv2
check "采访员乙不能整理甲的录音摘要(403)" 403 PUT "/recordings/$R1/summary" iv2 '{"summary":"x"}'
check "采访员乙不能在甲的项目标节点(403)" 403 POST /timeline-markers iv2 \
  "{\"project_id\":$P1,\"recording_id\":$R1,\"timestamp_second\":1,\"label\":\"x\"}"

echo "================ 4. 档案员：可整理摘要/节点，不能改项目资料/采集 ================"
check "档案员可查看任意项目" 200 GET "/projects/$P1" arc
check "档案员不能改项目资料(403)" 403 PUT "/projects/$P1" arc '{"title":"档案员改名"}'
check "档案员不能添加问题(403)" 403 POST "/projects/$P1/questions" arc '{"content":"多管闲事"}'
check "档案员不能采集录音(403)" 403 POST /recordings arc \
  "{\"project_id\":$P1,\"question_id\":$Q1,\"duration_seconds\":3}"
check "档案员不能删除项目(403)" 403 DELETE "/projects/$P1" arc

echo "================ 5. 跨项目写入被拒绝 ================"
# 乙建自己的项目 P2 与问题 Q2
check "采访员乙创建自己的项目 P2" 200 POST /projects iv2 \
  "{\"title\":\"乙的项目-$TS\",\"interviewee_name\":\"李奶奶\",\"birth_year\":1940}"
P2=$(jget "['data']['id']" < /tmp/resp.json)
check "P2 进入进行中" 200 PUT "/projects/$P2/status" iv2 '{"status":"in_progress"}'
check "乙向 P2 添加问题 Q2" 200 POST "/projects/$P2/questions" iv2 '{"content":"您小时候住哪？"}'
Q2=$(curl -s "$API/projects/$P2/questions" -H "Authorization: Bearer ${TOKEN[iv2]}" | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['list'][0]['id'])")
echo "P2=$P2 Q2=$Q2"

# 甲把 P2 的问题挂到 P1 的录音上 -> 409 跨项目
check "录音问题必须同项目(甲用乙的Q2建录音->409)" 409 POST /recordings iv1 \
  "{\"project_id\":$P1,\"question_id\":$Q2,\"duration_seconds\":1}"
# 乙在自己项目 P2 里引用甲项目 P1 的录音 R1 建节点 -> 409
check "节点必须对应同项目录音(乙引用甲R1->409)" 409 POST /timeline-markers iv2 \
  "{\"project_id\":$P2,\"recording_id\":$R1,\"timestamp_second\":1,\"label\":\"串项目\"}"

echo "================ 6. 管理员全局管理 ================"
check "管理员可查看全部项目(>=2)" 200 GET "/projects?page=1&page_size=50" admin
TOT=$(python3 -c "import sys,json;print(json.load(sys.stdin)['data']['total'])" < /tmp/resp.json)
[ "$TOT" -ge 2 ] && { echo "PASS  管理员项目总数=$TOT (含所有人项目)"; PASS=$((PASS+1)); } || { echo "FAIL 管理员 total=$TOT"; FAIL=$((FAIL+1)); }
check "管理员可改任意项目资料" 200 PUT "/projects/$P2" admin '{"background":"管理员补充背景"}'
check "管理员可查看审计日志" 200 GET "/audit-logs?page=1&page_size=5" admin

echo "================ 7. 归档后禁止继续写入 ================"
check "项目完成(in_progress->completed)" 200 PUT "/projects/$P1/status" iv1 '{"status":"completed"}'
check "采访员不能归档自己项目之外的流转：乙归档甲项目(403)" 403 PUT "/projects/$P1/status" iv2 '{"status":"archived"}'
check "档案员归档 P1(completed->archived)" 200 PUT "/projects/$P1/status" arc '{"status":"archived"}'
check "归档后采访员不能改摘要(409)" 409 PUT "/recordings/$R1/summary" iv1 '{"summary":"归档后改"}'
check "归档后档案员不能标节点(409)" 409 POST /timeline-markers arc \
  "{\"project_id\":$P1,\"recording_id\":$R1,\"timestamp_second\":9,\"label\":\"归档后补标\"}"
check "归档后采访员不能加问题(409)" 409 POST "/projects/$P1/questions" iv1 '{"content":"归档后加问题"}'
check "归档后不能删除项目(409)" 409 DELETE "/projects/$P1" admin
check "归档后不能再做状态流转(409)" 409 PUT "/projects/$P1/status" admin '{"status":"in_progress"}'
check "归档后仍可读项目(200)" 200 GET "/projects/$P1" arc
check "归档后仍可播放音频(200)" 200 GET "/recordings/$R1/audio" iv1

echo
echo "================ 自测结果：PASS=$PASS FAIL=$FAIL ================"
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
