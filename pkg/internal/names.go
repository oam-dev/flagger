package internal

var (
	DefaultCanaryHaltUrl = "http://localhost:9090/gate/halt"
)

func init() {
	if v, ok := defaultCanaryHaltUrl(); ok {
		DefaultCanaryHaltUrl = v
	}
}
