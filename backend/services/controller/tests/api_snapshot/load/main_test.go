package load_test

import (
	"os"
	"testing"

	"github.com/leandrofars/oktopus/tests/api_snapshot/snapshot"
)

func TestMain(m *testing.M) {
	code := m.Run()
	snapshot.Shutdown()
	os.Exit(code)
}
