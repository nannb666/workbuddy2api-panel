package panel

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// submitImport 提交一次多文件异步导入，返回提交响应码/内容。
func submitImport(e *upgEnv, files map[string][]byte) (int, string) {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, raw := range files {
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			e.t.Fatal(err)
		}
		if _, err := fw.Write(raw); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		e.t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/panel/api/import/cockpit", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-key")
	rr := httptest.NewRecorder()
	e.panel.ServeHTTP(rr, req)
	return rr.Code, rr.Body.String()
}

func statusImport(e *upgEnv) map[string]any {
	e.t.Helper()
	req := httptest.NewRequest("GET", "/panel/api/import/status", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rr := httptest.NewRecorder()
	e.panel.ServeHTTP(rr, req)
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		e.t.Fatalf("status parse: %v %s", err, rr.Body)
	}
	return m
}

func cockpitDoc(uid string) []byte {
	doc := map[string]any{
		"uid": uid, "access_token": "at-" + uid, "refresh_token": "rt-" + uid,
		"expires_at": 1893456000000, "domain": "www.codebuddy.cn", "nickname": uid,
	}
	raw, _ := json.Marshal([]any{doc})
	return raw
}

// 提交即返回 202 + total；进度在任务完成后 finished==total，成功/跳过可核对。
func TestImportJobProgress(t *testing.T) {
	e := upgNewEnv(t)
	for _, uid := range []string{"u1", "u2", "u3"} {
		e.fake.register(uid, 100)
	}
	docs := make([]any, 0, 3)
	for _, uid := range []string{"u1", "u2", "u3"} {
		docs = append(docs, map[string]any{
			"uid": uid, "access_token": "at-" + uid, "refresh_token": "rt-" + uid,
			"expires_at": 1893456000000, "domain": "www.codebuddy.cn",
		})
	}
	raw, _ := json.Marshal(docs)
	code, body := submitImport(e, map[string][]byte{"batch.json": raw})
	if code != http.StatusAccepted || !strings.Contains(body, `"total":3`) {
		t.Fatalf("submit -> %d %s", code, body)
	}
	// 进度查询在新页面/刷新后依旧能读到同一任务状态。
	snap := statusImport(e)
	if snap["job_id"] == "" || snap["total"] != float64(3) {
		t.Fatalf("status snapshot=%v", snap)
	}
	final := waitImportDone(t, e.panel)
	var f struct {
		Imported int `json:"imported"`
		Skipped  int `json:"skipped"`
		Finished int `json:"finished"`
		Total    int `json:"total"`
	}
	if err := json.Unmarshal([]byte(final), &f); err != nil {
		t.Fatal(err)
	}
	if f.Finished != 3 || f.Total != 3 || f.Imported != 3 || f.Skipped != 0 {
		t.Fatalf("final=%+v want 3/3/3/0", f)
	}
}

// 批内重复 UID + 池里已存在 UID 都跳过，不覆盖凭证、不重复签到/查余额。
func TestImportJobDeduplicates(t *testing.T) {
	e := upgNewEnv(t, upgSeed{uid: "existing", credits: 50})
	e.fake.register("fresh", 100)
	docs := []any{
		map[string]any{"uid": "existing", "access_token": "NEW", "refresh_token": "NEW", "domain": "www.codebuddy.cn"},
		map[string]any{"uid": "fresh", "access_token": "at-fresh", "refresh_token": "rt-fresh", "domain": "www.codebuddy.cn"},
		map[string]any{"uid": "fresh", "access_token": "at-fresh2", "refresh_token": "rt-fresh2", "domain": "www.codebuddy.cn"},
	}
	raw, _ := json.Marshal(docs)
	code, body := submitImport(e, map[string][]byte{"dup.json": raw})
	if code != 202 {
		t.Fatalf("submit -> %d %s", code, body)
	}
	final := waitImportDone(t, e.panel)
	var f struct {
		Imported int      `json:"imported"`
		Skipped  int      `json:"skipped"`
		Errors   []string `json:"errors"`
	}
	_ = json.Unmarshal([]byte(final), &f)
	if f.Imported != 1 || f.Skipped != 2 {
		t.Fatalf("dedup final=%+v want imported=1 skipped=2", f)
	}
	// existing 的凭证没被覆盖。
	if a := e.pool.AuthByUID("existing"); a == nil || a.AccessTokenValue() != "at-existing" {
		t.Fatalf("existing credential overwritten: %+v", a)
	}
	// fresh 只入一次，凭证是第一条（at-fresh），不是第二条覆盖。
	if a := e.pool.AuthByUID("fresh"); a == nil || a.AccessTokenValue() != "at-fresh" {
		t.Fatalf("fresh credential wrong: %+v", a)
	}
}

// 单任务锁：未完成时再次提交 409；状态接口无任务时返回空 job_id。
func TestImportJobSingleFlight(t *testing.T) {
	// 真实 panel：手动清空全局任务后状态返回空 job_id；随后构造运行中任务验证 409。
	// 全局任务完成后仍保留快照（供刷新恢复），故必须先手动清空。
	e := upgNewEnv(t)
	prev := panelImport.current
	t.Cleanup(func() { panelImport.current = prev })
	panelImport.current = nil
	empty := statusImport(e)
	if empty["job_id"] != "" || empty["running"] != false {
		t.Fatalf("empty status=%v", empty)
	}
	panelImport.current = &importJob{ID: "test-running", Running: true, Total: 9, Finished: 4}
	snap := statusImport(e)
	if snap["job_id"] != "test-running" || snap["running"] != true || snap["finished"] != float64(4) {
		t.Fatalf("running snapshot=%v", snap)
	}
	code, _ := submitImport(e, map[string][]byte{"x.json": cockpitDoc("x")})
	if code != http.StatusConflict {
		t.Fatalf("second import while running -> %d want 409", code)
	}
}
