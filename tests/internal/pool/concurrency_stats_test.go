package pool

import (
	"testing"
	"time"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
)

// 并发统计只算「可用号」：未禁用、未暂停、未冷却/熔断/降权；
// 分子当前在途，分母生效并发上限之和（含已满号）。
func TestAvailableConcurrencyOnlyHealthy(t *testing.T) {
	p := New("")
	defer p.Close()
	auth.SetGlobalEnabled(true)
	t.Cleanup(func() { auth.SetGlobalEnabled(true) })
	for _, a := range []*auth.Auth{
		{UID: "cn1"},
		{UID: "cn2"},
		{UID: "cn3"},
		{UID: "cn4"},
		{UID: "g1", Domain: "www.workbuddy.ai"},
	} {
		p.Add(a)
	}
	p.SetDefaultConcurrency(2)

	// cn1 占满 2 个名额，g1 占 1 个：used=3、limit=2+2=4（cn2 冷却后不计）。
	if !p.Acquire("cn1") || !p.Acquire("cn1") || !p.Acquire("g1") {
		t.Fatal("acquire failed")
	}
	p.Cooldown("cn2", CoolSoft, time.Hour, "429")
	p.Pause("cn3")
	p.Disable("cn4", "test")
	used, limit := p.AvailableConcurrency()
	if used != 3 || limit != 4 {
		t.Fatalf("used/limit=%d/%d want 3/4（冷却/暂停/禁用不计）", used, limit)
	}

	// 单号并发覆盖实时计入分母；已占用的 2 个名额不因上限变化而消失。
	p.SetAccountConcurrency("cn1", 5)
	used, limit = p.AvailableConcurrency()
	if used != 3 || limit != 7 {
		t.Fatalf("after override used/limit=%d/%d want 3/7", used, limit)
	}
}
