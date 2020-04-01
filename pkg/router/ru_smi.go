package router

import (
	"context"
	"fmt"
	"math"

	"github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/internal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const hundred = 100

type RollingUpdateSmiRouter struct {
	*SmiRouter
}

func (rsr *RollingUpdateSmiRouter) Reconcile(canary *v1beta1.Canary) error {
	if internal.IsRollingUpdate(canary) {
		return nil
	} else {
		return rsr.SmiRouter.Reconcile(canary)
	}
}

func (rsr *RollingUpdateSmiRouter) SetRoutes(canary *v1beta1.Canary, primaryWeight int, canaryWeight int, mirrored bool) error {
	if internal.IsRollingUpdate(canary) {
		// canary have been promoted
		if canary.Status.Phase == v1beta1.CanaryPhasePromoting && canaryWeight == 0 && primaryWeight == hundred {
			return nil
		} else {
			primaryName := canary.Spec.SourceRef.Name
			canaryName := canary.Spec.TargetRef.Name
			maxReplicas := canary.Spec.Analysis.MaxReplicas
			canaryReplicas := int32(percent(canaryWeight, maxReplicas))
			primaryReplicas := int32(maxReplicas) - canaryReplicas

			err := rsr.updateReplicas(canary, canaryName, &canaryReplicas)
			if err != nil {
				return fmt.Errorf("set route of canary deployment %s.%s failed %w, weight: %d", canaryName, canary.Namespace, err, canaryWeight)
			}

			err = rsr.updateReplicas(canary, primaryName, &primaryReplicas)
			if err != nil {
				return fmt.Errorf("set route of primary deployment %s.%s failed %w, weight: %d", primaryName, canary.Namespace, err, primaryWeight)
			}

			if canaryReplicas != int32(maxReplicas) {
				return rsr.haltCanary(canary)
			}
		}

		return nil
	} else {
		return rsr.SmiRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored)
	}
}

func (rsr *RollingUpdateSmiRouter) haltCanary(canary *v1beta1.Canary) error {
	ca := canary.DeepCopy()
	webHooks := make([]v1beta1.CanaryWebhook, 0, len(ca.Spec.Analysis.Webhooks)+1)
	webHooks = append(webHooks, v1beta1.CanaryWebhook{
		Name: "flagger-default-halt",
		Type: v1beta1.ConfirmRolloutHook,
		URL:  internal.DefaultCanaryHaltUrl,
	})
	webHooks = append(webHooks, ca.Spec.Analysis.Webhooks...)
	ca.Spec.Analysis.Webhooks = webHooks
	new, e := rsr.flaggerClient.FlaggerV1beta1().Canaries(ca.Namespace).Update(context.Background(), ca, metav1.UpdateOptions{})
	new.DeepCopyInto(canary)
	return e
}

func (rsr *RollingUpdateSmiRouter) updateReplicas(canary *v1beta1.Canary, name string, replicas *int32) error {
	ctx := context.Background()
	dc := rsr.kubeClient.AppsV1().Deployments(canary.Namespace)
	dep, err := dc.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("can't query deployment %s.%s", name, canary.Namespace)
	}

	dep.Spec.Replicas = replicas
	_, err = dc.Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update deployment %s.%s replicas failed", name, canary.Namespace)
	}
	return nil
}

func (rsr *RollingUpdateSmiRouter) GetRoutes(canary *v1beta1.Canary) (primaryWeight int, canaryWeight int, mirrored bool, err error) {
	if internal.IsRollingUpdate(canary) {
		canaryName := canary.Spec.TargetRef.Name
		if cd, err := rsr.kubeClient.AppsV1().Deployments(canary.Namespace).Get(context.Background(), canaryName, metav1.GetOptions{}); err != nil {
			err = fmt.Errorf("canary %s.%s is not exist %w", canaryName, canary.Namespace, err)
		} else {
			canaryWeight = percentOf(int(cd.Status.ReadyReplicas), canary.Spec.Analysis.MaxReplicas)
			if canaryWeight > hundred {
				canaryWeight = hundred
			}
			primaryWeight = hundred - canaryWeight
		}
		return
	} else {
		return rsr.SmiRouter.GetRoutes(canary)
	}
}

func (rsr *RollingUpdateSmiRouter) Finalize(canary *v1beta1.Canary) error {
	return nil
}

func percentOf(part int, total int) int {
	if part > total {
		panic(fmt.Errorf("part: %d can't be great than total: %d", part, total))
	}
	return int((float64(part) * float64(hundred)) / float64(total))
}

func percent(percent int, all int) int {
	if percent > hundred || percent < 0 {
		panic(fmt.Errorf("percent: %d cant't be less than 0 or greater than 100", percent))
	}
	return int(math.Ceil((float64(all) * float64(percent)) / float64(hundred)))
}
