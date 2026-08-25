package api

import "testing"

func TestPipelineStatus_Terminal(t *testing.T) {
	terminal := []PipelineStatus{StatusSuccess, StatusFailed, StatusCanceled, StatusSkipped, StatusManual}
	for _, s := range terminal {
		if !s.Terminal() {
			t.Errorf("%q.Terminal() = false, want true", s)
		}
	}

	active := []PipelineStatus{StatusCreated, StatusWaiting, StatusPending, StatusRunning}
	for _, s := range active {
		if s.Terminal() {
			t.Errorf("%q.Terminal() = true, want false", s)
		}
	}
}
