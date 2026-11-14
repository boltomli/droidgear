package preferences

import "testing"

// UpdateChannel is the shared seam with internal/service/updater: unset means
// preview (the pipeline's x.y.z.w builds), and only an explicit "stable"
// opts out.
func TestUpdateChannel(t *testing.T) {
	stable := UpdateChannelStable
	preview := UpdateChannelPreview
	bogus := "beta"

	cases := []struct {
		name string
		pref *string
		want string
	}{
		{name: "unset defaults to preview", pref: nil, want: UpdateChannelPreview},
		{name: "explicit preview", pref: &preview, want: UpdateChannelPreview},
		{name: "explicit stable", pref: &stable, want: UpdateChannelStable},
		{name: "unknown value is not a freeze", pref: &bogus, want: UpdateChannelPreview},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AppPreferences{UpdatesChannel: tc.pref}.UpdateChannel()
			if got != tc.want {
				t.Fatalf("UpdateChannel() = %q, want %q", got, tc.want)
			}
		})
	}
}
