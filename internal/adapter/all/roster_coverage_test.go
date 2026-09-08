package all

import (
	"testing"

	"github.com/wujunwei928/token-usage/internal/adapter/common"
)

// TestRosterCoversRegisteredSpecs 锁住 cline 漏统的 bug 模式:新 adapter 写了
// spec_<agent>.go 却忘了加进 common 的 rosterOrder,导致 all-report 与 web
// snapshot(都遍历 Roster())静默跳过它,而 `token-usage <agent> daily` 直连
// 命令照常出数——症状极隐蔽。有 spec 注册就必须在 roster 里。
func TestRosterCoversRegisteredSpecs(t *testing.T) {
	roster := map[string]bool{}
	for _, name := range common.Roster() {
		roster[name] = true
	}
	for agent := range registeredSpecs {
		if !roster[agent] {
			t.Errorf("agent %q has a unified spec but is missing from common rosterOrder", agent)
		}
	}
}
