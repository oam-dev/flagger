package controller

import (
	"context"
	"math/rand"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	oamCore "github.com/crossplane/oam-kubernetes-runtime/apis/core"
	oamutil "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	"github.com/weaveworks/flagger/pkg/canary"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	informers "github.com/weaveworks/flagger/pkg/client/informers/externalversions"
	flog "github.com/weaveworks/flagger/pkg/logger"
	"github.com/weaveworks/flagger/pkg/metrics/observers"
	"github.com/weaveworks/flagger/pkg/notifier"
	"github.com/weaveworks/flagger/pkg/router"
	"github.com/weaveworks/flagger/pkg/version"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var canaryFactory *canary.Factory
var ctrl *Controller
var routerFactory *router.Factory
var tLog *zap.SugaredLogger
var oct_namespace string
var ns corev1.Namespace

func TestIntegrationTest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t,
		"OAM Flagger Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	var err error
	tLog, err = flog.NewLoggerWithEncoding("debug", "json")
	Expect(err).NotTo(HaveOccurred())
	rand.Seed(time.Now().UnixNano())
	oct_namespace = "oam-flagger-" + strconv.FormatInt(rand.Int63(), 16)
	By("Bootstrapping test environment")
	useExistCluster := true
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../../", "charts/flagger/crds"), // this has all the required CRDs,
		},
		UseExistingCluster: &useExistCluster,
	}
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("Add schemes")
	err = oamCore.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	kubeClient, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	// flagger client
	flaggerClient, err := clientset.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	By("Construct the flagger controller")
	newTestController(kubeClient, flaggerClient)
	By("Create the OAM controller test namespace")
	ns = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   oct_namespace,
			Labels: map[string]string{"app": "flagger-testing"},
		},
	}
	err = k8sClient.Create(context.TODO(), &ns)
	Expect(err).Should(SatisfyAny(BeNil(), &oamutil.AlreadyExistMatcher{}))
	By("Setup common resources")
	err = setupTestSecretVol()
	Expect(err).Should(SatisfyAny(BeNil(), &oamutil.AlreadyExistMatcher{}))
	err = setupTestConfigMapVol()
	Expect(err).Should(SatisfyAny(BeNil(), &oamutil.AlreadyExistMatcher{}))
}, 60)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	Expect(k8sClient.Delete(context.TODO(), &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(
		SatisfyAny(
			BeNil(), &oamutil.NotFoundMatcher{}))

	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
	tLog.Sync()
})

func newTestController(kubeClient *kubernetes.Clientset, flaggerClient clientset.Interface) {
	// init controller
	flaggerInformerFactory := informers.NewSharedInformerFactory(flaggerClient, 0)

	fi := Informers{
		CanaryInformer: flaggerInformerFactory.Flagger().V1beta1().Canaries(),
		MetricInformer: flaggerInformerFactory.Flagger().V1beta1().MetricTemplates(),
		AlertInformer:  flaggerInformerFactory.Flagger().V1beta1().AlertProviders(),
	}

	// init router
	meshClient, err := clientset.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	routerFactory = router.NewFactory(cfg, kubeClient, flaggerClient, "nginx.ingress.kubernetes.io", "", tLog, meshClient)

	// init observer
	observerFactory, _ := observers.NewFactory("")

	// init canary factory
	configTracker := &canary.ConfigTracker{
		Logger:        tLog,
		KubeClient:    kubeClient,
		FlaggerClient: flaggerClient,
	}
	canaryFactory = canary.NewFactory(cfg, kubeClient, flaggerClient, configTracker, []string{"app", "name"}, tLog)

	ctrl = NewController(
		kubeClient,
		flaggerClient,
		fi,
		10*time.Second,
		tLog,
		&notifier.NopNotifier{},
		canaryFactory,
		routerFactory,
		observerFactory,
		flaggerv1.OAMProvider,
		version.VERSION,
		"")
}
