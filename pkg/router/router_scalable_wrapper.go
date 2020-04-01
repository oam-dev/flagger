package router

import (
	"context"
	"fmt"

	"github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"github.com/weaveworks/flagger/pkg/internal"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type RouterScalableWrapper struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	innerRouter   Interface
}

func (r *RouterScalableWrapper) Reconcile(canary *v1beta1.Canary) error {
	return r.innerRouter.Reconcile(canary)
}

func (r *RouterScalableWrapper) SetRoutes(canary *v1beta1.Canary, primaryWeight int, canaryWeight int, mirrored bool) error {
	if !internal.IsExtentOn(canary) {
		if err := r.innerRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored); err != nil {
			return fmt.Errorf("adjust promoting router %s.%s failed %w, primaryWeight: %d, canaryWeight: %d", canary.Name, canary.Namespace, err, primaryWeight, canaryWeight)
		}
		return nil
	}
	// only adjust canary replicas
	if internal.IsPromoted(canary) {
		if err := r.innerRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored); err != nil {
			return fmt.Errorf("adjust promoting router %s.%s failed %w, primaryWeight: %d, canaryWeight: %d", canary.Name, canary.Namespace, err, primaryWeight, canaryWeight)
		}
		// now target is primary
		primaryName := canary.Spec.TargetRef.Name
		primaryReplicas := int32(canary.Spec.Analysis.MaxReplicas)
		err := r.updateReplicas(canary, primaryName, &primaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of primary deployment %s.%s failed %w, replicas: %d", primaryName, canary.Namespace, err, primaryReplicas)
		}
		// now source is canary
		canaryName := r.getSourceName(canary)
		canaryReplicas := int32(0)
		err = r.updateReplicas(canary, canaryName, &canaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of canary deployment %s.%s failed %w, replicas: %d", canaryName, canary.Namespace, err, canaryReplicas)
		}
	} else if internal.IsFailed(canary) {
		if err := r.innerRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored); err != nil {
			return fmt.Errorf("adjust promoting router %s.%s failed %w, primaryWeight: %d, canaryWeight: %d", canary.Name, canary.Namespace, err, primaryWeight, canaryWeight)
		}
		// rollback, we make rollback as fast as possible.
		// now source is primary
		primaryName := r.getSourceName(canary)
		primaryReplicas := int32(canary.Spec.Analysis.MaxReplicas)
		err := r.updateReplicas(canary, primaryName, &primaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of primary deployment %s.%s failed %w, replicas: %d", primaryName, canary.Namespace, err, primaryReplicas)
		}
		// now target is canary
		canaryName := canary.Spec.TargetRef.Name
		canaryReplicas := int32(0)
		err = r.updateReplicas(canary, canaryName, &canaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of canary deployment %s.%s failed %w, replicas: %d", canaryName, canary.Namespace, err, canaryReplicas)
		}
	} else {
		if routable, err := r.checkRoutable(canary, primaryWeight, canaryWeight); err == nil && routable {
			if err := r.innerRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored); err != nil {
				return fmt.Errorf("adjust promoting router %s.%s failed %w, primaryWeight: %d, canaryWeight: %d", canary.Name, canary.Namespace, err, primaryWeight, canaryWeight)
			}
		}
		var canaryReplicas int32 = 0
		// prefer specified canary replicas
		if canary.Spec.Analysis.CanaryReplicas > 0 {
			canaryReplicas = int32(canary.Spec.Analysis.CanaryReplicas)
		}
		// use auto canary weight to compute canary replicas
		maxReplicas := canary.Spec.Analysis.MaxReplicas
		if canary.Spec.Analysis.StepWeight > 0 {
			canaryReplicas = int32(percent(canaryWeight, maxReplicas))
		}

		if canaryReplicas == 0 && canaryWeight != 0 {
			canaryReplicas = 1
		}

		canaryName := canary.Spec.TargetRef.Name
		err := r.updateReplicas(canary, canaryName, &canaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of canary deployment %s.%s failed %w, replicas: %d", canaryName, canary.Namespace, err, canaryReplicas)
		}

		canaryAvailableReplicas, err := r.getAvailableReplicas(canary, canaryName)
		if err != nil {
			return fmt.Errorf("query available replicas of canary deployment %s.%s failed %w", canaryName, canary.Namespace, err)
		}

		primaryReplicas := int32(maxReplicas) - canaryAvailableReplicas
		primaryName := r.getSourceName(canary)
		if primaryReplicas == 0 && canaryWeight != 100 {
			// if canary weight is not 100%, we can't adjust primaryReplicas to 0
			// at least 1.
			primaryReplicas = 1
		}
		err = r.updateReplicas(canary, primaryName, &primaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of primary deployment %s.%s failed %w, replicas: %d", primaryName, canary.Namespace, err, primaryReplicas)
		}
	}
	return nil
}

func (r *RouterScalableWrapper) haltCanary(canary *v1beta1.Canary) error {
	ca := canary.DeepCopy()
	webHooks := make([]v1beta1.CanaryWebhook, 0, len(ca.Spec.Analysis.Webhooks)+1)
	webHooks = append(webHooks, v1beta1.CanaryWebhook{
		Name: "flagger-default-halt",
		Type: v1beta1.ConfirmRolloutHook,
		URL:  internal.DefaultCanaryHaltUrl,
	})
	webHooks = append(webHooks, ca.Spec.Analysis.Webhooks...)
	ca.Spec.Analysis.Webhooks = webHooks
	new, e := r.flaggerClient.FlaggerV1beta1().Canaries(ca.Namespace).Update(context.Background(), ca, metav1.UpdateOptions{})
	new.DeepCopyInto(canary)
	return e
}

func (r *RouterScalableWrapper) updateReplicas(canary *v1beta1.Canary, name string, replicas *int32) error {
	ctx := context.Background()
	dc := r.kubeClient.AppsV1().Deployments(canary.Namespace)
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

func (r *RouterScalableWrapper) GetRoutes(canary *v1beta1.Canary) (primaryWeight int, canaryWeight int, mirrored bool, err error) {
	if internal.IsExtentOn(canary) && canary.Spec.Analysis.StepWeight <= 0 {
		// prefer specified canary weight
		canaryWeight = canary.Spec.Analysis.CanaryWeight
		primaryWeight = hundred - canaryWeight
		return
	}
	// use inner router weight
	return r.innerRouter.GetRoutes(canary)
}

func (r *RouterScalableWrapper) Finalize(canary *v1beta1.Canary) error {
	return nil
}

func (r *RouterScalableWrapper) getAvailableReplicas(canary *v1beta1.Canary, name string) (int32, error) {
	dc := r.kubeClient.AppsV1().Deployments(canary.Namespace)
	dep, err := dc.Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("can't query deployment %s.%s", name, canary.Namespace)
	}
	return dep.Status.AvailableReplicas, nil
}

func (r *RouterScalableWrapper) getSourceName(canary *v1beta1.Canary) string {
	var sourceName string
	if internal.HasSourceTargetRef(canary) {
		sourceName = canary.Spec.SourceRef.Name
	} else {
		sourceName = fmt.Sprintf("%s-primary", canary.Spec.TargetRef.Name)
	}
	return sourceName
}

/**
some conditions are not routable, e.g: route traffic to zero replica workload will cause problem
only invoked in progressing phase
*/
func (r *RouterScalableWrapper) checkRoutable(canary *v1beta1.Canary, primaryWeight int, canaryWeight int) (bool, error) {
	var primaryRoutable bool
	primaryName := r.getSourceName(canary)
	primaryAvailableReplicas, err := r.getAvailableReplicas(canary, primaryName)
	if err != nil {
		return false, err
	}
	primaryRoutable = primaryWeight == 0 || primaryAvailableReplicas > 0

	var canaryRoutable bool
	canaryName := canary.Spec.TargetRef.Name
	canaryAvailableReplicas, err := r.getAvailableReplicas(canary, canaryName)
	if err != nil {
		return false, err
	}
	canaryRoutable = canaryWeight == 0 || canaryAvailableReplicas > 0

	return primaryRoutable && canaryRoutable, nil
}
