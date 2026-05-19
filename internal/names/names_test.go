package names_test

import (
	"testing"

	"github.com/djosh34/nsx-operator/internal/names"
)

func TestNormalizeNetworkCloudFQDNKeepsHostIdentityStable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "trims whitespace", input: " nsx-a.example.net ", want: "nsx-a.example.net"},
		{name: "lowercases host", input: "NSX-A.Example.Net", want: "nsx-a.example.net"},
		{name: "removes trailing slash", input: "nsx-a.example.net/", want: "nsx-a.example.net"},
		{name: "uses url host", input: "https://NSX-A.Example.Net/policy/api/v1", want: "nsx-a.example.net"},
		{name: "preserves explicit port", input: "https://NSX-A.Example.Net:8443/policy/api/v1", want: "nsx-a.example.net:8443"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := names.NormalizeNetworkCloudFQDN(test.input); got != test.want {
				t.Fatalf("NormalizeNetworkCloudFQDN(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestNSXGroupNameUsesReadableStableProjection(t *testing.T) {
	tests := []struct {
		name string
		id   names.NSXGroupLogicalID
		want string
	}{
		{
			name: "plain fqdn",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net",
				GroupID:          "app-foo",
			},
			want: "nsx-a.example.net--app-foo",
		},
		{
			name: "fqdn with port",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net:8443",
				GroupID:          "app-foo",
			},
			want: "nsx-a.example.net-8443--app-foo",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := names.NSXGroupName(test.id); got != test.want {
				t.Fatalf("NSXGroupName(%+v) = %q, want %q", test.id, got, test.want)
			}
		})
	}
}

func TestNSXGroupNameIsDeterministicWithoutGeneratedSuffix(t *testing.T) {
	id := names.NSXGroupLogicalID{
		NetworkCloudFQDN: "NSX-A.Example.Net:8443",
		GroupID:          "app-foo",
	}
	want := "nsx-a.example.net-8443--app-foo"

	for i := 0; i < 20; i++ {
		if got := names.NSXGroupName(id); got != want {
			t.Fatalf("NSXGroupName iteration %d = %q, want exact readable projection %q", i, got, want)
		}
	}
}

func TestParseNSXGroupNameRoundTripsGeneratedNames(t *testing.T) {
	tests := []struct {
		name string
		id   names.NSXGroupLogicalID
		want names.NSXGroupLogicalID
	}{
		{
			name: "plain fqdn",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net",
				GroupID:          "app-foo",
			},
			want: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net",
				GroupID:          "app-foo",
			},
		},
		{
			name: "fqdn with port",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net:8443",
				GroupID:          "app-foo",
			},
			want: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net:8443",
				GroupID:          "app-foo",
			},
		},
		{
			name: "normalizes input before round trip",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: " HTTPS://NSX-A.Example.Net:8443/policy/api/v1 ",
				GroupID:          " app-web ",
			},
			want: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net:8443",
				GroupID:          "app-web",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generatedName := names.NSXGroupName(test.id)
			got, err := names.ParseNSXGroupName(generatedName)
			if err != nil {
				t.Fatalf("ParseNSXGroupName(%q) error = %v", generatedName, err)
			}
			if got != test.want {
				t.Fatalf("ParseNSXGroupName(%q) = %+v, want %+v", generatedName, got, test.want)
			}
		})
	}
}

func TestParseNSXGroupNameRejectsMalformedNames(t *testing.T) {
	tests := []string{
		"",
		"cloud-only",
		"--group",
		"cloud--",
		"cloud--group--extra",
	}

	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			if got, err := names.ParseNSXGroupName(test); err == nil {
				t.Fatalf("ParseNSXGroupName(%q) = %+v, want error", test, got)
			}
		})
	}
}
