package logger

import "testing"

func TestWithFieldsMergesContextAndExtraFields(t *testing.T) {
	entry := New("gateway").WithFields(map[string]interface{}{
		"request_id": "req-1",
		"model":      "deepseek-v4-pro",
	})

	merged := mergeFields(entry.fields, map[string]interface{}{
		"status": 200,
	})

	if got, want := merged["request_id"], "req-1"; got != want {
		t.Fatalf("request_id = %v, want %v", got, want)
	}
	if got, want := merged["model"], "deepseek-v4-pro"; got != want {
		t.Fatalf("model = %v, want %v", got, want)
	}
	if got, want := merged["status"], 200; got != want {
		t.Fatalf("status = %v, want %v", got, want)
	}
}
