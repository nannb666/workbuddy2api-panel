package usage

import (
	"testing"
	"time"
)

// 仪表盘 RPM/TPM：最近 5 分钟总量 ÷ 5；失败尝试计 RPM，TPM 只计 prompt+completion，
// 不含 cache token；窗口外样本排除。
func TestPerformance5m(t *testing.T) {
	r := New("")
	now := time.Now()
	d := Delta{PromptTokens: 100, HasPromptTokens: true, CompletionTokens: 50, HasCompletion: true,
		CacheHitTokens: 99999, HasCacheTokens: true}
	for i := 0; i < 4; i++ {
		r.Add(now, "cn", "u1", "m", d, true)
	}
	r.Add(now, "cn", "u1", "m", Delta{}, false) // 失败也计 RPM
	r.Add(now.Add(-4*time.Minute), "cn", "u1", "m",
		Delta{PromptTokens: 100, HasPromptTokens: true}, true)
	r.Add(now.Add(-6*time.Minute), "cn", "u1", "m",
		Delta{PromptTokens: 1000, HasPromptTokens: true}, true) // 窗口外，忽略

	rpm, tpm := r.Performance5m()
	if rpm != 1 || tpm != 140 {
		t.Fatalf("rpm/tpm=%d/%d want 1/140（6 请求/5；tokens=(4*150+100)/5）", rpm, tpm)
	}
	if rpm, tpm := New("").Performance5m(); rpm != 0 || tpm != 0 {
		t.Fatalf("empty rpm/tpm=%d/%d want 0/0", rpm, tpm)
	}
}
