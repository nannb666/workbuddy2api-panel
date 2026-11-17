package proxypool

import (
	"path/filepath"
	"testing"
	"time"
)

// 最近使用：固定绑定命中后记录该节点的最后选路时刻，未使用节点保持零值。
func TestProxyLastUsedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy-pool.json")
	m := testManager(t, path)
	a := addProxy(t, m, "http://127.0.0.1:4101")
	b := addProxy(t, m, "http://127.0.0.1:4102")
	m.health[a] = health{status: "healthy"}
	m.health[b] = health{status: "healthy"}
	if err := m.Bind("u1", Binding{Mode: "fixed", ProxyID: a}); err != nil {
		t.Fatal(err)
	}
	if err := m.Bind("u2", Binding{Mode: "fixed", ProxyID: b}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.selectLocked("u1", "cn"); err != nil {
		t.Fatal(err)
	}
	view := func(id string) View {
		for _, v := range m.Snapshot().Proxies {
			if v.ID == id {
				return v
			}
		}
		t.Fatalf("proxy %s not in snapshot", id)
		return View{}
	}
	first := view(a).LastUsedAt
	if first.IsZero() {
		t.Fatal("u1 选路后 a 的 last_used_at 应为非零")
	}
	if !view(b).LastUsedAt.IsZero() {
		t.Fatal("未使用的 b 不应有 last_used_at")
	}

	time.Sleep(2 * time.Millisecond)
	if _, err := m.selectLocked("u2", "cn"); err != nil {
		t.Fatal(err)
	}
	second := view(b).LastUsedAt
	if second.IsZero() {
		t.Fatal("u2 选路后 b 的 last_used_at 应为非零")
	}
	if !second.After(first) && !second.Equal(first) {
		t.Fatalf("b last_used=%v 应不早于 a last_used=%v", second, first)
	}
}
