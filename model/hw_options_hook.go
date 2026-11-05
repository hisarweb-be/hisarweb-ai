package model

// The one place this package reaches service/: the startup seeding of our
// production defaults lives in service/hw_seed_defaults.go, because it writes
// ratios, groups and site settings through both model and the setting packages,
// and service already imports model. An import the other way round would close
// the cycle model -> service -> model.
//
// service registers the seeding through SetHwOptionHook at its own start, and
// InitOptionMap calls it once the options table has been read. Before that
// registration, and in a process that never loads service, the hook is nil and
// nothing runs.

// hwOptionHook runs the startup seeding of our option defaults. It is set once
// during startup and read on every start of the option map, so the server
// rebuilds the defaults whenever it reloads its options from the database.
var hwOptionHook func()

// SetHwOptionHook registers the startup seeding that InitOptionMap runs after it
// has read the options table. Passing nil removes it; the tests use that to keep
// a case from seeding a database it did not ask for.
func SetHwOptionHook(hook func()) {
	hwOptionHook = hook
}
