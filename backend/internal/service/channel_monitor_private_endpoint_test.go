package service

import "testing"

func TestValidateEndpointPrivateHTTPIsOptIn(t *testing.T) {
	const endpoint = "http://127.0.0.1:8080"
	if err := validateEndpoint(endpoint); err == nil {
		t.Fatal("private HTTP endpoint must be rejected by default")
	}
	if err := validateEndpoint(endpoint, true); err != nil {
		t.Fatalf("private HTTP endpoint should be allowed when enabled: %v", err)
	}
	if err := validateEndpoint("http://169.254.169.254", true); err == nil {
		t.Fatal("cloud metadata endpoint must remain blocked")
	}
}
