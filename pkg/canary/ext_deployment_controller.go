package canary

import (
	"context"
	"fmt"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ExtDeploymentController struct {
	*DeploymentController
}

func (rdc *ExtDeploymentController) Initialize(canary *flaggerv1.Canary) (err error) {
	ctx := context.Background()
	if internal.HasSourceTargetRef(canary) {
		primaryName := canary.Spec.SourceRef.Name
		primary, err := rdc.kubeClient.AppsV1().Deployments(canary.Namespace).Get(ctx, primaryName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment %s.%s get query error: %w", primaryName, canary.Namespace, err)
		}

		if canary.Status.Phase == "" || canary.Status.Phase == flaggerv1.CanaryPhaseInitializing {
			if !canary.SkipAnalysis() {
				if _, err := IsDeploymentReady(primary, canary.GetProgressDeadlineSeconds()); err != nil {
					return fmt.Errorf("IsPrimaryReady failed: %w", err)
				}
			}
		}

		// In there is no sourceRef, run default logic, Initialized -> Progressing need check canary changed.
		// If there is sourceRef, we think canary already changed, directly do Initialized -> Progressing translate.
		if canary.Status.Phase == flaggerv1.CanaryPhaseInitialized {
			if err := rdc.SyncStatus(canary, flaggerv1.CanaryStatus{Phase: flaggerv1.CanaryPhaseProgressing}); err != nil {
				return err
			}
		}

		return nil
	} else {
		return rdc.DeploymentController.Initialize(canary)
	}
}

func (rdc *ExtDeploymentController) IsPrimaryReady(canary *flaggerv1.Canary) error {
	ctx := context.Background()
	if internal.HasSourceTargetRef(canary) {
		primaryName := canary.Spec.SourceRef.Name
		primary, err := rdc.kubeClient.AppsV1().Deployments(canary.Namespace).Get(ctx, primaryName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment %s.%s get query error: %w", primaryName, canary.Namespace, err)
		}

		_, err = IsDeploymentReady(primary, canary.GetProgressDeadlineSeconds())
		return err
	} else {
		return rdc.DeploymentController.IsPrimaryReady(canary)
	}
}

func (rdc *ExtDeploymentController) Promote(canary *flaggerv1.Canary) error {
	ctx := context.Background()
	if !internal.IsExtentOn(canary) {
		return rdc.DeploymentController.Promote(canary)
	}
	if canary.Status.CanaryWeight != 100 {
		if err := rdc.SetStatusWeight(canary, 100); err != nil {
			return err
		}
		if new, err := rdc.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(ctx, canary.Name, metav1.GetOptions{}); err != nil {
			return fmt.Errorf("canary %s.%s get query failed: %w", canary.Name, canary.Namespace, err)
		} else {
			new.DeepCopyInto(canary)
		}
	}
	return nil
}

func (rdc *ExtDeploymentController) ScaleToZero(canary *flaggerv1.Canary) error {
	if internal.IsExtentOn(canary) {
		// don't scale down canary
		return nil
	} else {
		return rdc.DeploymentController.ScaleToZero(canary)
	}
}

func (rdc *ExtDeploymentController) ScaleFromZero(canary *flaggerv1.Canary) error {
	if internal.IsExtentOn(canary) {
		return nil
	} else {
		return rdc.DeploymentController.ScaleFromZero(canary)
	}
}
