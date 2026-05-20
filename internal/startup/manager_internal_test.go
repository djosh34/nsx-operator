package startup

import (
	"testing"

	nsxv1alpha "github.com/djosh34/nsx-operator/api/v1alpha"
	"github.com/djosh34/nsx-operator/internal/config"
	"github.com/djosh34/nsx-operator/internal/nsxclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWriteControlForCloudAppliesGlobalOverrideAndPerCloudDisable(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name        string
		nsxConfig   config.NSXConfig
		cloud       nsxv1alpha.NSXNetworkCloud
		wantEnabled bool
		wantReason  nsxclient.WriteDisabledReason
		wantFQDN    string
		wantName    string
	}{
		{
			name:        "global disabled overrides explicit cloud enabled",
			nsxConfig:   config.NSXConfig{WritesEnabled: false, WritesEnabledConfigured: true},
			cloud:       networkCloudForWriteControl("cloud-a", "NSX-A.Example.Test", &enabled),
			wantEnabled: false,
			wantReason:  nsxclient.WriteDisabledReasonGlobalConfig,
			wantFQDN:    "nsx-a.example.test",
			wantName:    "cloud-a",
		},
		{
			name:        "cloud disabled when global enabled",
			nsxConfig:   config.NSXConfig{WritesEnabled: true, WritesEnabledConfigured: true},
			cloud:       networkCloudForWriteControl("cloud-b", "NSX-B.Example.Test:8443", &disabled),
			wantEnabled: false,
			wantReason:  nsxclient.WriteDisabledReasonNetworkCloud,
			wantFQDN:    "nsx-b.example.test:8443",
			wantName:    "cloud-b",
		},
		{
			name:        "omitted cloud setting stays enabled when global enabled",
			nsxConfig:   config.NSXConfig{WritesEnabled: true, WritesEnabledConfigured: true},
			cloud:       networkCloudForWriteControl("cloud-c", "NSX-C.Example.Test", nil),
			wantEnabled: true,
			wantReason:  "",
			wantFQDN:    "nsx-c.example.test",
			wantName:    "cloud-c",
		},
	}

	for testIndex := range tests {
		tt := tests[testIndex]
		t.Run(tt.name, func(t *testing.T) {
			got := writeControlForCloud(&tt.nsxConfig, &tt.cloud)
			if got.Enabled != tt.wantEnabled {
				t.Fatalf("Enabled = %t, want %t", got.Enabled, tt.wantEnabled)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.NetworkCloudFQDN != tt.wantFQDN {
				t.Fatalf("NetworkCloudFQDN = %q, want %q", got.NetworkCloudFQDN, tt.wantFQDN)
			}
			if got.NetworkCloudName != tt.wantName {
				t.Fatalf("NetworkCloudName = %q, want %q", got.NetworkCloudName, tt.wantName)
			}
		})
	}
}

func networkCloudForWriteControl(name string, fqdn string, writesEnabled *bool) nsxv1alpha.NSXNetworkCloud {
	return nsxv1alpha.NSXNetworkCloud{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: nsxv1alpha.NSXNetworkCloudSpec{
			NetworkCloudFQDN: fqdn,
			NetworkCloudID:   name,
			Name:             name,
			WritesEnabled:    writesEnabled,
		},
	}
}
