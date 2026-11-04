package codex

// LaunchTemporaryRun starts a profile in an isolated runtime home instead of
// mutating the live ~/.codex config: the profile is snapshotted into a fresh
// CODEX_HOME, cleaned up on exit, and handed to the preferred terminal.
//
// preferredTerminal is empty to let the launcher pick the platform default.
// The GUI's LaunchCodex only contributes that preference (it reads
// preferences.json, which is the shell's business) — keeping everything else
// here is what lets `dg` reuse the same launch path later.
func LaunchTemporaryRun(id string, cwd *string, preferredTerminal string) error {
	CleanupStaleRuntimeHomes()
	profile, err := GetProfile(id)
	if err != nil {
		return err
	}
	plan, err := BuildTemporaryRunPlan(profile)
	if err != nil {
		return err
	}
	spec := LaunchSpec{
		program:    plan.program,
		args:       plan.args,
		env:        plan.env,
		unsetEnv:   plan.unsetEnv,
		cwd:        cwd,
		supportDir: plan.runtimeHome,
	}
	return LaunchInTerminal(spec, preferredTerminal)
}
