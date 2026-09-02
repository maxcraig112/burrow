package envfile

import (
	"flag"
	"os"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

func Load(logger zerolog.Logger) {
	path := flag.String("env", ".env", "path to .env file")
	flag.Parse()
	if err := godotenv.Load(*path); err != nil && !os.IsNotExist(err) {
		logger.Warn().Str("path", *path).Err(err).Msg("could not load env file")
	}
}
