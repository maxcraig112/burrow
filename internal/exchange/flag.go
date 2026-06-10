package exchange

import "os"

const (
	EnvNameplate = "NAMEPLATE" // when set, every Send returns this fixed nameplate
)

// Flags holds runtime configuration sourced from environment variables.
type Flags struct {
	FixedNameplate string // EnvNameplate — empty means generate randomly
}

// FlagsFromEnv reads all exchange Flags from the current environment.
func FlagsFromEnv() Flags {
	return Flags{
		FixedNameplate: os.Getenv(EnvNameplate),
	}
}
