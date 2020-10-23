package canary

import (
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type oamWorkload struct {
	w *unstructured.Unstructured
}

func (o *oamWorkload) GetSpec() map[string]interface{} {
	obg, found, _ := unstructured.NestedMap(o.w.Object, "spec")
	if !found {
		return nil
	}
	return obg
}

func (o *oamWorkload) GetStatusObservedGeneration() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "observedGeneration")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "spec", "replicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetStatusReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "replicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetStatusUpdatedReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "updatedReplicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *oamWorkload) GetStatusAvailableReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "availableReplicas")
	if !found {
		return nil
	}
	return &obg
}

// isWorkloadReady determines if a workload is ready by checking the status conditions
// if a workload has exceeded the progress deadline it returns a non retriable error
func (o *oamWorkload) isWorkloadReady(deadline int) (bool, error) {
	retriable := true
	switch o.w.GetKind() {
	case "Deployment":
		deployData, err := o.w.MarshalJSON()
		if err != nil {
			return false, err
		}
		var deploy appsv1.Deployment
		err = json.Unmarshal(deployData, &deploy)
		if err != nil {
			return false, err
		}
		return IsDeploymentReady(&deploy, deadline)
	}

	// workload standard condition 1: has ObservedGeneration in status
	observedGeneration := o.GetStatusObservedGeneration()
	if observedGeneration == nil {
		return false, fmt.Errorf(
			"kind:%s is not standard OAM workload, status.observedGeneration not found", o.w.GetKind())
	}
	if o.w.GetGeneration() <= *observedGeneration {
		replica := o.GetReplicas()
		statusReplica := o.GetStatusReplicas()
		if statusReplica == nil {
			return false, fmt.Errorf(
				"kind:%s is not standard OAM workload, status.replicas not found", o.w.GetKind())
		}
		statusUpdatedReplicas := o.GetStatusUpdatedReplicas()
		if statusUpdatedReplicas == nil {
			return false, fmt.Errorf(
				"kind:%s is not standard OAM workload, status.updatedReplicas not found", o.w.GetKind())
		}
		if replica != nil && *statusUpdatedReplicas < *replica {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d out of %d new replicas have been updated",
				*statusUpdatedReplicas, *replica)
		} else if *statusReplica > *statusUpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d old replicas are pending termination",
				*statusReplica-*statusUpdatedReplicas)
		}
	} else {
		return true, fmt.Errorf(
			"waiting for rollout to finish: observed workload generation less then desired generation")
	}
	return true, nil
}

// IsWorkloadReady determines if a workload is ready by checking the status conditions
// if a workload has exceeded the progress deadline it returns a non retriable error
func IsWorkloadReady(workload *unstructured.Unstructured, deadline int) (bool, error) {
	oamWorkload := &oamWorkload{w: workload}
	return oamWorkload.isWorkloadReady(deadline)
}
