package tui

import (
	"context"
)

// list builds a section opener around one of the generic list sources.
func list(source func() ListSource) func(context.Context) (Screen, error) {
	return func(ctx context.Context) (Screen, error) {
		return newListScreen(ctx, source())
	}
}

// newCodexScreen is the Codex tool pane — the GUI's providers / auth-profiles
// / sessions sub-views as sections.
func newCodexScreen(ctx context.Context) (Screen, error) {
	return newSectionedScreen(ctx, "Codex", []screenSection{
		{key: "providers", title: "Providers", open: list(CodexSource)},
		{key: "auth", title: "Auth", open: list(CodexAuthSource)},
		{key: "sessions", title: "Sessions", open: list(CodexSessionsSource)},
	})
}

// newPiScreen is the Pi tool pane — providers / profiles / sessions.
func newPiScreen(ctx context.Context) (Screen, error) {
	return newSectionedScreen(ctx, "Pi", []screenSection{
		{key: "profiles", title: "Profiles", open: list(PiSource)},
		{key: "providers", title: "Providers", open: list(PiProvidersSource)},
		{key: "sessions", title: "Sessions", open: list(PiSessionsSource)},
	})
}

// newOmpScreen is the OMP tool pane — config / providers / sessions.
func newOmpScreen(ctx context.Context) (Screen, error) {
	return newSectionedScreen(ctx, "OMP", []screenSection{
		{key: "config", title: "Config", open: list(OmpSource)},
		{key: "providers", title: "Providers", open: list(OmpProvidersSource)},
		{key: "sessions", title: "Sessions", open: list(OmpSessionsSource)},
	})
}

// newChannelsScreen is the Channels pane — the GUI's detail /
// export-templates sub-views as sections.
func newChannelsScreen(ctx context.Context) (Screen, error) {
	return newSectionedScreen(ctx, "Channels", []screenSection{
		{key: "detail", title: "Channels", open: list(ChannelsSource)},
		{key: "export", title: "Export", open: list(ExportSource)},
	})
}
