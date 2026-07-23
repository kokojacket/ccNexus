package config

import (
	"fmt"
	"testing"
)

func TestGetBasicAuthReturnsOneAtomicSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UpdateBasicAuth(true, "alice", "alice-password")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10000; i++ {
			if i%2 == 0 {
				cfg.UpdateBasicAuth(true, "alice", "alice-password")
			} else {
				cfg.UpdateBasicAuth(false, "bob", "bob-password")
			}
		}
	}()

	for i := 0; i < 10000; i++ {
		enabled, username, password := cfg.GetBasicAuth()
		state := fmt.Sprintf("%t:%s:%s", enabled, username, password)
		if state != "true:alice:alice-password" && state != "false:bob:bob-password" {
			t.Fatalf("observed mixed Basic Auth state %q", state)
		}
	}
	<-done
}
