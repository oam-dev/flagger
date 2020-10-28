module github.com/weaveworks/flagger

go 1.14

require (
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/aws/aws-sdk-go v1.31.9
	github.com/crossplane/oam-kubernetes-runtime v0.3.0-rc1.0.20201019050404-723f8ecf8444
	github.com/davecgh/go-spew v1.1.1
	github.com/google/go-cmp v0.5.2
	github.com/oam-dev/kubevela v0.0.8
	github.com/onsi/ginkgo v1.13.0
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.6.0
	github.com/stretchr/testify v1.6.1
	go.uber.org/zap v1.14.1
	gopkg.in/h2non/gock.v1 v1.0.15
	k8s.io/api v0.18.8
	k8s.io/apimachinery v0.18.8
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.18.8
	sigs.k8s.io/controller-runtime v0.6.3
)

replace (
    k8s.io/klog => github.com/stefanprodan/klog v0.0.0-20190418165334-9cbb78b20423
    k8s.io/client-go => k8s.io/client-go v0.18.6
)
