package panel

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/usage"
)

// overview 透出可用号并发占用：当前在途 / 生效上限之和；
// 冷却/暂停/禁用号不计入。
func TestOverviewConcurrencyStats(t *testing.T) {
	e := upgNewEnv(t, upgSeed{uid: "u1", credits: 10}, upgSeed{uid: "u2", credits: 10})
	e.pool.SetDefaultConcurrency(2)
	if !e.pool.Acquire("u1") {
		t.Fatal("acquire u1")
	}
	read := func() (int, int) {
		req := httptest.NewRequest("GET", "/panel/api/overview", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		rr := httptest.NewRecorder()
		e.panel.ServeHTTP(rr, req)
		var ov struct {
			Used  int `json:"concurrency_used"`
			Limit int `json:"concurrency_limit"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &ov); err != nil {
			t.Fatalf("overview parse: %v %s", err, rr.Body)
		}
		return ov.Used, ov.Limit
	}
	if used, limit := read(); used != 1 || limit != 4 {
		t.Fatalf("used/limit=%d/%d want 1/4", used, limit)
	}
	e.pool.Pause("u2")
	if used, limit := read(); used != 1 || limit != 2 {
		t.Fatalf("after pause used/limit=%d/%d want 1/2", used, limit)
	}
}

// overview 透出 RPM/TPM（sub2api 口径）：最近 5 分钟平均每分钟请求数/Token 数。
func TestOverviewRPMTPM(t *testing.T) {
	e := upgNewEnv(t, upgSeed{uid: "u1", credits: 10})
	rec := usage.New("")
	now := time.Now()
	for i := 0; i < 5; i++ {
		rec.Add(now, "cn", "u1", "m", usage.Delta{
			PromptTokens: 50, HasPromptTokens: true,
			CompletionTokens: 100, HasCompletion: true,
		}, true)
	}
	e.panel.cfg.Usage = rec

	req := httptest.NewRequest("GET", "/panel/api/overview", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rr := httptest.NewRecorder()
	e.panel.ServeHTTP(rr, req)
	var ov struct {
		RPM int64 `json:"rpm"`
		TPM int64 `json:"tpm"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &ov); err != nil {
		t.Fatalf("overview parse: %v %s", err, rr.Body)
	}
	if ov.RPM != 1 || ov.TPM != 150 {
		t.Fatalf("rpm/tpm=%d/%d want 1/150", ov.RPM, ov.TPM)
	}
}
