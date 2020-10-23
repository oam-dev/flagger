package canary

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2/applicationconfiguration"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	oamutil "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func GetUnstructured(ctx context.Context, kind, apiVersion, name, namespace string, c client.Client) (*unstructured.Unstructured, error) {
	var workload unstructured.Unstructured
	workload.SetKind(kind)
	workload.SetAPIVersion(apiVersion)
	if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &workload); err != nil {
		return nil, err
	}
	return &workload, nil
}

type Revisions []v1.ControllerRevision

func (r Revisions) Len() int           { return len(r) }
func (r Revisions) Less(i, j int) bool { return r[i].Revision > r[j].Revision }
func (r Revisions) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }

func FindSourceWorkload(canary *flaggerv1.Canary, c client.Client, kubeClient kubernetes.Interface) (*unstructured.Unstructured, error) {
	ctx := context.TODO()
	workload, err := GetUnstructured(ctx, canary.Spec.TargetRef.Kind, canary.Spec.TargetRef.APIVersion, canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, c)
	if err != nil {
		return nil, fmt.Errorf("get OAM workload %s.%s err %v", canary.Spec.TargetRef.Name, canary.Spec.TargetRef.Namespace, err)
	}
	lb := workload.GetLabels()
	if lb == nil {
		return nil, fmt.Errorf("didn't find any labels from OAM workload %v", canary.Spec.TargetRef)
	}
	componentName := lb[oam.LabelAppComponent]
	revisionList, err := kubeClient.AppsV1().ControllerRevisions(canary.Namespace).List(ctx,
		metav1.ListOptions{LabelSelector: labels.SelectorFromValidatedSet(
			map[string]string{applicationconfiguration.ControllerRevisionComponentLabel: componentName},
		).String()})
	if err != nil {
		return nil, fmt.Errorf("get revision from component %s err %v", componentName, err)
	}
	var r Revisions = revisionList.Items
	sort.Sort(r)
	for _, rev := range revisionList.Items {
		if rev.Name == canary.Spec.TargetRef.Name {
			continue
		}
		workloadName := rev.Name
		revisionWorkload, err := GetUnstructured(ctx, canary.Spec.TargetRef.Kind, canary.Spec.TargetRef.APIVersion, workloadName, canary.Spec.TargetRef.Namespace, c)
		if err != nil {
			if kerrors.IsNotFound(err) {
				return getWorkloadByName(ctx, c, rev)
			}
			return nil, errors.Wrap(err, fmt.Sprintf("source workload of %v not found", componentName))
		}
		// Searching from latest revision to oldest,
		// Use the latest running workload not named target as the source workload.
		return revisionWorkload, nil
	}
	return nil, fmt.Errorf("source workload of %v not found", componentName)
}

// The workload is named instead using the OAM revision convention
// we need to find out the name from the workload inside the component revision
func getWorkloadByName(ctx context.Context, c client.Client, rev v1.ControllerRevision) (*unstructured.Unstructured, error) {
	component, err := oamutil.UnpackRevisionData(&rev)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to unpack componentRevision %s with revision %s",
			rev.Name, rev.Revision))
	}
	// get the workload object from the component
	var res map[string]interface{}
	err = json.Unmarshal(component.Spec.Workload.Raw, &res)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Failed to extract workload from component %s with status %v",
			component.Name, component.Status))
	}
	wl := unstructured.Unstructured{
		Object: res,
	}
	workloadName := wl.GetName()
	// it cannot be empty as we have tried to use the revision name convention
	if workloadName == "" {
		return nil, fmt.Errorf("the workload has no name and we cannot find"+
			" the revision workload from component %s with status %v",
			component.Name, component.Status)
	}
	revisionWorkload, err := GetUnstructured(ctx, wl.GetKind(), wl.GetAPIVersion(), workloadName, wl.GetNamespace(), c)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("source workload of %s not found in component %s",
			workloadName, component.GetName()))
	}
	return revisionWorkload, nil
}
