package cli

import (
	"testing"
)

// OCI detection is a hostname allowlist, so every registry domain we publish
// charts to has to be listed. A miss is not a clean failure: the chart falls
// through to the classic branch, which runs `helm repo add` + index.yaml and
// gets a 403 from an OCI-only registry.
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
			name:       "unknown host stays a classic repo",
			artUri:     "charts.example.com/mafia",
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
			if !tc.wantUseOci && got.RepoUri[:8] != "https://" {
				t.Errorf("classic repo should get an https:// prefix, got %q", got.RepoUri)
			}
		})
	}
}
