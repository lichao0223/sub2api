package service

import "testing"

func TestQoderSupportsUpstreamBillingProbeConfiguration(t *testing.T) {
	if !IsUpstreamBillingProbeIdentity(PlatformQoder, AccountTypeAPIKey) {
		t.Fatal("Qoder API key accounts should accept the default billing probe setting")
	}
	if !upstreamBillingProbeTargetIsOfficialAPI("https://api.qoder.com.cn/api/v1/cloud") {
		t.Fatal("Qoder official API must be treated as an official upstream")
	}
}
