package daemoncore

import (
	"reflect"
	"testing"
)

func TestRunTasksExecutesInOrder(t *testing.T) {
	var calls []string

	RunTasks(
		func() { calls = append(calls, "energy") },
		nil,
		func() { calls = append(calls, "hotwater") },
		func() { calls = append(calls, "heating") },
	)

	want := []string{"energy", "hotwater", "heating"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("RunTasks() order = %v, want %v", calls, want)
	}
}
