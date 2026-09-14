package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wujunwei928/token-usage/internal/report"
	"github.com/wujunwei928/token-usage/internal/server"
)

// 机器改名后 OS 用户名(机器名\用户名)跟着变,ensureLocalUser 若按名字
// 劈出新用户,ingest 会在 BindDevice 处报 "device ... is bound to another
// user"。本地身份必须锚定本机 device(ADR 0012:身份存 DB、重启不丢):
// 谁拥有 device.json 里的 Device,谁就是这个 web 的本地用户。
func TestEnsureLocalUserAdoptsDeviceOwnerAfterMachineRename(t *testing.T) {
	// 隔离 device 文件:测试读这里,绝不碰开发机真实的 device.json。
	const deviceID = "d825a570b0381d82b71165a9e9babf61"
	deviceFile := filepath.Join(t.TempDir(), "device.json")
	if err := os.WriteFile(deviceFile,
		[]byte("{\"deviceId\":\""+deviceID+"\",\"label\":\"DESKTOP-0C85PN2\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKEN_USAGE_DEVICE_FILE", deviceFile)

	ctx := context.Background()
	store, err := server.OpenStore(t.TempDir() + "/web.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 改名前:旧机器名用户持有本机 device 与历史数据。
	oldID, err := store.CreateUser(ctx, `DESKTOP-0C85PN2\hyd`, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReplaceDay(ctx, oldID, deviceID, "DESKTOP-0C85PN2", "2026-09-13",
		[]server.HourRow{{Hour: 9, Tool: "claude", Model: "m", Input: 42}}); err != nil {
		t.Fatal(err)
	}

	// 改名后首次启动留下的空壳用户(真实事故现场 users 表里就有它)。
	if _, err := store.CreateUser(ctx, `KEEPMOVING\hyd`, "", "", ""); err != nil {
		t.Fatal(err)
	}

	u, err := ensureLocalUser(ctx, store, `KEEPMOVING\hyd`)
	if err != nil {
		t.Fatal(err)
	}

	// 先复现用户看到的症状:ingest 必须不再报 device 绑定冲突。
	snap := &report.Snapshot{DeviceID: deviceID, DeviceLabel: "KEEPMOVING", Date: "2026-09-14",
		Hours: []report.HourCell{{Hour: 10, Tool: "claude", Model: "m", Input: 7}}}
	if err := ingestSnapshots(ctx, store, u.ID, []*report.Snapshot{snap}); err != nil {
		t.Fatalf("ingest after machine rename: %v", err)
	}
	// 根因断言:本地用户必须是 device 属主,不是劈出来的新用户。
	if u.ID != oldID {
		t.Fatalf("ensureLocalUser forked the local user: id=%d, want device owner %d", u.ID, oldID)
	}
}
