package internal

import (
	"os"
	"strconv"
	"strings"
)

const (
	noopRoute     = "NOOP_ROUTE"
	canaryHaltUrl = "CANARY_HALT_URL"
)

func IsNoopRoute() bool {
	if e, ok := os.LookupEnv(noopRoute); ok {
		v, err := strconv.ParseBool(strings.TrimSpace(e))
		if err != nil {
			return false
		}
		return v
	}
	return false
}

func defaultCanaryHaltUrl() (string, bool) {
	return os.LookupEnv(canaryHaltUrl)
}
