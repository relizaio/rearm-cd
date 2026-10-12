package cli

import (
	"strings"
	"testing"
)

// A chart is OCI unless its address asks for a classic repository (an explicit
// http(s):// scheme or a Harbor /chartrepo/ path): every deployment comes from a
// BOM component whose media type is the OCI Helm chart config. The previous
// hostname allowlist sent any unlisted registry to the classic branch, which runs
// `helm repo add` + index.yaml and gets a 403 from an OCI-only registry.
func TestHelmOciDetection(t *testing.T) {
	cases := []struct {
		name       string
		artUri     string
		wantUseOci bool
		wantOciUri string
	}{
		{
			name:       "rearmhq registry is OCI",
			artUri:     "registry.rearmhq.com/card-shuffle/mafia",
			wantUseOci: true,
			wantOciUri: "oci://registry.rearmhq.com/card-shuffle/mafia",
		},
		{
			name:       "relizahub registry is OCI",
			artUri:     "registry.test.relizahub.com/a98e122c-public/mafia",
			wantUseOci: true,
			wantOciUri: "oci://registry.test.relizahub.com/a98e122c-public/mafia",
		},
		{
			name:       "explicit oci scheme wins",
			artUri:     "oci://registry.example.com/charts/mafia",
			wantUseOci: true,
			wantOciUri: "oci://registry.example.com/charts/mafia",
		},
		{
			name:       "harbor chartrepo path stays a classic repo",
			artUri:     "registry.rearmhq.com/chartrepo/library/mafia",
			wantUseOci: false,
		},
		{
			name:       "any other registry is OCI too, no allowlist",
			artUri:     "charts.example.com/team/mafia",
			wantUseOci: true,
			wantOciUri: "oci://charts.example.com/team/mafia",
		},
		{
			name:       "rearmhq dtrack5 chart (scully-rearm-cd 403) is OCI",
			artUri:     "registry.rearmhq.com/library/dtrack5",
			wantUseOci: true,
			wantOciUri: "oci://registry.rearmhq.com/library/dtrack5",
		},
		{
			name:       "explicit https scheme stays a classic repo",
			artUri:     "https://charts.example.com/mafia",
			wantUseOci: false,
		},
		{
			name:       "explicit http scheme stays a classic repo",
			artUri:     "http://charts.example.com/mafia",
			wantUseOci: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rd := &RearmDeployment{ArtUri: tc.artUri}
			got := GetHelmRepoInfoFromDeployment(rd)
			if got.UseOci != tc.wantUseOci {
				t.Fatalf("UseOci = %v, want %v (artUri %q)", got.UseOci, tc.wantUseOci, tc.artUri)
			}
			if tc.wantUseOci && got.OciUri != tc.wantOciUri {
				t.Errorf("OciUri = %q, want %q", got.OciUri, tc.wantOciUri)
			}
			if !tc.wantUseOci && !strings.HasPrefix(got.RepoUri, "https://") && !strings.HasPrefix(got.RepoUri, "http://") {
				t.Errorf("classic repo should keep or get an http(s):// scheme, got %q", got.RepoUri)
			}
		})
	}
}
