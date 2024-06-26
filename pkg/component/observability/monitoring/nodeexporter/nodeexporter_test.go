// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package nodeexporter_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/component"
	. "github.com/gardener/gardener/pkg/component/observability/monitoring/nodeexporter"
	"github.com/gardener/gardener/pkg/resourcemanager/controller/garbagecollector/references"
	"github.com/gardener/gardener/pkg/utils/retry"
	retryfake "github.com/gardener/gardener/pkg/utils/retry/fake"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

var _ = Describe("NodeExporter", func() {
	var (
		ctx = context.TODO()

		managedResourceName = "shoot-core-node-exporter"
		namespace           = "some-namespace"
		image               = "some-image:some-tag"

		c         client.Client
		values    Values
		component component.DeployWaiter

		managedResource       *resourcesv1alpha1.ManagedResource
		managedResourceSecret *corev1.Secret
	)

	BeforeEach(func() {
		c = fakeclient.NewClientBuilder().WithScheme(kubernetes.SeedScheme).Build()
		values = Values{
			Image: image,
		}

		managedResource = &resourcesv1alpha1.ManagedResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      managedResourceName,
				Namespace: namespace,
			},
		}
		managedResourceSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "managedresource-" + managedResource.Name,
				Namespace: namespace,
			},
		}
	})

	Describe("#Deploy", func() {
		var (
			serviceAccountYAML = `apiVersion: v1
automountServiceAccountToken: false
kind: ServiceAccount
metadata:
  creationTimestamp: null
  labels:
    component: node-exporter
  name: node-exporter
  namespace: kube-system
`
			serviceYAML = `apiVersion: v1
kind: Service
metadata:
  creationTimestamp: null
  labels:
    component: node-exporter
  name: node-exporter
  namespace: kube-system
spec:
  clusterIP: None
  ports:
  - name: metrics
    port: 16909
    protocol: TCP
    targetPort: 0
  selector:
    component: node-exporter
  type: ClusterIP
status:
  loadBalancer: {}
`
			daemonSetYAML = `apiVersion: apps/v1
kind: DaemonSet
metadata:
  creationTimestamp: null
  labels:
    component: node-exporter
    gardener.cloud/role: monitoring
    origin: gardener
  name: node-exporter
  namespace: kube-system
spec:
  selector:
    matchLabels:
      component: node-exporter
  template:
    metadata:
      creationTimestamp: null
      labels:
        component: node-exporter
        gardener.cloud/role: monitoring
        networking.gardener.cloud/from-seed: allowed
        networking.gardener.cloud/to-public-networks: allowed
        origin: gardener
    spec:
      automountServiceAccountToken: false
      containers:
      - command:
        - /bin/node_exporter
        - --web.listen-address=:16909
        - --path.procfs=/host/proc
        - --path.sysfs=/host/sys
        - --path.rootfs=/host
        - --log.level=error
        - --collector.disable-defaults
        - --collector.conntrack
        - --collector.cpu
        - --collector.diskstats
        - --collector.filefd
        - --collector.filesystem
        - --collector.filesystem.mount-points-exclude=^/(run|var)/.+$|^/(boot|dev|sys|usr)($|/.+$)
        - --collector.loadavg
        - --collector.meminfo
        - --collector.uname
        - --collector.stat
        - --collector.pressure
        - --collector.textfile
        - --collector.textfile.directory=/textfile-collector
        image: some-image:some-tag
        imagePullPolicy: IfNotPresent
        livenessProbe:
          httpGet:
            path: /
            port: 16909
          initialDelaySeconds: 5
          timeoutSeconds: 5
        name: node-exporter
        ports:
        - containerPort: 16909
          hostPort: 16909
          name: scrape
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /
            port: 16909
          initialDelaySeconds: 5
          timeoutSeconds: 5
        resources:
          limits:
            memory: 250Mi
          requests:
            cpu: 50m
            memory: 50Mi
        volumeMounts:
        - mountPath: /host
          name: host
          readOnly: true
        - mountPath: /textfile-collector
          name: textfile
          readOnly: true
      hostNetwork: true
      hostPID: true
      priorityClassName: system-cluster-critical
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534
        seccompProfile:
          type: RuntimeDefault
      serviceAccountName: node-exporter
      tolerations:
      - effect: NoSchedule
        operator: Exists
      - effect: NoExecute
        operator: Exists
      volumes:
      - hostPath:
          path: /
        name: host
      - hostPath:
          path: /var/lib/node-exporter/textfile-collector
        name: textfile
  updateStrategy:
    type: RollingUpdate
status:
  currentNumberScheduled: 0
  desiredNumberScheduled: 0
  numberMisscheduled: 0
  numberReady: 0
`
			vpaYAML = `apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  creationTimestamp: null
  name: node-exporter
  namespace: kube-system
spec:
  resourcePolicy:
    containerPolicies:
    - containerName: '*'
      controlledValues: RequestsOnly
      minAllowed:
        memory: 50Mi
  targetRef:
    apiVersion: apps/v1
    kind: DaemonSet
    name: node-exporter
  updatePolicy:
    updateMode: Auto
status: {}
`
		)

		JustBeforeEach(func() {
			component = New(c, namespace, values)
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(BeNotFoundError())

			Expect(component.Deploy(ctx)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(Succeed())
			expectedMr := &resourcesv1alpha1.ManagedResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:            managedResource.Name,
					Namespace:       managedResource.Namespace,
					ResourceVersion: "1",
					Labels:          map[string]string{"origin": "gardener"},
				},
				Spec: resourcesv1alpha1.ManagedResourceSpec{
					InjectLabels: map[string]string{"shoot.gardener.cloud/no-cleanup": "true"},
					SecretRefs: []corev1.LocalObjectReference{{
						Name: managedResource.Spec.SecretRefs[0].Name,
					}},
					KeepObjects: ptr.To(false),
				},
			}
			utilruntime.Must(references.InjectAnnotations(expectedMr))
			Expect(managedResource).To(DeepEqual(expectedMr))

			managedResourceSecret.Name = managedResource.Spec.SecretRefs[0].Name
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(Succeed())
			Expect(managedResourceSecret.Type).To(Equal(corev1.SecretTypeOpaque))
			Expect(managedResourceSecret.Immutable).To(Equal(ptr.To(true)))
			Expect(managedResourceSecret.Labels["resources.gardener.cloud/garbage-collectable-reference"]).To(Equal("true"))
			Expect(string(managedResourceSecret.Data["serviceaccount__kube-system__node-exporter.yaml"])).To(Equal(serviceAccountYAML))
			Expect(string(managedResourceSecret.Data["service__kube-system__node-exporter.yaml"])).To(Equal(serviceYAML))
			Expect(string(managedResourceSecret.Data["daemonset__kube-system__node-exporter.yaml"])).To(Equal(daemonSetYAML))
		})

		Context("VPA disabled", func() {
			It("should successfully deploy all resources", func() {
				Expect(managedResourceSecret.Data).To(HaveLen(3))
			})
		})

		Context("VPA enabled", func() {
			BeforeEach(func() {
				values.VPAEnabled = true
			})

			It("should successfully deploy the VPA resource", func() {
				Expect(managedResourceSecret.Data).To(HaveLen(4))
				Expect(string(managedResourceSecret.Data["verticalpodautoscaler__kube-system__node-exporter.yaml"])).To(Equal(vpaYAML))
			})
		})
	})

	Describe("#Destroy", func() {
		It("should successfully destroy all resources", func() {
			component = New(c, namespace, values)
			Expect(c.Create(ctx, managedResource)).To(Succeed())
			Expect(c.Create(ctx, managedResourceSecret)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(Succeed())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(Succeed())

			Expect(component.Destroy(ctx)).To(Succeed())

			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResource), managedResource)).To(BeNotFoundError())
			Expect(c.Get(ctx, client.ObjectKeyFromObject(managedResourceSecret), managedResourceSecret)).To(BeNotFoundError())
		})
	})

	Context("waiting functions", func() {
		var (
			fakeOps   *retryfake.Ops
			resetVars func()
		)

		BeforeEach(func() {
			component = New(c, namespace, values)

			fakeOps = &retryfake.Ops{MaxAttempts: 1}
			resetVars = test.WithVars(
				&retry.Until, fakeOps.Until,
				&retry.UntilTimeout, fakeOps.UntilTimeout,
			)
		})

		AfterEach(func() {
			resetVars()
		})

		Describe("#Wait", func() {
			It("should fail because reading the ManagedResource fails", func() {
				Expect(component.Wait(ctx)).To(MatchError(ContainSubstring("not found")))
			})

			It("should fail because the ManagedResource doesn't become healthy", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, &resourcesv1alpha1.ManagedResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:       managedResourceName,
						Namespace:  namespace,
						Generation: 1,
					},
					Status: resourcesv1alpha1.ManagedResourceStatus{
						ObservedGeneration: 1,
						Conditions: []gardencorev1beta1.Condition{
							{
								Type:   resourcesv1alpha1.ResourcesApplied,
								Status: gardencorev1beta1.ConditionFalse,
							},
							{
								Type:   resourcesv1alpha1.ResourcesHealthy,
								Status: gardencorev1beta1.ConditionFalse,
							},
						},
					},
				})).To(Succeed())

				Expect(component.Wait(ctx)).To(MatchError(ContainSubstring("is not healthy")))
			})

			It("should successfully wait for the managed resource to become healthy", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, &resourcesv1alpha1.ManagedResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:       managedResourceName,
						Namespace:  namespace,
						Generation: 1,
					},
					Status: resourcesv1alpha1.ManagedResourceStatus{
						ObservedGeneration: 1,
						Conditions: []gardencorev1beta1.Condition{
							{
								Type:   resourcesv1alpha1.ResourcesApplied,
								Status: gardencorev1beta1.ConditionTrue,
							},
							{
								Type:   resourcesv1alpha1.ResourcesHealthy,
								Status: gardencorev1beta1.ConditionTrue,
							},
						},
					},
				})).To(Succeed())

				Expect(component.Wait(ctx)).To(Succeed())
			})
		})

		Describe("#WaitCleanup", func() {
			It("should fail when the wait for the managed resource deletion times out", func() {
				fakeOps.MaxAttempts = 2

				Expect(c.Create(ctx, managedResource)).To(Succeed())

				Expect(component.WaitCleanup(ctx)).To(MatchError(ContainSubstring("still exists")))
			})

			It("should not return an error when it's already removed", func() {
				Expect(component.WaitCleanup(ctx)).To(Succeed())
			})
		})
	})
})
