package pi

import (
	"testing"
)

func TestPathsPiOfficialSessionDirEnv(t *testing.T) {
	// pi 官方通过 PI_CODING_AGENT_SESSION_DIR 切换会话目录，adapter 必须识别。
	dir := t.TempDir()
	t.Setenv(PiSessionDirEnv, dir)
	t.Setenv(PiAgentDirEnv, "")

	got, err := Paths(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != dir {
		t.Fatalf("Paths() = %v, want [%s]", got, dir)
	}
}

func TestPathsTokenUsageOverrideWins(t *testing.T) {
	// token-usage 自己的 PI_AGENT_DIR 是显式覆盖，优先于 pi 官方 env。
	override := t.TempDir()
	official := t.TempDir()
	t.Setenv(PiAgentDirEnv, override)
	t.Setenv(PiSessionDirEnv, official)

	got, err := Paths(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != override {
		t.Fatalf("Paths() = %v, want [%s]", got, override)
	}
}
