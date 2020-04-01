package route

type SCTrafficStrategy string

const (
	SCTrafficStrategyHEADER SCTrafficStrategy = "HEADER"

	SCTrafficStrategyPARAM SCTrafficStrategy = "PARAM"

	SCTrafficStrategyCOOKIE SCTrafficStrategy = "COOKIE"
)

type SpringCloudCondition struct {
	// spring cloud traffic strategy
	Strategy  SCTrafficStrategy `json:"strategy"`

	// key in specific strategy
	Key string `json:"key"`

	Operator ConditionOperator `json:"operator"`

	// values
	Values []string `json:"values"`
}

type SpringCloudMatchRequest struct {
	// call path
	// e.g: goods/query
	Path string `json:"path"`

	// condition trigger policy
	// e.g: AND | OR
	TriggerPolicy ConditionPolicy `json:"triggerPolicy"`

	Conditions []SpringCloudCondition `json:"conditions"`
}