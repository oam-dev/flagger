package canary

import (
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type OAMWorkload struct {
	w *unstructured.Unstructured
}

func (o *OAMWorkload) GetSpec() map[string]interface{} {
	obg, found, _ := unstructured.NestedMap(o.w.Object, "spec")
	if !found {
		return nil
	}
	return obg
}

func (o *OAMWorkload) GetStatusObservedGeneration() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "observedGeneration")
	if !found {
		return nil
	}
	return &obg
}

func (o *OAMWorkload) GetReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "spec", "replicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *OAMWorkload) GetStatusReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "replicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *OAMWorkload) GetStatusUpdatedReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "updatedReplicas")
	if !found {
		return nil
	}
	return &obg
}

func (o *OAMWorkload) GetStatusAvailableReplicas() *int64 {
	obg, found, _ := unstructured.NestedInt64(o.w.Object, "status", "availableReplicas")
	if !found {
		return nil
	}
	return &obg
}

// isDeploymentReady determines if a deployment is ready by checking the status conditions
// if a deployment has exceeded the progress deadline it returns a non retriable error
func (o *OAMWorkload) isDeploymentReady(deadline int) (bool, error) {
	retriable := true
	observedGeneration := o.GetStatusObservedGeneration()
	if observedGeneration == nil {
		return false, fmt.Errorf(
			"kind:deployment, status.observedGeneration not found")
	}
	if o.w.GetGeneration() <= *observedGeneration {
		progress := getDeploymentCondition(o.w, appsv1.DeploymentProgressing)
		if progress != nil {
			// Determine if the deployment is stuck by checking if there is a minimum replicas unavailable condition
			// and if the last update time exceeds the deadline
			available := getDeploymentCondition(o.w, appsv1.DeploymentAvailable)
			if available != nil && available.Status == "False" && available.Reason == "MinimumReplicasUnavailable" {
				from := available.LastUpdateTime
				delta := time.Duration(deadline) * time.Second
				retriable = !from.Add(delta).Before(time.Now())
			}
		}

		replica := o.GetReplicas()
		statusReplica := o.GetStatusReplicas()
		if statusReplica == nil {
			return false, fmt.Errorf(
				"kind:deployment, status.replicas not found")
		}
		statusUpdatedReplicas := o.GetStatusUpdatedReplicas()
		if statusUpdatedReplicas == nil {
			return false, fmt.Errorf(
				"kind:deployment, status.updatedReplicas not found")
		}
		statusAvailableReplicas := o.GetStatusAvailableReplicas()
		if statusAvailableReplicas == nil {
			return false, fmt.Errorf(
				"kind:deployment, status.availableReplicas not found")
		}

		if progress != nil && progress.Reason == "ProgressDeadlineExceeded" {
			return false, fmt.Errorf("deployment %q exceeded its progress deadline", o.w.GetName())
		} else if replica != nil && *statusUpdatedReplicas < *replica {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d out of %d new replicas have been updated",
				*statusUpdatedReplicas, *replica)
		} else if *statusReplica > *statusUpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d old replicas are pending termination",
				*statusReplica-*statusUpdatedReplicas)
		} else if *statusAvailableReplicas < *statusUpdatedReplicas {
			return retriable, fmt.Errorf("waiting for rollout to finish: %d of %d updated replicas are available",
				*statusAvailableReplicas, *statusUpdatedReplicas)
		}
	} else {
		return true, fmt.Errorf(
			"waiting for rollout to finish: observed deployment generation less then desired generation")
	}
	return true, nil
}

func getDeploymentCondition(
	w *unstructured.Unstructured,
	conditionType appsv1.DeploymentConditionType,
) *appsv1.DeploymentCondition {
	statusInterface, found, _ := unstructured.NestedMap(w.Object, "status")
	if !found {
		return nil
	}
	data, err := json.Marshal(statusInterface)
	if err != nil {
		return nil
	}
	var status appsv1.DeploymentStatus

	if err = json.Unmarshal(data, &status); err != nil {
		return nil
	}
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == conditionType {
			return &c
		}
	}
	return nil
}

func (o *OAMWorkload) isWorkloadReady(deadline int) (bool, error) {
	retriable := true
	switch o.w.GetKind() {
	case "Deployment":
		return o.isDeploymentReady(deadline)
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
