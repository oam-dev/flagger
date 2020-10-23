package controller

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2/applicationconfiguration"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	oamutil "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	smiv1alpha1 "github.com/weaveworks/flagger/pkg/apis/smi/v1alpha1"

	canaryv1 "github.com/weaveworks/flagger/pkg/canary"
	"github.com/weaveworks/flagger/pkg/router"
)

const selectLabel = "app"
const meshProvider = flaggerv1.SMIProvider

// create the canary for test
func getRollingUpgradeCanary(name, deployment, sourceName string) *flaggerv1.Canary {
	canary := flaggerv1.Canary{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oct_namespace,
			Name:      name,
		},
		Spec: flaggerv1.CanarySpec{
			Provider: meshProvider,
			TargetRef: flaggerv1.CrossNamespaceObjectReference{
				Name:       deployment,
				Namespace:  oct_namespace,
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			AutoscalerRef: &flaggerv1.CrossNamespaceObjectReference{
				Name:       name,
				APIVersion: "autoscaling/v2beta1",
				Kind:       "HorizontalPodAutoscaler",
			}, Service: flaggerv1.CanaryService{
				Port: 9898,
			}, Analysis: &flaggerv1.CanaryAnalysis{
				Interval:     "1m",
				Threshold:    10,
				StepWeight:   10,
				MaxWeight:    50,
				StepReplicas: 2,
				MaxReplicas:  7,
			},
		},
	}
	if sourceName != "" {
		canary.Spec.SourceRef = &flaggerv1.CrossNamespaceObjectReference{
			Name:       sourceName,
			Namespace:  oct_namespace,
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		}
	}
	return &canary
}

// create the deployment for canary test
func getTestDeploymentWithRevision(name, version, componentName string, revision int64) (*appsv1.Deployment,
	*appsv1.ControllerRevision) {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oct_namespace,
			Name:      name,
			Labels: map[string]string{
				oam.LabelAppComponent: componentName,
				selectLabel:           componentName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectLabel: componentName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						selectLabel: componentName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: name,
							// replace with busybox when get a chance
							Image: "stefanprodan/podinfo:" + version,
							Command: []string{
								"./podinfo",
								"--port=9898",
							},
							Args:       nil,
							WorkingDir: "",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 9898,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "http-metrics",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									ContainerPort: 8888,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/podinfo/config",
									ReadOnly:  true,
								},
								{
									Name:      "secret",
									MountPath: "/etc/podinfo/secret",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "podinfo-config-vol",
									},
								},
							},
						},
						{
							Name: "secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "podinfo-secret-vol",
								},
							},
						},
					},
				},
			},
		},
	}

	// create a component for this deployment
	dc := &v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: oct_namespace,
		},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Raw: oamutil.JSONMarshal(d),
			},
		},
	}
	// create a controller revision for this component
	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      applicationconfiguration.ConstructRevisionName(componentName, revision),
			Namespace: oct_namespace,
			Labels: map[string]string{
				applicationconfiguration.ControllerRevisionComponentLabel: componentName,
			},
		},
		Data:     runtime.RawExtension{Object: dc},
		Revision: revision,
	}

	return d, cr
}

func setupTestSecretVol() error {
	secretVol := corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oct_namespace,
			Name:      "podinfo-secret-vol",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"apiKey": []byte("test"),
		},
	}
	return k8sClient.Create(context.TODO(), &secretVol)
}

func setupTestConfigMapVol() error {
	configMapVol := corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oct_namespace,
			Name:      "podinfo-config-vol",
		},
		Data: map[string]string{
			"color": "red",
		},
	}
	return k8sClient.Create(context.TODO(), &configMapVol)
}

var _ = Describe("OAM Rolling Upgrade Test Suite", func() {
	var canary *flaggerv1.Canary
	var canaryName, componentName string
	var targetResource, sourceResource *appsv1.Deployment
	var targetRevision, sourceRevision int64
	var scr, tcr *appsv1.ControllerRevision

	initializeController := func() {
		gomega.Eventually(
			func() error {
				// we need to recreate the controller like each scheduling iteration
				rollingController, err := canaryv1.NewRollingController(canaryFactory, canary)
				if err != nil {
					return err
				}
				return rollingController.Initialize(canary)
			},
			time.Second*60, time.Second).ShouldNot(gomega.HaveOccurred())

		var targetDeploy appsv1.Deployment
		gomega.Eventually(
			func() error {
				objectKey := client.ObjectKey{
					Name:      targetResource.GetName(),
					Namespace: targetResource.GetNamespace(),
				}
				err := k8sClient.Get(context.TODO(), objectKey, &targetDeploy)
				if err != nil {
					return err
				}
				_, err = canaryv1.IsDeploymentReady(&targetDeploy, 0)
				return err
			},
			time.Second*10, time.Second).ShouldNot(gomega.HaveOccurred())
		// we scale down the target to zero at initialization
		gomega.Expect(*targetDeploy.Spec.Replicas).Should(gomega.BeEquivalentTo(0))
		var sourceDeploy appsv1.Deployment
		gomega.Eventually(
			func() error {
				objectKey := client.ObjectKey{
					Name:      sourceResource.GetName(),
					Namespace: sourceResource.GetNamespace(),
				}
				err := k8sClient.Get(context.TODO(), objectKey, &sourceDeploy)
				if err != nil {
					return err
				}
				_, err = canaryv1.IsDeploymentReady(&sourceDeploy, 0)
				return err
			},
			time.Second*10, time.Second).ShouldNot(gomega.HaveOccurred())
		// we scale down the target to zero at initialization
		gomega.Expect(*sourceDeploy.Spec.Replicas).Should(gomega.BeEquivalentTo(canary.Spec.Analysis.MaxReplicas))
	}

	initializedKubeRouter := func() {
		// get the controller again
		rollingController, err := canaryv1.NewRollingController(canaryFactory, canary)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		labelSelector, ports, err := rollingController.GetMetadata(canary)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		kubeRouter := router.NewKubernetesOAMRouter(ctrl.routerFactory, labelSelector, ports, componentName)
		err = kubeRouter.Initialize(canary)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		// check canary/primary services have been created/updated
		var mainSvc, sourceSvc, targetSvc corev1.Service
		mainName, primaryName, canaryName := canary.GetServiceNames()
		gomega.Eventually(
			func() error {
				objectKey := client.ObjectKey{
					Name:      canaryName,
					Namespace: targetResource.GetNamespace(),
				}
				return k8sClient.Get(context.TODO(), objectKey, &targetSvc)
			},
			time.Second*2, time.Millisecond*200).ShouldNot(gomega.HaveOccurred())
		value, exist := targetSvc.Spec.Selector[selectLabel]
		gomega.Expect(exist).Should(gomega.BeTrue())
		gomega.Expect(value).Should(gomega.Equal(componentName))
		// check source svc that points to the right pods
		gomega.Eventually(
			func() error {
				objectKey := client.ObjectKey{
					Name:      primaryName,
					Namespace: sourceResource.GetNamespace(),
				}
				return k8sClient.Get(context.TODO(), objectKey, &sourceSvc)
			},
			time.Second*2, time.Millisecond*200).ShouldNot(gomega.HaveOccurred())
		value, exist = sourceSvc.Spec.Selector[selectLabel]
		gomega.Expect(exist).Should(gomega.BeTrue())
		gomega.Expect(value).Should(gomega.Equal(componentName))
		// check main svc that points to the right pods
		err = kubeRouter.Reconcile(canary)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		gomega.Eventually(
			func() error {
				objectKey := client.ObjectKey{
					Name:      mainName,
					Namespace: sourceResource.GetNamespace(),
				}
				return k8sClient.Get(context.TODO(), objectKey, &mainSvc)
			},
			time.Second*2, time.Millisecond*200).ShouldNot(gomega.HaveOccurred())
		value, exist = mainSvc.Spec.Selector[selectLabel]
		gomega.Expect(exist).Should(gomega.BeTrue())
		gomega.Expect(value).Should(gomega.Equal(componentName))
	}

	BeforeEach(func() {
		By("[OAM Rolling Controller Test] Setup up resources")
		// randomize the canary name
		componentName = "rolling-component"
		targetRevision = rand.Int63n(1000) + 3
		targetName := applicationconfiguration.ConstructRevisionName(componentName, targetRevision)
		targetResource, tcr = getTestDeploymentWithRevision(targetName, "5.0.2", componentName, targetRevision)
		err := k8sClient.Create(context.TODO(), targetResource)
		gomega.Expect(err).Should(gomega.SatisfyAny(gomega.BeNil(), &oamutil.AlreadyExistMatcher{}))
		err = k8sClient.Create(context.TODO(), tcr)
		gomega.Expect(err).Should(gomega.SatisfyAny(gomega.BeNil(), &oamutil.AlreadyExistMatcher{}))
		By("Target resources and revisions created")
		sourceRevision = targetRevision - 2
		sourceName := applicationconfiguration.ConstructRevisionName(componentName, sourceRevision)
		sourceResource, scr = getTestDeploymentWithRevision(sourceName, "4.0.6", componentName, sourceRevision)
		By("Source resources and revisions created")
		canaryName = "test-" + strconv.FormatInt(rand.Int63(), 16)
		targetName = applicationconfiguration.ConstructRevisionName(componentName, targetRevision)
		canary = getRollingUpgradeCanary(canaryName, targetName, "")
		err = k8sClient.Create(context.TODO(), canary)
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		By("Canary generated and created")
	})

	AfterEach(func() {
		// Control-runtime test environment has a bug that can't delete resources like deployment/namespaces
		// We have to use different names to segregate between tests
		By("[TEST] Clean up resources after an integration test")
		err := k8sClient.Delete(context.TODO(), canary, client.PropagationPolicy(metav1.DeletePropagationForeground))
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		err = k8sClient.Delete(context.TODO(), targetResource, client.PropagationPolicy(metav1.DeletePropagationForeground))
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		err = k8sClient.Delete(context.TODO(), tcr, client.PropagationPolicy(metav1.DeletePropagationForeground))
		gomega.Expect(err).Should(gomega.SatisfyAny(gomega.BeNil(), &oamutil.NotFoundMatcher{}))
		err = k8sClient.Delete(context.TODO(), sourceResource, client.PropagationPolicy(metav1.DeletePropagationForeground))
		gomega.Expect(err).Should(gomega.SatisfyAny(gomega.BeNil(), &oamutil.NotFoundMatcher{}))
		err = k8sClient.Delete(context.TODO(), scr, client.PropagationPolicy(metav1.DeletePropagationForeground))
		gomega.Expect(err).Should(gomega.SatisfyAny(gomega.BeNil(), &oamutil.NotFoundMatcher{}))
	})

	Context("OAM Rolling Canary Controller Test Suite", func() {
		BeforeEach(func() {
			err := k8sClient.Create(context.TODO(), sourceResource)
			gomega.Expect(err).Should(gomega.SatisfyAny(gomega.BeNil(), &oamutil.AlreadyExistMatcher{}))
			err = k8sClient.Create(context.TODO(), scr)
			gomega.Expect(err).Should(gomega.SatisfyAny(gomega.BeNil(), &oamutil.AlreadyExistMatcher{}))
			By("Source resources and revisions created in the cluster")
		})

		It("Test OAM rolling controller initialize", func() {
			initializeController()
		})

		It("Test OAM K8s router initialize", func() {
			initializeController()
			initializedKubeRouter()
		})

		It("Test OAM Mesh router set then get route", func() {
			initializeController()
			// get the controller again
			rollingController, err := canaryv1.NewRollingController(canaryFactory, canary)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			labelSelector, _, err := rollingController.GetMetadata(canary)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			meshRouter := router.NewOAMRouteWrapper(routerFactory, rollingController, meshProvider, labelSelector)
			err = meshRouter.Reconcile(canary)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			primaryWeight := 10
			canaryWeight := 90
			mirrored := false
			err = meshRouter.SetRoutes(canary, primaryWeight, canaryWeight, mirrored)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			p, c, m, err := meshRouter.GetRoutes(canary)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(p).Should(gomega.BeEquivalentTo(primaryWeight))
			gomega.Expect(c).Should(gomega.BeEquivalentTo(canaryWeight))
			gomega.Expect(m).Should(gomega.BeFalse())
		})

		FIt("Test advance canary end to end", func() {
			step := int(math.Ceil(float64(canary.Spec.Analysis.MaxReplicas*canary.Spec.Analysis.StepWeight) / 100))
			var sourceDeploy, targetDeploy appsv1.Deployment
			var updatedCanary flaggerv1.Canary
			canaryWeight := 0
			// follow the canary process one step at a time
			for i := step; i <= canary.Spec.Analysis.MaxReplicas; i += step {
				canaryWeight += canary.Spec.Analysis.StepWeight
				for {
					// loop until the canary number is right
					objectKey := client.ObjectKey{
						Name:      targetResource.GetName(),
						Namespace: targetResource.GetNamespace(),
					}
					ctrl.advanceCanary(canary.Name, canary.Namespace)
					gomega.Eventually(
						func() error {
							err := k8sClient.Get(context.TODO(), objectKey, &targetDeploy)
							if err != nil {
								return err
							}
							_, err = canaryv1.IsDeploymentReady(&targetDeploy, 0)
							return err
						}, time.Second*10, time.Millisecond*500).ShouldNot(gomega.HaveOccurred())
					if targetDeploy.Status.ReadyReplicas < int32(i) {
						continue
					} else {
						// check we expanded by one step at time, check on both deployment
						gomega.Expect(targetDeploy.Status.ReadyReplicas).Should(gomega.Equal(int32(i)))
						objectKey := client.ObjectKey{
							Name:      sourceResource.GetName(),
							Namespace: sourceResource.GetNamespace(),
						}
						gomega.Eventually(
							func() error {
								err := k8sClient.Get(context.TODO(), objectKey, &sourceDeploy)
								if err != nil {
									return err
								}
								_, err = canaryv1.IsDeploymentReady(&sourceDeploy, 0)
								if err != nil {
									return err
								}
								if sourceDeploy.Status.ReadyReplicas != int32(canary.Spec.Analysis.MaxReplicas-i) {
									return fmt.Errorf("wrong source replica = %d", sourceDeploy.Status.ReadyReplicas)
								}
								return nil
							},
							time.Second*2, time.Millisecond*200).ShouldNot(gomega.HaveOccurred())
						break
					}
				}
				// check TrafficSplit status
				var smiRoute smiv1alpha1.TrafficSplit
				trafficKey := client.ObjectKey{
					Name:      targetResource.GetName(), //the mesh route is the same as the target
					Namespace: canary.GetNamespace(),
				}
				err := k8sClient.Get(context.TODO(), trafficKey, &smiRoute)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				for _, backend := range smiRoute.Spec.Backends {
					if strings.Contains(backend.Service, "canary") {
						gomega.Expect(backend.Weight.Value()).Should(gomega.BeEquivalentTo(canaryWeight))
					} else {
						gomega.Expect(backend.Weight.Value()).Should(gomega.BeEquivalentTo(100 - canaryWeight))
					}
				}
				// check canary status
				objectKey := client.ObjectKey{
					Name:      canary.GetName(),
					Namespace: canary.GetNamespace(),
				}
				err = k8sClient.Get(context.TODO(), objectKey, &updatedCanary)
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
				gomega.Expect(updatedCanary.Status.CanaryWeight).Should(gomega.BeEquivalentTo(canaryWeight))
				if updatedCanary.Status.Phase >= flaggerv1.CanaryPhasePromoting {
					// we've finished the canary process
					break
				}
			}
			// check the canary phase is set to succeed
			gomega.Eventually(
				func() flaggerv1.CanaryPhase {
					objectKey := client.ObjectKey{
						Name:      canary.GetName(),
						Namespace: canary.GetNamespace(),
					}
					ctrl.advanceCanary(canary.Name, canary.Namespace)
					err := k8sClient.Get(context.TODO(), objectKey, &updatedCanary)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					return updatedCanary.Status.Phase
				}, time.Second*2, time.Millisecond*200).Should(gomega.BeEquivalentTo(flaggerv1.CanaryPhaseSucceeded))
		})

		It("Test search for source workload not following component rev", func() {
			// recreate one with a different name, the controller revision
			sourceName := "younameit"
			sourceResource, scr := getTestDeploymentWithRevision(sourceName, "4.0.6", componentName, sourceRevision)
			err := k8sClient.Create(context.TODO(), sourceResource)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			err = k8sClient.Create(context.TODO(), scr)
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			initializeController()
		})

	})
})
