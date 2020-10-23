package canary

import (
	"context"
	"fmt"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RollingUpdateDeploymentController struct {
	*DeploymentController
}

func (rdc *RollingUpdateDeploymentController) Initialize(canary *flaggerv1.Canary) (err error) {
	if internal.IsRollingUpdate(canary) {
		primaryName := canary.Spec.SourceRef.Name
		primary, err := rdc.kubeClient.AppsV1().Deployments(canary.Namespace).Get(context.Background(), primaryName, metav1.GetOptions{})
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

func (rdc *RollingUpdateDeploymentController) IsPrimaryReady(canary *flaggerv1.Canary) error {
	if internal.IsRollingUpdate(canary) {
		primaryName := canary.Spec.SourceRef.Name
		primary, err := rdc.kubeClient.AppsV1().Deployments(canary.Namespace).Get(context.Background(), primaryName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment %s.%s get query error: %w", primaryName, canary.Namespace, err)
		}

		_, err = IsDeploymentReady(primary, canary.GetProgressDeadlineSeconds())
		return err
	} else {
		return rdc.DeploymentController.IsPrimaryReady(canary)
	}
}

func (rdc *RollingUpdateDeploymentController) Promote(canary *flaggerv1.Canary) error {
	if internal.IsRollingUpdate(canary) {
		if canary.Status.CanaryWeight != 100 {
			if err := rdc.SetStatusWeight(canary, 100); err != nil {
				return err
			}
			if new, err := rdc.flaggerClient.FlaggerV1beta1().Canaries(canary.Namespace).Get(context.Background(), canary.Name, metav1.GetOptions{}); err != nil {
				return fmt.Errorf("canary %s.%s get query failed: %w", canary.Name, canary.Namespace, err)
			} else {
				new.DeepCopyInto(canary)
			}
		}
		return nil
	} else {
		return rdc.DeploymentController.Promote(canary)
	}
}

func (rdc *RollingUpdateDeploymentController) ScaleToZero(canary *flaggerv1.Canary) error {
	if internal.IsRollingUpdate(canary) {
		// don't scale down canary
		return nil
	} else {
		return rdc.DeploymentController.ScaleToZero(canary)
	}
}

func (rdc *RollingUpdateDeploymentController) ScaleFromZero(canary *flaggerv1.Canary) error {
	if internal.IsRollingUpdate(canary) {
		return nil
	} else {
		return rdc.DeploymentController.ScaleFromZero(canary)
	}
}
