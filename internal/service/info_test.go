package service

import "testing"

func TestGetApplicationInfo(t *testing.T) {
	info := GetApplicationInfo()

	if info.Name != ApplicationName {
		t.Fatalf(
			"expected name %q, got %q",
			ApplicationName,
			info.Name,
		)
	}

	if info.Version != Version {
		t.Fatalf(
			"expected version %q, got %q",
			Version,
			info.Version,
		)
	}
}
