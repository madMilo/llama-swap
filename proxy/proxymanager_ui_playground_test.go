package proxy

import "testing"

func TestUITruthy(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", " yes ", "On"}
	for _, value := range truthy {
		if !uiTruthy(value) {
			t.Fatalf("expected %q to be truthy", value)
		}
	}

	falsy := []string{"", "0", "false", "no", "off", "hello"}
	for _, value := range falsy {
		if uiTruthy(value) {
			t.Fatalf("expected %q to be falsy", value)
		}
	}
}

func TestUIPlaygroundMockModels(t *testing.T) {
	existing := []UIModel{{ID: "existing-model"}}
	actual := uiPlaygroundMockModels(existing)
	if len(actual) != 1 || actual[0].ID != "existing-model" {
		t.Fatalf("expected existing models to be preserved")
	}

	mocked := uiPlaygroundMockModels(nil)
	if len(mocked) < 4 {
		t.Fatalf("expected seeded mock model list, got %d", len(mocked))
	}
	if mocked[0].ID == "" {
		t.Fatalf("expected first mock model ID to be set")
	}
}

func TestUIPlaygroundURLs(t *testing.T) {
	if got := uiPlaygroundTabURL("chat", false); got != "/ui/playground?tab=chat" {
		t.Fatalf("unexpected non-mock tab URL: %s", got)
	}
	if got := uiPlaygroundTabURL("chat", true); got != "/ui/playground?tab=chat&mock=1" {
		t.Fatalf("unexpected mock tab URL: %s", got)
	}
	if got := uiPlaygroundPartialURL("images", false); got != "/ui/partials/playground/images" {
		t.Fatalf("unexpected non-mock partial URL: %s", got)
	}
	if got := uiPlaygroundPartialURL("images", true); got != "/ui/partials/playground/images?mock=1" {
		t.Fatalf("unexpected mock partial URL: %s", got)
	}
}
