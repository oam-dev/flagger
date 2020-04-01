package internal

import (
	"fmt"

	"github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

const OAM_CANARY_EXT_SWITCH = "oam.canary.extension.switch"

func IsRollingUpdate(canary *v1beta1.Canary) bool {
	return canary.Spec.Analysis.StepReplicas != 0
}

func IsExtentOn(canary *v1beta1.Canary) bool {
	return canary.Annotations[OAM_CANARY_EXT_SWITCH] == "true"
}

func HasSourceTargetRef(canary *v1beta1.Canary) bool {
	return canary.Spec.SourceRef.Name != ""
}

//  Whether canary promoted
func IsPromoted(canary *v1beta1.Canary) bool {
	return canary.Status.Phase == v1beta1.CanaryPhasePromoting || canary.Status.Phase == v1beta1.CanaryPhaseFinalising ||
		canary.Status.Phase == v1beta1.CanaryPhaseSucceeded
}

// Whether canary failed, controller should rollback in this condition
func IsFailed(canary *v1beta1.Canary) bool {
	return canary.Status.Phase == v1beta1.CanaryPhaseFailed || canary.Status.FailedChecks >= canary.GetAnalysisThreshold()
}

//  Whether canary finished
func IsFinished(canary *v1beta1.Canary) bool {
	return canary.Status.Phase == v1beta1.CanaryPhasePromoting || canary.Status.Phase == v1beta1.CanaryPhaseFinalising ||
		canary.Status.Phase == v1beta1.CanaryPhaseSucceeded || canary.Status.Phase == v1beta1.CanaryPhaseFailed ||
		canary.Status.Phase == v1beta1.CanaryPhaseTerminated || canary.Status.Phase == v1beta1.CanaryPhaseTerminating
}

// Whether canary failed, controller should rollback in this condition
func IsInitializing(canary *v1beta1.Canary) bool {
	return canary.Status.Phase == v1beta1.CanaryPhaseInitializing || canary.Status.Phase == ""
}

func IsInitialized(canary *v1beta1.Canary) bool {
	return canary.Status.Phase == v1beta1.CanaryPhaseInitialized
}

func GetSourceName(canary *v1beta1.Canary) string {
	var sourceName string
	if HasSourceTargetRef(canary) {
		sourceName = canary.Spec.SourceRef.Name
	} else {
		sourceName = fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)
	}
	return sourceName
}
