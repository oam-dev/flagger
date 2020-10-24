package canary

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	autoscalingapi "k8s.io/api/autoscaling/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/scale"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	"github.com/weaveworks/flagger/pkg/internal"
)

var _ Controller = &OAMRolloutController{}

type OAMRolloutController struct {
	scaleClient   scale.ScalesGetter
	client        client.Client
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
	logger        *zap.SugaredLogger
	configTracker Tracker
	labels        []string
	// we only need to fetch them once
	SourceWorkload *unstructured.Unstructured
	targetWorkload *unstructured.Unstructured
}

func NewRollingController(factory *Factory, canary *flaggerv1.Canary) (*OAMRolloutController, error) {
	cfg := factory.kubeCfg
	kubeClient := factory.kubeClient
	flaggerClient := factory.flaggerClient
	logger := factory.logger
	configTracker := factory.configTracker
	labels := factory.labels
	var scheme = runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	restMapper, err := apiutil.NewDynamicRESTMapper(cfg)
	if err != nil {
		return nil, err
	}
	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(kubeClient.Discovery())
	scaleClient, err := scale.NewForConfig(cfg, restMapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	if err != nil {
		return nil, err
	}
	controller := OAMRolloutController{
		scaleClient:   scaleClient,
		client:        c,
		logger:        logger,
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
		labels:        labels,
		configTracker: configTracker,
	}
	// fetch the source once and for all since we can't use the informer cache
	err = controller.fetchSourceWorkload(canary)
	if err != nil {
		return nil, err
	}
	// fetch the target once and for all since we can't use the informer cache
	controller.targetWorkload, err = GetUnstructured(context.TODO(), canary.Spec.TargetRef.Kind,
		canary.Spec.TargetRef.APIVersion, canary.Spec.TargetRef.Name, canary.GetNamespace(), c)
	if err != nil {
		return nil, err
	}
	return &controller, err
}

func (orc *OAMRolloutController) Initialize(canary *flaggerv1.Canary) (err error) {
	if canary.Status.Phase == "" || canary.Status.Phase == flaggerv1.CanaryPhaseInitializing {
		if !canary.SkipAnalysis() {
			if _, err := orc.IsCanaryReady(canary); err != nil {
				return fmt.Errorf("%w", err)
			}
		}
		// scale the target/canary resource to zero first
		if err := orc.Scale(canary.Spec.TargetRef.Name, 0); err != nil {
			return fmt.Errorf("scaling down canary resource %s.%s failed: %w", canary.Spec.TargetRef.Name,
				canary.Namespace, err)
		}
		orc.logger.Infof("scaling down canary resource %s.%s to zero succeed", canary.Spec.TargetRef.Name,
			canary.Namespace)
		// scale the source resource to canary setting
		if err := orc.Scale(orc.SourceWorkload.GetName(), int32(canary.Spec.Analysis.MaxReplicas)); err != nil {
			return fmt.Errorf("scaling down canary resource %s.%s failed: %w", orc.SourceWorkload.GetName(),
				canary.Namespace, err)
		}
		orc.logger.With("canary", fmt.Sprintf("%s.%s", canary.Name, canary.Namespace)).
			Infof("scaling primary resource %s.%s to %d succeed", orc.SourceWorkload.GetName(),
				canary.Namespace, canary.Spec.Analysis.MaxReplicas)
	}
	return nil
}

func (orc *OAMRolloutController) fetchSourceWorkload(canary *flaggerv1.Canary) error {
	var err error
	if canary.Spec.SourceRef != nil {
		orc.SourceWorkload, err = GetUnstructured(context.TODO(), canary.Spec.SourceRef.Kind,
			canary.Spec.SourceRef.APIVersion,
			canary.Spec.SourceRef.Name, canary.Spec.SourceRef.Namespace, orc.client)
		if err != nil {
			orc.logger.Error("failed to locate the resource from resourceRef := %v, err = %s",
				canary.Spec.SourceRef, err)
			return errors.Wrap(err, fmt.Sprintf("failed to locate the resource from resourceRef: %v",
				canary.Spec.SourceRef))

		}
	} else {
		orc.SourceWorkload, err = FindSourceWorkload(canary, orc.client, orc.kubeClient)
		if err != nil {
			orc.logger.Errorf("failed to auto locate the source, err = %s", err)
			return errors.Wrap(err, fmt.Sprintf("failed to auto locate the source for canary %s", canary.Name))
		}
	}
	return nil
}

func (orc *OAMRolloutController) IsPrimaryReady(canary *flaggerv1.Canary) error {
	// Source is the Primary
	_, err := IsWorkloadReady(orc.SourceWorkload, canary.GetProgressDeadlineSeconds())
	return err
}

func (orc *OAMRolloutController) IsCanaryReady(canary *flaggerv1.Canary) (bool, error) {
	// Target is the Canary
	return IsWorkloadReady(orc.targetWorkload, canary.GetProgressDeadlineSeconds())
}

func (orc *OAMRolloutController) Promote(canary *flaggerv1.Canary) error {
	// update the canary status
	if canary.Status.CanaryWeight != 100 {
		if err := orc.SetStatusWeight(canary, 100); err != nil {
			return err
		}
	}
	return nil
}

// The rollback and promoting are different process in OAM while it's the same in the flagger workflow
func (orc *OAMRolloutController) ScaleToZero(canary *flaggerv1.Canary) error {
	if internal.IsPromoted(canary) {
		// We need to scale the source/primary to zero when the the canary is being promoted
		return orc.Scale(orc.SourceWorkload.GetName(), 0)
	}
	// in other cases, it's a rollback, we
	return orc.Scale(orc.SourceWorkload.GetName(), int32(canary.Spec.Analysis.MaxReplicas))
}

func (orc *OAMRolloutController) ScaleFromZero(_ *flaggerv1.Canary) error {
	// We don't need to scale from zero, only need for cold start from HPA
	return nil
}

// GetMetadata returns selectorLabel and ports for creating service.
// The target ports are already specified in canary service spec,
// ports here are additional ones which were discovered automatically
func (orc *OAMRolloutController) GetMetadata(cd *flaggerv1.Canary) (string, map[string]int32, error) {
	var label string
	var ports map[string]int32

	// we assume that the workload label is the same as the pod template selector
	workload := orc.targetWorkload
	targetLabels := workload.GetLabels()
	for _, l := range orc.labels {
		if _, ok := targetLabels[l]; ok {
			// we pick the first key that exists in the workload
			label = l
			break
		}
	}
	if len(label) == 0 {
		return "", nil, fmt.Errorf("workload %s.%s meta data label must contain one of %v",
			workload.GetName(), workload.GetNamespace(), orc.labels)
	}

	if cd.Spec.Service.PortDiscovery {
		//Assume the workload will always using spec.template as PodTemplate if ports is discoverable
		obj, found, err := unstructured.NestedMap(workload.Object, "spec", "template")
		if err != nil {
			return "", nil, fmt.Errorf("failed to discover port from OAM workload %s.%s err %v",
				cd.Spec.TargetRef.Name, cd.GetNamespace(), err)
		}
		if found {
			data, err := json.Marshal(obj)
			if err != nil {
				return label, nil, nil
			}
			var template v1.PodTemplate
			err = json.Unmarshal(data, &template)
			if err != nil {
				return label, nil, nil
			}
			ports = getPorts(cd, template.Template.Spec.Containers)
		}
	}
	return label, ports, nil
}

func (orc *OAMRolloutController) SyncStatus(canary *flaggerv1.Canary, status flaggerv1.CanaryStatus) error {
	// assuming that the target object has a spec
	obg, found, err := unstructured.NestedMap(orc.targetWorkload.Object, "spec")
	if err != nil {
		return fmt.Errorf("fetch OAM workload spec %s.%s err %v", canary.Spec.TargetRef.Name,
			canary.GetNamespace(), err)
	}
	if !found {
		return fmt.Errorf("OAM workload spec %s.%s not found", canary.Spec.TargetRef.Name,
			canary.GetNamespace())
	}

	configs, err := orc.configTracker.GetConfigRefs(canary)
	if err != nil {
		if !strings.Contains(err.Error(), "TargetRef.Kind invalid:") {
			return fmt.Errorf("GetConfigRefs failed: %w", err)
		}
		configs = nil
	}

	return syncCanaryStatus(orc.flaggerClient, canary, status, obg, func(cdCopy *flaggerv1.Canary) {
		cdCopy.Status.TrackedConfigs = configs
	})
}

func (orc *OAMRolloutController) SetStatusFailedChecks(cd *flaggerv1.Canary, val int) error {
	return setStatusFailedChecks(orc.flaggerClient, cd, val)
}

func (orc *OAMRolloutController) SetStatusWeight(cd *flaggerv1.Canary, val int) error {

	return setStatusWeight(orc.flaggerClient, cd, val)
}

func (orc *OAMRolloutController) SetStatusIterations(cd *flaggerv1.Canary, val int) error {
	return setStatusIterations(orc.flaggerClient, cd, val)
}

func (orc *OAMRolloutController) SetStatusPhase(cd *flaggerv1.Canary, phase flaggerv1.CanaryPhase) error {
	return setStatusPhase(orc.flaggerClient, cd, phase)
}

func (orc *OAMRolloutController) HasTargetChanged(canary *flaggerv1.Canary) (bool, error) {
	// HasTargetChanged returns whether the workload has changed, in OAM rolling update mode,
	// the target workload is on a fixed revision, it won't change.
	if canary.Status.Phase == flaggerv1.CanaryPhaseInitialized {
		// However, we can't make progress if we always return false when the canary status is initialized
		// the canary status will be stuck and never advance
		return true, nil
	}
	return false, nil
}

func (orc *OAMRolloutController) HaveDependenciesChanged(canary *flaggerv1.Canary) (bool, error) {
	changed, err := orc.configTracker.HasConfigChanged(canary)
	if err != nil && strings.Contains(err.Error(), "TargetRef.Kind invalid:") {
		return false, nil
	}
	return changed, err
}

// Finalize will revert rolling update back, we just scale up here.
func (orc *OAMRolloutController) Finalize(canary *flaggerv1.Canary) error {
	return orc.Scale(orc.SourceWorkload.GetName(), int32(canary.Spec.Analysis.MaxReplicas))
}

// Scale sets the canary workload replicas
func (orc *OAMRolloutController) Scale(resourceName string, replicas int32) error {
	ctx := context.TODO()
	var res *unstructured.Unstructured
	if orc.SourceWorkload.GetName() == resourceName {
		res = orc.SourceWorkload
		orc.logger.Infof("Going to scale the source/primary resource %s", res.GetName())
	} else if orc.targetWorkload.GetName() == resourceName {
		res = orc.targetWorkload
		orc.logger.Infof("Going to scale the target/canary resource %s", res.GetName())
	} else {
		return fmt.Errorf("cannot scale an unknown resource %s", resourceName)
	}
	resPatch := client.MergeFrom(res.DeepCopyObject())
	// TOOD: work with other paths, we assume it's spec.replicas for now which works with most of k8s/oam workloads
	err := unstructured.SetNestedField(res.Object, int64(replicas), "spec", "replicas")
	if err != nil {
		orc.logger.Errorf("Failed to modify the scale of a resource %s with err = %v", res.GetName(), err)
		return err
	}
	// merge patch to scale the resource
	if err := orc.client.Patch(ctx, res, resPatch, &client.PatchOptions{}); err != nil {
		orc.logger.Errorf("Failed to scale a resource %s with err = %v", res.GetName(), err)
		return err
	}
	orc.logger.Infof("Successfully scaled a resource, resource GVK = %s, name = %s, target replica= %d",
		res.GroupVersionKind().String(), res.GetName(), replicas)
	return nil
}

// utilize the 'scale' subresource in k8s, experimental feature, not tested
func (orc *OAMRolloutController) scaleWithSubResource(name, namespace, apiVersion string, replicas int32) error {
	ctx := context.Background()
	resourceList, err := orc.kubeClient.Discovery().ServerResourcesForGroupVersion(apiVersion)
	if err != nil {
		return err
	}
	var resource, group string
	for _, r := range resourceList.APIResources {
		if strings.Contains(r.Name, "/") {
			continue
		}
		group = r.Group
		resource = r.Name
		break
	}
	sc := &autoscalingapi.Scale{}
	sc.Name = name
	sc.Namespace = namespace
	sc.Spec.Replicas = replicas

	sc, err = orc.scaleClient.Scales(namespace).Update(ctx, schema.GroupResource{
		Group:    group,
		Resource: resource,
	}, sc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("scaling %s.%s to %v by scale subresource failed: %w", name, namespace, replicas, err)
	}
	return nil
}
