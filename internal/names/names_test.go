package names_test

import (
	"strings"
	"testing"

	"github.com/djosh34/nsx-operator/internal/names"
	"k8s.io/apimachinery/pkg/util/validation"
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
			want: "nsx-a.example.net-app-foo",
		},
		{
			name: "fqdn with port",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a.example.net:8443",
				GroupID:          "app-foo",
			},
			want: "nsx-a.example.net-8443-app-foo",
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
	want := "nsx-a.example.net-8443-app-foo"

	for i := 0; i < 20; i++ {
		if got := names.NSXGroupName(id); got != want {
			t.Fatalf("NSXGroupName iteration %d = %q, want exact readable projection %q", i, got, want)
		}
	}
}

func TestNSXGroupNameMakesCompleteMetadataNameKubernetesSafe(t *testing.T) {
	id := names.NSXGroupLogicalID{
		NetworkCloudFQDN: " HTTPS://NSX_A.Example.Net:8443/policy/api/v1 ",
		GroupID:          " App/Web_GROUP ",
	}

	got := names.NSXGroupName(id)
	if got != "nsx-a.example.net-8443-app-web-group" {
		t.Fatalf("NSXGroupName(%+v) = %q, want full generated name sanitized", id, got)
	}
	if errs := validation.IsDNS1123Subdomain(got); len(errs) != 0 {
		t.Fatalf("NSXGroupName(%+v) = %q, Kubernetes validation errors = %v", id, got, errs)
	}
}

func TestNSXGroupNameHandlesBoundaryInputs(t *testing.T) {
	tests := []struct {
		name string
		id   names.NSXGroupLogicalID
		want string
	}{
		{
			name: "leading and trailing invalid characters are removed",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "://NSX-A.example.net/",
				GroupID:          "_app-web_",
			},
			want: "nsx-a.example.net-app-web",
		},
		{
			name: "repeated separators collapse",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "nsx-a...example.net",
				GroupID:          "app---web",
			},
			want: "nsx-a.example.net-app-web",
		},
		{
			name: "empty values get stable fallback",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "   ",
				GroupID:          "",
			},
			want: "nsx-group-d8156bae0c4243d3",
		},
		{
			name: "all invalid values get stable fallback",
			id: names.NSXGroupLogicalID{
				NetworkCloudFQDN: "://",
				GroupID:          "___",
			},
			want: "nsx-group-c4b5e7c67c581862",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := names.NSXGroupName(test.id)
			if got != test.want {
				t.Fatalf("NSXGroupName(%+v) = %q, want %q", test.id, got, test.want)
			}
			if errs := validation.IsDNS1123Subdomain(got); len(errs) != 0 {
				t.Fatalf("NSXGroupName(%+v) = %q, Kubernetes validation errors = %v", test.id, got, errs)
			}
		})
	}
}

func TestNSXGroupNameTruncatesLongNamesWithDeterministicCollisionResistantSuffix(t *testing.T) {
	idA := names.NSXGroupLogicalID{
		NetworkCloudFQDN: "nsx-a.example.net",
		GroupID:          strings.Repeat("shared-prefix-", 30) + "alpha",
	}
	idB := names.NSXGroupLogicalID{
		NetworkCloudFQDN: "nsx-a.example.net",
		GroupID:          strings.Repeat("shared-prefix-", 30) + "bravo",
	}

	gotA := names.NSXGroupName(idA)
	gotAAgain := names.NSXGroupName(idA)
	gotB := names.NSXGroupName(idB)

	if len(gotA) > 253 {
		t.Fatalf("NSXGroupName(%+v) length = %d, want at most 253: %q", idA, len(gotA), gotA)
	}
	if errs := validation.IsDNS1123Subdomain(gotA); len(errs) != 0 {
		t.Fatalf("NSXGroupName(%+v) = %q, Kubernetes validation errors = %v", idA, gotA, errs)
	}
	if gotA != gotAAgain {
		t.Fatalf("NSXGroupName(%+v) produced %q then %q, want deterministic value", idA, gotA, gotAAgain)
	}
	if gotA == gotB {
		t.Fatalf("NSXGroupName values collided for distinct long IDs: %q", gotA)
	}
	if !strings.HasPrefix(gotA, "nsx-a.example.net-shared-prefix") {
		t.Fatalf("NSXGroupName(%+v) = %q, want readable prefix preserved", idA, gotA)
	}
}
