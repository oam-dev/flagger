package router

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/canary"
	"github.com/weaveworks/flagger/pkg/internal"
)

type OAMRouteWrapper struct {
	logger      *zap.SugaredLogger
	innerRouter Interface
	scalar      *canary.OAMRolloutController
}

func NewOAMRouteWrapper(factory *Factory, scalar *canary.OAMRolloutController, provider, labelSelector string) *OAMRouteWrapper {
	return &OAMRouteWrapper{
		logger:      factory.logger,
		scalar:      scalar,
		innerRouter: factory.innerMeshRouter(provider, labelSelector),
	}
}

func (r *OAMRouteWrapper) Reconcile(canary *v1beta1.Canary) error {
	return r.innerRouter.Reconcile(canary)
}

func (r *OAMRouteWrapper) SetRoutes(canary *v1beta1.Canary, primaryWeight int, canaryWeight int, mirrored bool) error {
	r.logger.Infof("New traffic split of canary %s.%s, primary weight: %d, canary weight: %d",
		canary.GetName(), canary.GetNamespace(), primaryWeight, canaryWeight)
	if internal.IsPromoted(canary) {
		// only adjust canary replicas
		if err := r.innerRouter.SetRoutes(canary, 0, 100, mirrored); err != nil {
			return fmt.Errorf("adjust promoting router %s.%s failed %w, primaryWeight: %d, canaryWeight: %d", canary.Name, canary.Namespace, err, primaryWeight, canaryWeight)
		}
		// now scale up the target to max replica, the canary will be zeroed in ScaleToZero in the controller
		targetName := canary.Spec.TargetRef.Name
		targetReplica := int32(canary.Spec.Analysis.MaxReplicas)
		if err := r.scalar.Scale(targetName, targetReplica); err != nil {
			return fmt.Errorf("adjust replicas of primary deployment %s.%s failed %w, replicas: %d", targetName, canary.Namespace, err, targetReplica)
		}
		r.logger.Infof("Successfully promote the replicas of canary deployment %s.%s, replicas: %d", targetName,
			canary.Namespace, targetReplica)
	} else if internal.IsFailed(canary) {
		if err := r.innerRouter.SetRoutes(canary, 100, 0, mirrored); err != nil {
			return fmt.Errorf("adjust promoting router %s.%s failed %w, primaryWeight: %d, canaryWeight: %d", canary.Name, canary.Namespace, err, primaryWeight, canaryWeight)
		}
		// rollback, we make rollback as fast as possible.
		// now source is primary
		primaryName := r.getSourceName()
		primaryReplicas := int32(canary.Spec.Analysis.MaxReplicas)
		err := r.scalar.Scale(primaryName, primaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of primary deployment %s.%s failed %w, replicas: %d", primaryName, canary.Namespace, err, primaryReplicas)
		}
		r.logger.Infof("Successfully restored the replicas of primary deployment %s.%s, replicas: %d", primaryName,
			r.scalar.SourceWorkload.GetNamespace(), primaryReplicas)
		// now target is canary
		canaryName := canary.Spec.TargetRef.Name
		canaryReplicas := int32(0)
		err = r.scalar.Scale(canaryName,  canaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of canary deployment %s.%s failed %w, replicas: %d", canaryName, canary.Namespace, err, canaryReplicas)
		}
		r.logger.Infof("Successfully roll backed the replicas of canary deployment %s.%s, replicas: %d", canaryName,
			canary.Namespace, canaryReplicas)
	} else {
		if err := r.innerRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored); err != nil {
			return fmt.Errorf("adjust promoting router %s.%s failed %w, primaryWeight: %d, canaryWeight: %d", canary.Name, canary.Namespace, err, primaryWeight, canaryWeight)
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
		// save at least 1
		if canaryReplicas == 0 && canaryWeight != 0 {
			canaryReplicas = 1
		}
		canaryName := canary.Spec.TargetRef.Name
		err := r.scalar.Scale(canaryName, canaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of canary deployment %s.%s failed %w, replicas: %d", canaryName, canary.Namespace, err, canaryReplicas)
		}
		primaryReplicas := int32(maxReplicas) - canaryReplicas
		primaryName := r.getSourceName()
		if primaryReplicas == 0 && canaryWeight != hundred {
			// if canary weight is not 100%, we can't adjust primaryReplicas to 0
			// at least 1.
			primaryReplicas = 1
		}
		err = r.scalar.Scale(primaryName, primaryReplicas)
		if err != nil {
			return fmt.Errorf("adjust replicas of primary deployment %s.%s failed %w, replicas: %d", primaryName, r.scalar.SourceWorkload.GetNamespace(), err, primaryReplicas)
		}
	}
	return nil
}

func (r *OAMRouteWrapper) GetRoutes(canary *v1beta1.Canary) (primaryWeight int, canaryWeight int, mirrored bool, err error) {
	// use inner router weight
	return r.innerRouter.GetRoutes(canary)
}

func (r *OAMRouteWrapper) Finalize(_ *v1beta1.Canary) error {
	return nil
}

func (r *OAMRouteWrapper) getSourceName() string {
	return r.scalar.SourceWorkload.GetName()
}
