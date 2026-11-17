package panel

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestAccountUIHelpers 用 node + DOM 桩执行 app.js，验证账号池排序键与导入进度
// 等纯函数可用（app.js 内部 let/const 可通过同一 vm 脚本访问）。
// 无 node 环境跳过（与 TestAppJSTopLevelSmoke 同口径）。
func TestAccountUIHelpers(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; JS helper test skipped")
	}
	harness := `const fs = require('fs');
const vm = require('vm');
const src = fs.readFileSync(process.argv[2], 'utf8');
const inert = new Proxy(function () {}, {
  get(t, k) { if (k === Symbol.toPrimitive) return () => ''; return inert; },
  set() { return true; }, apply() { return inert; }, construct() { return inert; }, has() { return true; },
});
const store = {};
const sandbox = new Proxy({
  location: { hash: '#accounts' }, history: { replaceState() {} },
  localStorage: { getItem: k => store[k] === undefined ? null : store[k], setItem: (k, v) => { store[k] = String(v); } },
  navigator: { clipboard: { writeText: () => Promise.resolve() } },
  document: { querySelectorAll: () => [], querySelector: () => inert, getElementById: () => inert, addEventListener() {}, documentElement: inert, head: inert, body: inert, createElement: () => inert, cookie: '' },
  fetch: () => new Promise(() => {}),
  addEventListener() {}, removeEventListener() {},
  matchMedia: () => ({ matches: false, addEventListener() {} }),
  MutationObserver: class { constructor() {} observe() {} disconnect() {} takeRecords() { return []; } },
  setInterval: () => 0, clearInterval: () => {}, setTimeout: () => 0, clearTimeout: () => {},
  console, JSON, Math, Date, Number, String, Boolean, Object, Array, Promise, Map, Set, RegExp, Error, TypeError, isNaN, parseInt, parseFloat, encodeURIComponent, decodeURIComponent, URL, Symbol, Proxy, Reflect,
}, { get(t, k) { return t[k]; }, has() { return true; } });
sandbox.window = sandbox; sandbox.globalThis = sandbox;
vm.createContext(sandbox);
const checks = ` + "`" + `
// 1) 排序键覆盖账号表可排序列；2) 导入进度文案 8/99；3) 关键函数存在。
const keys = Object.keys(ACC_SORT_KEYS);
const want = ['name','credits','success','priority','last_success'];
if (want.some(k => !keys.includes(k))) { throw new Error('missing sort keys: ' + keys.join(',')); }
if ('group' in ACC_SORT_KEYS) { throw new Error('group sort key should be removed'); }
accFilter = 'healthy';
if (typeof visibleAccount !== 'undefined') { throw new Error('visibleAccount should be removed with groups'); }
const txt = importProgressText({ total: 99, finished: 8, imported: 7, skipped: 1, current: 'a.json / u1' });
if (!txt.includes('8/99') || !txt.includes('已导入 7')) { throw new Error('progress text: ' + txt); }
if (typeof renderImportProgress !== 'function' || typeof applyModelLocksPosition !== 'function') { throw new Error('missing helpers'); }
'HELPERS OK';
` + "`" + `;
const result = vm.runInContext(src + '\n' + checks, sandbox, { filename: 'app.js' });
if (result !== 'HELPERS OK') { console.log('FAIL: ' + result); throw new Error(String(result)); }
console.log('HELPERS OK');
`
	hf, err := os.CreateTemp(t.TempDir(), "helpers-*.cjs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hf.WriteString(harness); err != nil {
		t.Fatal(err)
	}
	_ = hf.Close()
	out, err := exec.Command(node, hf.Name(), "app.js").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "HELPERS OK") {
		t.Fatalf("account UI helpers: %v\n%s", err, out)
	}
}
