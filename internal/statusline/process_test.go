package statusline

import (
	"os"
	"testing"
)

func TestProcessIsAlive(t *testing.T) {
	if !processIsAlive(uint32(os.Getpid())) {
		t.Errorf("processIsAlive(own pid %d) = false, want true", os.Getpid())
	}
	if processIsAlive(0) {
		t.Error("processIsAlive(0) = true, want false")
	}
}
