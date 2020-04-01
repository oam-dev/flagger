package route

type ConditionPolicy string

const (
	PolicyAND ConditionPolicy = "AND"
	PolicyOR ConditionPolicy = "OR"
)

type ConditionOperator string

const (
	// =
	OperatorEqual ConditionOperator = "EQ"
	// !=
	OperatorNotEqual ConditionOperator = "NQ"
	// >
	OperatorGreatThan ConditionOperator = "GT"
	// <
	OperatorLessThan ConditionOperator = "LT"
	// >=
	OperatorGreatThanOrEqual ConditionOperator = "GE"
	// <=
	OperatorLessThanOrEqual ConditionOperator = "LE"

	// TODO: support white list, mod
)

type DubboCondition struct {
	// start from 0
	ParamIndex int32 `json:"paramIndex"`

	// parameter extract key
	// e.g:
	// - for array, [0] extract first elem
	// - for map, .get("1") extract value of key 1 from map
	// - for list, .get(1) extract second elem
	// - for object, .getName() extract method return value
	Key string `json:"key"`

	// "=", ">", "<", ">=", "<=", "!=", white list, mod
	Operator ConditionOperator `json:"operator"`

	// values
	Values []string `json:"values"`
}

type DubboMatchRequest struct {
	// service name
	// e.g: com.alibaba.edas.CanaryService
	ServiceName string `json:"serviceName"`

	// service version
	// e.g: 1.0.0
	Version string `json:"version"`

	// service group
	// e.g: canary-group
	Group string `json:"group"`

	// specific service method
	// e.g: call
	MethodName string `json:"methodName"`

	// service method param types, start from index 0
	// e.g:
	// - java.lang.String
	// - int
	// - java.lang.Integer
	// - java.lang.String[]
	// - com.alibaba.edas.User
	// - java.util.List<java.lang.String>
	// - java.util.Map<java.lang.String, java.lang.String>
	ParamTypes []string `json:"paramTypes"`

	// condition trigger policy
	// e.g: AND | OR
	TriggerPolicy ConditionPolicy `json:"triggerPolicy"`

	Conditions []DubboCondition `json:"conditions"`
}