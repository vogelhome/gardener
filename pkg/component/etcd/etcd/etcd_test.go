// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Masterminds/semver/v3"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	hvpav1alpha1 "github.com/gardener/hvpa-controller/api/v1alpha1"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	vpaautoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	testclock "k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/gardener/gardener/pkg/component/etcd/etcd"
	"github.com/gardener/gardener/pkg/component/etcd/etcd/constants"
	"github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/gardener"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	secretsutils "github.com/gardener/gardener/pkg/utils/secrets"
	secretsmanager "github.com/gardener/gardener/pkg/utils/secrets/manager"
	fakesecretsmanager "github.com/gardener/gardener/pkg/utils/secrets/manager/fake"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/gardener/gardener/third_party/mock/client-go/rest"
	mockclient "github.com/gardener/gardener/third_party/mock/controller-runtime/client"
)

var _ = Describe("Etcd", func() {
	Describe("#ServiceName", func() {
		It("should return the expected service name", func() {
			Expect(constants.ServiceName(testRole)).To(Equal("etcd-" + testRole + "-client"))
		})
	})

	var (
		ctrl       *gomock.Controller
		c          *mockclient.MockClient
		fakeClient client.Client
		sm         secretsmanager.Interface
		etcd       Interface
		log        logr.Logger

		ctx                     = context.TODO()
		fakeErr                 = errors.New("fake err")
		class                   = ClassNormal
		replicas                = ptr.To[int32](1)
		storageCapacity         = "12Gi"
		storageCapacityQuantity = resource.MustParse(storageCapacity)
		storageClassName        = "my-storage-class"
		defragmentationSchedule = "abcd"
		priorityClassName       = "some-priority-class"

		secretNameCA         = "ca-etcd"
		secretNamePeerCA     = "ca-etcd-peer"
		secretNameServer     = "etcd-server-" + testRole
		secretNameServerPeer = "etcd-peer-server-" + testRole
		secretNameClient     = "etcd-client"

		maintenanceTimeWindow = gardencorev1beta1.MaintenanceTimeWindow{
			Begin: "1234",
			End:   "5678",
		}
		now                     = time.Time{}
		quota                   = resource.MustParse("8Gi")
		garbageCollectionPolicy = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
		garbageCollectionPeriod = metav1.Duration{Duration: 12 * time.Hour}
		compressionPolicy       = druidv1alpha1.GzipCompression
		compressionSpec         = druidv1alpha1.CompressionSpec{
			Enabled: ptr.To(true),
			Policy:  &compressionPolicy,
		}
		backupLeaderElectionEtcdConnectionTimeout = &metav1.Duration{Duration: 10 * time.Second}
		backupLeaderElectionReelectionPeriod      = &metav1.Duration{Duration: 11 * time.Second}

		updateModeAuto     = hvpav1alpha1.UpdateModeAuto
		containerPolicyOff = vpaautoscalingv1.ContainerScalingModeOff
		controlledValues   = vpaautoscalingv1.ContainerControlledValuesRequestsOnly
		metricsBasic       = druidv1alpha1.Basic
		metricsExtensive   = druidv1alpha1.Extensive

		etcdName = "etcd-" + testRole
		hvpaName = "etcd-" + testRole

		etcdObjFor = func(
			class Class,
			replicas int32,
			backupConfig *BackupConfig,
			existingDefragmentationSchedule,
			existingBackupSchedule string,
			existingResourcesContainerEtcd *corev1.ResourceRequirements,
			existingResourcesContainerBackupRestore *corev1.ResourceRequirements,
			caSecretName string,
			clientSecretName string,
			serverSecretName string,
			peerCASecretName *string,
			peerServerSecretName *string,
			topologyAwareRoutingEnabled bool,
		) *druidv1alpha1.Etcd {
			defragSchedule := defragmentationSchedule
			if existingDefragmentationSchedule != "" {
				defragSchedule = existingDefragmentationSchedule
			}

			resourcesContainerEtcd := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("1G"),
				},
			}
			if existingResourcesContainerEtcd != nil {
				resourcesContainerEtcd = existingResourcesContainerEtcd
			}

			resourcesContainerBackupRestore := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("23m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			}
			if existingResourcesContainerBackupRestore != nil {
				resourcesContainerBackupRestore = existingResourcesContainerBackupRestore
			}

			clientService := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"networking.resources.gardener.cloud/from-all-scrape-targets-allowed-ports": `[{"protocol":"TCP","port":2379},{"protocol":"TCP","port":8080}]`,
						"networking.resources.gardener.cloud/namespace-selectors":                   `[{"matchLabels":{"kubernetes.io/metadata.name":"garden"}}]`,
						"networking.resources.gardener.cloud/pod-label-selector-namespace-alias":    "all-shoots",
					},
				},
			}
			if topologyAwareRoutingEnabled {
				metav1.SetMetaDataAnnotation(&clientService.ObjectMeta, "service.kubernetes.io/topology-aware-hints", "auto")
				metav1.SetMetaDataLabel(&clientService.ObjectMeta, "endpoint-slice-hints.resources.gardener.cloud/consider", "true")
			}

			obj := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      etcdName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"gardener.cloud/operation": "reconcile",
						"gardener.cloud/timestamp": now.Format(time.RFC3339Nano),
					},
					Labels: map[string]string{
						"gardener.cloud/role": "controlplane",
						"role":                testRole,
					},
				},
				Spec: druidv1alpha1.EtcdSpec{
					Replicas:          replicas,
					PriorityClassName: &priorityClassName,
					Labels: map[string]string{
						"gardener.cloud/role":              "controlplane",
						"role":                             testRole,
						"app":                              "etcd-statefulset",
						"networking.gardener.cloud/to-dns": "allowed",
						"networking.gardener.cloud/to-public-networks":   "allowed",
						"networking.gardener.cloud/to-private-networks":  "allowed",
						"networking.gardener.cloud/to-runtime-apiserver": "allowed",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"gardener.cloud/role": "controlplane",
							"role":                testRole,
							"app":                 "etcd-statefulset",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{
						Resources: resourcesContainerEtcd,
						ClientUrlTLS: &druidv1alpha1.TLSConfig{
							TLSCASecretRef: druidv1alpha1.SecretReference{
								SecretReference: corev1.SecretReference{
									Name:      caSecretName,
									Namespace: testNamespace,
								},
								DataKey: ptr.To("bundle.crt"),
							},
							ServerTLSSecretRef: corev1.SecretReference{
								Name:      serverSecretName,
								Namespace: testNamespace,
							},
							ClientTLSSecretRef: corev1.SecretReference{
								Name:      clientSecretName,
								Namespace: testNamespace,
							},
						},
						ServerPort:              ptr.To[int32](2380),
						ClientPort:              ptr.To[int32](2379),
						Metrics:                 &metricsBasic,
						DefragmentationSchedule: &defragSchedule,
						Quota:                   &quota,
						ClientService: &druidv1alpha1.ClientService{
							Annotations: clientService.Annotations,
							Labels:      clientService.Labels,
						},
					},
					Backup: druidv1alpha1.BackupSpec{
						TLS: &druidv1alpha1.TLSConfig{
							TLSCASecretRef: druidv1alpha1.SecretReference{
								SecretReference: corev1.SecretReference{
									Name:      caSecretName,
									Namespace: testNamespace,
								},
								DataKey: ptr.To("bundle.crt"),
							},
							ServerTLSSecretRef: corev1.SecretReference{
								Name:      serverSecretName,
								Namespace: testNamespace,
							},
							ClientTLSSecretRef: corev1.SecretReference{
								Name:      clientSecretName,
								Namespace: testNamespace,
							},
						},
						Port:                    ptr.To[int32](8080),
						Resources:               resourcesContainerBackupRestore,
						GarbageCollectionPolicy: &garbageCollectionPolicy,
						GarbageCollectionPeriod: &garbageCollectionPeriod,
						SnapshotCompression:     &compressionSpec,
					},
					StorageCapacity:     &storageCapacityQuantity,
					StorageClass:        &storageClassName,
					VolumeClaimTemplate: ptr.To(etcdName),
				},
			}

			if class == ClassImportant {
				obj.Spec.Annotations = map[string]string{"cluster-autoscaler.kubernetes.io/safe-to-evict": "false"}
				obj.Spec.Etcd.Metrics = &metricsExtensive
				obj.Spec.VolumeClaimTemplate = ptr.To(testRole + "-etcd")
			}

			if replicas == 3 {
				obj.Spec.Labels = utils.MergeStringMaps(obj.Spec.Labels, map[string]string{
					"networking.resources.gardener.cloud/to-etcd-" + testRole + "-client-tcp-2379": "allowed",
					"networking.resources.gardener.cloud/to-etcd-" + testRole + "-client-tcp-2380": "allowed",
					"networking.resources.gardener.cloud/to-etcd-" + testRole + "-client-tcp-8080": "allowed",
				})
				obj.Spec.Etcd.PeerUrlTLS = &druidv1alpha1.TLSConfig{
					ServerTLSSecretRef: corev1.SecretReference{
						Name:      secretNameServerPeer,
						Namespace: testNamespace,
					},
					TLSCASecretRef: druidv1alpha1.SecretReference{
						SecretReference: corev1.SecretReference{
							Name:      secretNamePeerCA,
							Namespace: testNamespace,
						},
						DataKey: ptr.To(secretsutils.DataKeyCertificateBundle),
					},
				}
			}

			if ptr.Deref(peerServerSecretName, "") != "" {
				obj.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef = corev1.SecretReference{
					Name:      *peerServerSecretName,
					Namespace: testNamespace,
				}
			}

			if ptr.Deref(peerCASecretName, "") != "" {
				obj.Spec.Etcd.PeerUrlTLS.TLSCASecretRef = druidv1alpha1.SecretReference{
					SecretReference: corev1.SecretReference{
						Name:      *peerCASecretName,
						Namespace: testNamespace,
					},
					DataKey: ptr.To(secretsutils.DataKeyCertificateBundle),
				}
			}

			if backupConfig != nil {
				fullSnapshotSchedule := backupConfig.FullSnapshotSchedule
				if existingBackupSchedule != "" {
					fullSnapshotSchedule = existingBackupSchedule
				}

				provider := druidv1alpha1.StorageProvider(backupConfig.Provider)
				deltaSnapshotPeriod := metav1.Duration{Duration: 5 * time.Minute}
				deltaSnapshotMemoryLimit := resource.MustParse("100Mi")

				obj.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
					SecretRef: &corev1.SecretReference{Name: backupConfig.SecretRefName},
					Container: &backupConfig.Container,
					Provider:  &provider,
					Prefix:    backupConfig.Prefix + "/etcd-" + testRole,
				}
				obj.Spec.Backup.FullSnapshotSchedule = &fullSnapshotSchedule
				obj.Spec.Backup.DeltaSnapshotPeriod = &deltaSnapshotPeriod
				obj.Spec.Backup.DeltaSnapshotRetentionPeriod = &metav1.Duration{Duration: 15 * 24 * time.Hour}
				obj.Spec.Backup.DeltaSnapshotMemoryLimit = &deltaSnapshotMemoryLimit

				if backupConfig.LeaderElection != nil {
					obj.Spec.Backup.LeaderElection = &druidv1alpha1.LeaderElectionSpec{
						EtcdConnectionTimeout: backupLeaderElectionEtcdConnectionTimeout,
						ReelectionPeriod:      backupLeaderElectionReelectionPeriod,
					}
				}
			}

			return obj
		}
		hvpaFor = func(class Class, replicas int32, scaleDownUpdateMode string) *hvpav1alpha1.Hvpa {
			obj := &hvpav1alpha1.Hvpa{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hvpaName,
					Namespace: testNamespace,
					Labels: map[string]string{
						"gardener.cloud/role": "controlplane",
						"role":                testRole,
						"app":                 "etcd-statefulset",
					},
				},
				Spec: hvpav1alpha1.HvpaSpec{
					Replicas: ptr.To[int32](1),
					MaintenanceTimeWindow: &hvpav1alpha1.MaintenanceTimeWindow{
						Begin: maintenanceTimeWindow.Begin,
						End:   maintenanceTimeWindow.End,
					},
					Hpa: hvpav1alpha1.HpaSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"role": "etcd-hpa-" + testRole,
							},
						},
						Deploy: false,
						Template: hvpav1alpha1.HpaTemplate{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"role": "etcd-hpa-" + testRole,
								},
							},
							Spec: hvpav1alpha1.HpaTemplateSpec{
								MinReplicas: &replicas,
								MaxReplicas: replicas,
								Metrics: []autoscalingv2beta1.MetricSpec{
									{
										Type: autoscalingv2beta1.ResourceMetricSourceType,
										Resource: &autoscalingv2beta1.ResourceMetricSource{
											Name:                     corev1.ResourceCPU,
											TargetAverageUtilization: ptr.To[int32](80),
										},
									},
									{
										Type: autoscalingv2beta1.ResourceMetricSourceType,
										Resource: &autoscalingv2beta1.ResourceMetricSource{
											Name:                     corev1.ResourceMemory,
											TargetAverageUtilization: ptr.To[int32](80),
										},
									},
								},
							},
						},
					},
					Vpa: hvpav1alpha1.VpaSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"role": "etcd-vpa-" + testRole,
							},
						},
						Deploy: true,
						ScaleUp: hvpav1alpha1.ScaleType{
							UpdatePolicy: hvpav1alpha1.UpdatePolicy{
								UpdateMode: &updateModeAuto,
							},
							StabilizationDuration: ptr.To("5m"),
							MinChange: hvpav1alpha1.ScaleParams{
								CPU: hvpav1alpha1.ChangeParams{
									Value:      ptr.To("1"),
									Percentage: ptr.To[int32](80),
								},
								Memory: hvpav1alpha1.ChangeParams{
									Value:      ptr.To("2G"),
									Percentage: ptr.To[int32](80),
								},
							},
						},
						ScaleDown: hvpav1alpha1.ScaleType{
							UpdatePolicy: hvpav1alpha1.UpdatePolicy{
								UpdateMode: &scaleDownUpdateMode,
							},
							StabilizationDuration: ptr.To("15m"),
							MinChange: hvpav1alpha1.ScaleParams{
								CPU: hvpav1alpha1.ChangeParams{
									Value:      ptr.To("1"),
									Percentage: ptr.To[int32](80),
								},
								Memory: hvpav1alpha1.ChangeParams{
									Value:      ptr.To("2G"),
									Percentage: ptr.To[int32](80),
								},
							},
						},
						LimitsRequestsGapScaleParams: hvpav1alpha1.ScaleParams{
							CPU: hvpav1alpha1.ChangeParams{
								Value:      ptr.To("2"),
								Percentage: ptr.To[int32](40),
							},
							Memory: hvpav1alpha1.ChangeParams{
								Value:      ptr.To("5G"),
								Percentage: ptr.To[int32](40),
							},
						},
						Template: hvpav1alpha1.VpaTemplate{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"role": "etcd-vpa-" + testRole,
								},
							},
							Spec: hvpav1alpha1.VpaTemplateSpec{
								ResourcePolicy: &vpaautoscalingv1.PodResourcePolicy{
									ContainerPolicies: []vpaautoscalingv1.ContainerResourcePolicy{
										{
											ContainerName: "etcd",
											MinAllowed: corev1.ResourceList{
												corev1.ResourceMemory: resource.MustParse("200M"),
											},
											MaxAllowed: corev1.ResourceList{
												corev1.ResourceCPU:    resource.MustParse("4"),
												corev1.ResourceMemory: resource.MustParse("28G"),
											},
											ControlledValues: &controlledValues,
										},
										{
											ContainerName:    "backup-restore",
											Mode:             &containerPolicyOff,
											ControlledValues: &controlledValues,
										},
									},
								},
							},
						},
					},
					WeightBasedScalingIntervals: []hvpav1alpha1.WeightBasedScalingInterval{
						{
							VpaWeight:         hvpav1alpha1.VpaOnly,
							StartReplicaCount: replicas,
							LastReplicaCount:  replicas,
						},
					},
					TargetRef: &autoscalingv2beta1.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "StatefulSet",
						Name:       etcdName,
					},
				},
			}

			if class == ClassImportant {
				obj.Spec.Vpa.Template.Spec.ResourcePolicy.ContainerPolicies[0].MinAllowed = corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("700M"),
				}
			}

			return obj
		}
		serviceMonitor = &monitoringv1.ServiceMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "garden-virtual-garden-etcd-" + testRole,
				Namespace: testNamespace,
				Labels:    map[string]string{"prometheus": "garden"},
			},
			Spec: monitoringv1.ServiceMonitorSpec{
				Selector: metav1.LabelSelector{MatchLabels: map[string]string{
					"name":     "etcd",
					"instance": "virtual-garden-" + etcdName,
				}},
				Endpoints: []monitoringv1.Endpoint{
					{
						Port:   "client",
						Scheme: "https",
						TLSConfig: &monitoringv1.TLSConfig{SafeTLSConfig: monitoringv1.SafeTLSConfig{
							InsecureSkipVerify: true,
							Cert: monitoringv1.SecretOrConfigMap{Secret: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "etcd-client"},
								Key:                  secretsutils.DataKeyCertificate,
							}},
							KeySecret: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "etcd-client"},
								Key:                  secretsutils.DataKeyPrivateKey,
							},
						}},
						RelabelConfigs: []*monitoringv1.RelabelConfig{
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_service_label_instance"},
								TargetLabel:  "role",
							},
							{
								Action:      "replace",
								Replacement: "virtual-garden-etcd",
								TargetLabel: "job",
							},
						},
						MetricRelabelConfigs: []*monitoringv1.RelabelConfig{{
							Action: "labeldrop",
							Regex:  `^instance$`,
						}},
					},
					{
						Port:   "backuprestore",
						Scheme: "https",
						TLSConfig: &monitoringv1.TLSConfig{SafeTLSConfig: monitoringv1.SafeTLSConfig{
							InsecureSkipVerify: true,
							Cert: monitoringv1.SecretOrConfigMap{Secret: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "etcd-client"},
								Key:                  secretsutils.DataKeyCertificate,
							}},
							KeySecret: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "etcd-client"},
								Key:                  secretsutils.DataKeyPrivateKey,
							},
						}},
						RelabelConfigs: []*monitoringv1.RelabelConfig{
							{
								SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_service_label_instance"},
								TargetLabel:  "role",
							},

							{
								Action:      "replace",
								Replacement: "virtual-garden-etcd-backup",
								TargetLabel: "job",
							},
						},
						MetricRelabelConfigs: []*monitoringv1.RelabelConfig{
							{
								Action: "labeldrop",
								Regex:  `^instance$`,
							},
							{
								SourceLabels: []monitoringv1.LabelName{"__name__"},
								Action:       "keep",
								Regex:        `^(etcdbr_defragmentation_duration_seconds_.+|etcdbr_defragmentation_duration_seconds_count|etcdbr_defragmentation_duration_seconds_sum|etcdbr_network_received_bytes|etcdbr_network_transmitted_bytes|etcdbr_restoration_duration_seconds_.+|etcdbr_restoration_duration_seconds_count|etcdbr_restoration_duration_seconds_sum|etcdbr_snapshot_duration_seconds_.+|etcdbr_snapshot_duration_seconds_count|etcdbr_snapshot_duration_seconds_sum|etcdbr_snapshot_gc_total|etcdbr_snapshot_latest_revision|etcdbr_snapshot_latest_timestamp|etcdbr_snapshot_required|etcdbr_validation_duration_seconds_.+|etcdbr_validation_duration_seconds_count|etcdbr_validation_duration_seconds_sum|process_resident_memory_bytes|process_cpu_seconds_total)$`,
							},
						},
					},
				},
			},
		}
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		c = mockclient.NewMockClient(ctrl)
		fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetesscheme.Scheme).Build()
		sm = fakesecretsmanager.New(fakeClient, testNamespace)
		log = logr.Discard()

		By("Create secrets managed outside of this package for whose secretsmanager.Get() will be called")
		Expect(fakeClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ca-etcd", Namespace: testNamespace}})).To(Succeed())
		etcd = New(log, c, testNamespace, sm, Values{
			Role:                    testRole,
			Class:                   class,
			Replicas:                replicas,
			StorageCapacity:         storageCapacity,
			StorageClassName:        &storageClassName,
			DefragmentationSchedule: &defragmentationSchedule,
			CARotationPhase:         "",
			PriorityClassName:       priorityClassName,
		})
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("#Deploy", func() {
		var scaleDownUpdateMode = hvpav1alpha1.UpdateModeMaintenanceWindow
		newSetHVPAConfigFunc := func(updateMode string) func() {
			return func() {
				etcd.SetHVPAConfig(&HVPAConfig{
					Enabled:               true,
					MaintenanceTimeWindow: maintenanceTimeWindow,
					ScaleDownUpdateMode:   ptr.To(updateMode),
				})
			}
		}
		setHVPAConfig := newSetHVPAConfigFunc(scaleDownUpdateMode)

		BeforeEach(setHVPAConfig)

		It("should fail because the etcd object retrieval fails", func() {
			c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(fakeErr)

			Expect(etcd.Deploy(ctx)).To(MatchError(fakeErr))
		})

		It("should fail because the statefulset object retrieval fails (using the default sts name)", func() {
			c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, ""))
			c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(fakeErr)

			Expect(etcd.Deploy(ctx)).To(MatchError(fakeErr))
		})

		It("should fail because the statefulset object retrieval fails (using the sts name from etcd object)", func() {
			statefulSetName := "sts-name"

			c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					(&druidv1alpha1.Etcd{
						Status: druidv1alpha1.EtcdStatus{
							Etcd: &druidv1alpha1.CrossVersionObjectReference{
								Name: statefulSetName,
							},
						},
					}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					return nil
				},
			)
			c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, statefulSetName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(fakeErr)

			Expect(etcd.Deploy(ctx)).To(MatchError(fakeErr))
		})

		It("should fail because the etcd cannot be created", func() {
			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Return(fakeErr),
			)

			Expect(etcd.Deploy(ctx)).To(MatchError(fakeErr))
		})

		It("should fail because the hvpa cannot be created", func() {
			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Return(fakeErr),
			)

			Expect(etcd.Deploy(ctx)).To(MatchError(fakeErr))
		})

		It("should fail because the hvpa cannot be deleted", func() {
			etcd.SetHVPAConfig(&HVPAConfig{})

			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()),
				c.EXPECT().Delete(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})).Return(fakeErr),
			)

			Expect(etcd.Deploy(ctx)).To(MatchError(fakeErr))
		})

		It("should successfully deploy (normal etcd)", func() {
			oldTimeNow := TimeNow
			defer func() { TimeNow = oldTimeNow }()
			TimeNow = func() time.Time { return now }

			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(etcdObjFor(
						class,
						1,
						nil,
						"",
						"",
						nil,
						nil,
						secretNameCA,
						secretNameClient,
						secretNameServer,
						nil,
						nil,
						false)))
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(hvpaFor(class, 1, scaleDownUpdateMode)))
				}),
			)

			Expect(etcd.Deploy(ctx)).To(Succeed())
		})

		It("should not panic during deploy when etcd resource exists, but its status is not yet populated", func() {
			oldTimeNow := TimeNow
			defer func() { TimeNow = oldTimeNow }()
			TimeNow = func() time.Time { return now }

			var (
				existingReplicas int32 = 245
			)

			etcd = New(log, c, testNamespace, sm, Values{
				Role:                    testRole,
				Class:                   class,
				Replicas:                nil,
				StorageCapacity:         storageCapacity,
				StorageClassName:        &storageClassName,
				DefragmentationSchedule: &defragmentationSchedule,
				CARotationPhase:         "",
				PriorityClassName:       priorityClassName,
			})
			setHVPAConfig()

			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					(&druidv1alpha1.Etcd{
						ObjectMeta: metav1.ObjectMeta{
							Name:      etcdName,
							Namespace: testNamespace,
						},
						Spec: druidv1alpha1.EtcdSpec{
							Replicas: existingReplicas,
						},
					}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					return nil
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj *druidv1alpha1.Etcd, _ client.Patch, _ ...client.PatchOption) {
					// ignore status when comparing
					obj.Status = druidv1alpha1.EtcdStatus{}

					Expect(obj).To(DeepEqual(etcdObjFor(
						class,
						existingReplicas,
						nil,
						"",
						"",
						nil,
						nil,
						secretNameCA,
						secretNameClient,
						secretNameServer,
						nil,
						nil,
						false)))
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(hvpaFor(class, existingReplicas, scaleDownUpdateMode)))
				}),
			)

			Expect(etcd.Deploy(ctx)).To(Succeed())
		})

		It("should successfully deploy (normal etcd) and retain replicas (etcd found)", func() {
			oldTimeNow := TimeNow
			defer func() { TimeNow = oldTimeNow }()
			TimeNow = func() time.Time { return now }

			var (
				existingReplicas int32 = 245
			)

			etcd = New(log, c, testNamespace, sm, Values{
				Role:                    testRole,
				Class:                   class,
				Replicas:                nil,
				StorageCapacity:         storageCapacity,
				StorageClassName:        &storageClassName,
				DefragmentationSchedule: &defragmentationSchedule,
				CARotationPhase:         "",
				PriorityClassName:       priorityClassName,
			})
			setHVPAConfig()

			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					(&druidv1alpha1.Etcd{
						ObjectMeta: metav1.ObjectMeta{
							Name:      etcdName,
							Namespace: testNamespace,
						},
						Spec: druidv1alpha1.EtcdSpec{
							Replicas: existingReplicas,
						},
						Status: druidv1alpha1.EtcdStatus{
							Etcd: &druidv1alpha1.CrossVersionObjectReference{
								Name: etcdName,
							},
						},
					}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					return nil
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj *druidv1alpha1.Etcd, _ client.Patch, _ ...client.PatchOption) {
					// ignore status when comparing
					obj.Status = druidv1alpha1.EtcdStatus{}

					Expect(obj).To(DeepEqual(etcdObjFor(
						class,
						existingReplicas,
						nil,
						"",
						"",
						nil,
						nil,
						secretNameCA,
						secretNameClient,
						secretNameServer,
						nil,
						nil,
						false)))
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(hvpaFor(class, existingReplicas, scaleDownUpdateMode)))
				}),
			)

			Expect(etcd.Deploy(ctx)).To(Succeed())
		})

		It("should successfully deploy (normal etcd) and retain annotations (etcd found)", func() {
			oldTimeNow := TimeNow
			defer func() { TimeNow = oldTimeNow }()
			TimeNow = func() time.Time { return now }

			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					(&druidv1alpha1.Etcd{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								"foo": "bar",
							},
							Name:      etcdName,
							Namespace: testNamespace,
						},
						Status: druidv1alpha1.EtcdStatus{
							Etcd: &druidv1alpha1.CrossVersionObjectReference{
								Name: etcdName,
							},
						},
					}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					return nil
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj *druidv1alpha1.Etcd, _ client.Patch, _ ...client.PatchOption) {
					// ignore status when comparing
					obj.Status = druidv1alpha1.EtcdStatus{}

					expectedObj := etcdObjFor(
						class,
						1,
						nil,
						"",
						"",
						nil,
						nil,
						secretNameCA,
						secretNameClient,
						secretNameServer,
						nil,
						nil,
						false)
					expectedObj.Annotations = utils.MergeStringMaps(expectedObj.Annotations, map[string]string{
						"foo": "bar",
					})

					Expect(obj).To(DeepEqual(expectedObj))
				}),

				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(hvpaFor(class, 1, scaleDownUpdateMode)))
				}),
			)

			Expect(etcd.Deploy(ctx)).To(Succeed())
		})

		It("should successfully deploy (normal etcd) and keep the existing defragmentation schedule", func() {
			oldTimeNow := TimeNow
			defer func() { TimeNow = oldTimeNow }()
			TimeNow = func() time.Time { return now }

			existingDefragmentationSchedule := "foobardefragexisting"

			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					(&druidv1alpha1.Etcd{
						ObjectMeta: metav1.ObjectMeta{
							Name:      etcdName,
							Namespace: testNamespace,
						},
						Spec: druidv1alpha1.EtcdSpec{
							Etcd: druidv1alpha1.EtcdConfig{
								DefragmentationSchedule: &existingDefragmentationSchedule,
							},
						},
						Status: druidv1alpha1.EtcdStatus{
							Etcd: &druidv1alpha1.CrossVersionObjectReference{
								Name: etcdName,
							},
						},
					}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					return nil
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj *druidv1alpha1.Etcd, _ client.Patch, _ ...client.PatchOption) {
					// ignore status when comparing
					obj.Status = druidv1alpha1.EtcdStatus{}

					Expect(obj).To(DeepEqual(etcdObjFor(
						class,
						1,
						nil,
						existingDefragmentationSchedule,
						"",
						nil,
						nil,
						secretNameCA,
						secretNameClient,
						secretNameServer,
						nil,
						nil,
						false)))
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(hvpaFor(class, 1, scaleDownUpdateMode)))
				}),
			)

			Expect(etcd.Deploy(ctx)).To(Succeed())
		})

		It("should successfully deploy (normal etcd) and keep the existing resource request settings (but not limits) to not interfer with HVPA controller", func() {
			oldTimeNow := TimeNow
			defer func() { TimeNow = oldTimeNow }()
			TimeNow = func() time.Time { return now }

			var (
				existingResourcesContainerEtcd = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2G"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("3"),
						corev1.ResourceMemory: resource.MustParse("4G"),
					},
				}
				existingResourcesContainerBackupRestore = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("5"),
						corev1.ResourceMemory: resource.MustParse("6G"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("7"),
						corev1.ResourceMemory: resource.MustParse("8G"),
					},
				}

				expectedResourcesContainerEtcd = corev1.ResourceRequirements{
					Requests: existingResourcesContainerEtcd.Requests,
				}
				expectedResourcesContainerBackupRestore = corev1.ResourceRequirements{
					Requests: existingResourcesContainerBackupRestore.Requests,
				}
			)

			gomock.InOrder(
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					(&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      etcdName,
							Namespace: testNamespace,
						},
						Spec: appsv1.StatefulSetSpec{
							Template: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name:      "etcd",
											Resources: existingResourcesContainerEtcd,
										},
										{
											Name:      "backup-restore",
											Resources: existingResourcesContainerBackupRestore,
										},
									},
								},
							},
						},
					}).DeepCopyInto(obj.(*appsv1.StatefulSet))
					return nil
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(etcdObjFor(
						class,
						1,
						nil,
						"",
						"",
						&expectedResourcesContainerEtcd,
						&expectedResourcesContainerBackupRestore,
						secretNameCA,
						secretNameClient,
						secretNameServer,
						nil,
						nil,
						false)))
				}),
				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
					Expect(obj).To(DeepEqual(hvpaFor(class, 1, scaleDownUpdateMode)))
				}),
			)

			Expect(etcd.Deploy(ctx)).To(Succeed())
		})

		for _, shootPurpose := range []gardencorev1beta1.ShootPurpose{gardencorev1beta1.ShootPurposeEvaluation, gardencorev1beta1.ShootPurposeProduction} {
			var purpose = shootPurpose
			It(fmt.Sprintf("should successfully deploy (important etcd): purpose = %q", purpose), func() {
				oldTimeNow := TimeNow
				defer func() { TimeNow = oldTimeNow }()
				TimeNow = func() time.Time { return now }

				class := ClassImportant

				updateMode := hvpav1alpha1.UpdateModeMaintenanceWindow
				if purpose == gardencorev1beta1.ShootPurposeProduction {
					updateMode = hvpav1alpha1.UpdateModeOff
				}

				etcd = New(log, c, testNamespace, sm, Values{
					Role:                    testRole,
					Class:                   class,
					Replicas:                replicas,
					StorageCapacity:         storageCapacity,
					StorageClassName:        &storageClassName,
					DefragmentationSchedule: &defragmentationSchedule,
					CARotationPhase:         "",
					PriorityClassName:       priorityClassName,
				})
				newSetHVPAConfigFunc(updateMode)()

				gomock.InOrder(
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(etcdObjFor(
							class,
							1,
							nil,
							"",
							"",
							nil,
							nil,
							secretNameCA,
							secretNameClient,
							secretNameServer,
							nil,
							nil,
							false)))
					}),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(hvpaFor(class, 1, updateMode)))
					}),
				)

				Expect(etcd.Deploy(ctx)).To(Succeed())
			})
		}

		When("backup is configured", func() {
			var backupConfig = &BackupConfig{
				Provider:                     "prov",
				SecretRefName:                "secret-key",
				Prefix:                       "prefix",
				Container:                    "bucket",
				FullSnapshotSchedule:         "1234",
				DeltaSnapshotRetentionPeriod: &metav1.Duration{Duration: 15 * 24 * time.Hour},
			}

			BeforeEach(func() {
				etcd.SetBackupConfig(backupConfig)
			})

			It("should successfully deploy (with backup)", func() {
				oldTimeNow := TimeNow
				defer func() { TimeNow = oldTimeNow }()
				TimeNow = func() time.Time { return now }

				gomock.InOrder(
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(etcdObjFor(
							class,
							1,
							backupConfig,
							"",
							"",
							nil,
							nil,
							secretNameCA,
							secretNameClient,
							secretNameServer,
							nil,
							nil,
							false)))
					}),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(hvpaFor(class, 1, scaleDownUpdateMode)))
					}),
				)

				Expect(etcd.Deploy(ctx)).To(Succeed())
			})

			It("should successfully deploy (with backup) and keep the existing backup schedule", func() {
				oldTimeNow := TimeNow
				defer func() { TimeNow = oldTimeNow }()
				TimeNow = func() time.Time { return now }

				existingBackupSchedule := "foobarbackupexisting"

				gomock.InOrder(
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
						(&druidv1alpha1.Etcd{
							ObjectMeta: metav1.ObjectMeta{
								Name:      etcdName,
								Namespace: testNamespace,
							},
							Spec: druidv1alpha1.EtcdSpec{
								Backup: druidv1alpha1.BackupSpec{
									FullSnapshotSchedule: &existingBackupSchedule,
								},
							},
							Status: druidv1alpha1.EtcdStatus{
								Etcd: &druidv1alpha1.CrossVersionObjectReference{
									Name: "",
								},
							},
						}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
						return nil
					}),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						expobj := etcdObjFor(
							class,
							1,
							backupConfig,
							"",
							existingBackupSchedule,
							nil,
							nil,
							secretNameCA,
							secretNameClient,
							secretNameServer,
							nil,
							nil,
							false)
						expobj.Status.Etcd = &druidv1alpha1.CrossVersionObjectReference{}

						Expect(obj).To(DeepEqual(expobj))
					}),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(hvpaFor(class, 1, scaleDownUpdateMode)))
					}),
				)

				Expect(etcd.Deploy(ctx)).To(Succeed())
			})
		})

		When("HA setup is configured", func() {
			var (
				rotationPhase gardencorev1beta1.CredentialsRotationPhase
			)

			createExpectations := func(caSecretName, clientSecretName, serverSecretName, peerCASecretName, peerServerSecretName string) {
				gomock.InOrder(
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
						func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
							if peerServerSecretName != "" {
								etcd.Spec.Etcd.PeerUrlTLS = &druidv1alpha1.TLSConfig{
									ServerTLSSecretRef: corev1.SecretReference{
										Name:      peerServerSecretName,
										Namespace: testNamespace,
									},
								}
							}
							return nil
						}),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(etcdObjFor(
							class,
							3,
							nil,
							"",
							"",
							nil,
							nil,
							caSecretName,
							clientSecretName,
							serverSecretName,
							&peerCASecretName,
							&peerServerSecretName,
							false,
						)))
					}),
					c.EXPECT().Delete(ctx, &hvpav1alpha1.Hvpa{ObjectMeta: metav1.ObjectMeta{Name: "etcd-" + testRole, Namespace: testNamespace}}),
				)
			}

			BeforeEach(func() {
				Expect(fakeClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ca-etcd-peer", Namespace: testNamespace}})).To(Succeed())
			})

			JustBeforeEach(func() {
				replicas = ptr.To[int32](3)
				etcd = New(log, c, testNamespace, sm, Values{
					Role:                    testRole,
					Class:                   class,
					Replicas:                replicas,
					StorageCapacity:         storageCapacity,
					StorageClassName:        &storageClassName,
					DefragmentationSchedule: &defragmentationSchedule,
					CARotationPhase:         rotationPhase,
					PriorityClassName:       priorityClassName,
					HighAvailabilityEnabled: true,
				})
			})

			Context("when CA rotation phase is in `Preparing` state", func() {
				var (
					clientCASecret *corev1.Secret
					peerCASecret   *corev1.Secret
				)

				BeforeEach(func() {
					rotationPhase = gardencorev1beta1.RotationPreparing

					secretNamesToTimes := map[string]time.Time{}

					// A "real" SecretsManager is needed here because in further tests we want to differentiate
					// between what was issued by the old and new CAs.
					var err error
					sm, err = secretsmanager.New(
						ctx,
						logr.New(logf.NullLogSink{}),
						testclock.NewFakeClock(time.Now()),
						fakeClient,
						testNamespace,
						"",
						secretsmanager.Config{
							SecretNamesToTimes: secretNamesToTimes,
						})
					Expect(err).ToNot(HaveOccurred())

					// Create new etcd CA
					_, err = sm.Generate(ctx,
						&secretsutils.CertificateSecretConfig{Name: v1beta1constants.SecretNameCAETCD, CommonName: "etcd", CertType: secretsutils.CACert})
					Expect(err).ToNot(HaveOccurred())

					// Create new peer CA
					_, err = sm.Generate(ctx,
						&secretsutils.CertificateSecretConfig{Name: v1beta1constants.SecretNameCAETCDPeer, CommonName: "etcd-peer", CertType: secretsutils.CACert})
					Expect(err).ToNot(HaveOccurred())

					// Set time to trigger CA rotation
					secretNamesToTimes[v1beta1constants.SecretNameCAETCDPeer] = now

					// Rotate CA
					_, err = sm.Generate(ctx,
						&secretsutils.CertificateSecretConfig{Name: v1beta1constants.SecretNameCAETCDPeer, CommonName: "etcd-peer", CertType: secretsutils.CACert},
						secretsmanager.Rotate(secretsmanager.KeepOld))
					Expect(err).ToNot(HaveOccurred())

					var ok bool
					clientCASecret, ok = sm.Get(v1beta1constants.SecretNameCAETCD)
					Expect(ok).To(BeTrue())

					peerCASecret, ok = sm.Get(v1beta1constants.SecretNameCAETCDPeer)
					Expect(ok).To(BeTrue())

					DeferCleanup(func() {
						rotationPhase = ""
					})
				})

				It("should successfully deploy", func() {
					oldTimeNow := TimeNow
					defer func() { TimeNow = oldTimeNow }()
					TimeNow = func() time.Time { return now }

					peerServerSecret, err := sm.Generate(ctx, &secretsutils.CertificateSecretConfig{
						Name:       "etcd-peer-server-" + testRole,
						CommonName: "etcd-server",
						DNSNames: []string{
							"etcd-" + testRole + "-peer",
							"etcd-" + testRole + "-peer.shoot--test--test",
							"etcd-" + testRole + "-peer.shoot--test--test.svc",
							"etcd-" + testRole + "-peer.shoot--test--test.svc.cluster.local",
							"*.etcd-" + testRole + "-peer",
							"*.etcd-" + testRole + "-peer.shoot--test--test",
							"*.etcd-" + testRole + "-peer.shoot--test--test.svc",
							"*.etcd-" + testRole + "-peer.shoot--test--test.svc.cluster.local",
						},
						CertType:                    secretsutils.ServerClientCert,
						SkipPublishingCACertificate: true,
					}, secretsmanager.SignedByCA(v1beta1constants.SecretNameCAETCDPeer, secretsmanager.UseCurrentCA), secretsmanager.Rotate(secretsmanager.InPlace))
					Expect(err).ToNot(HaveOccurred())

					clientSecret, err := sm.Generate(ctx, &secretsutils.CertificateSecretConfig{
						Name:                        SecretNameClient,
						CommonName:                  "etcd-client",
						CertType:                    secretsutils.ClientCert,
						SkipPublishingCACertificate: true,
					}, secretsmanager.SignedByCA(v1beta1constants.SecretNameCAETCD), secretsmanager.Rotate(secretsmanager.InPlace))
					Expect(err).ToNot(HaveOccurred())

					serverSecret, err := sm.Generate(ctx, &secretsutils.CertificateSecretConfig{
						Name:       "etcd-server-" + testRole,
						CommonName: "etcd-server",
						DNSNames: []string{
							"etcd-" + testRole + "-local",
							"etcd-" + testRole + "-client",
							"etcd-" + testRole + "-client.shoot--test--test",
							"etcd-" + testRole + "-client.shoot--test--test.svc",
							"etcd-" + testRole + "-client.shoot--test--test.svc.cluster.local",
							"*.etcd-" + testRole + "-peer",
							"*.etcd-" + testRole + "-peer.shoot--test--test",
							"*.etcd-" + testRole + "-peer.shoot--test--test.svc",
							"*.etcd-" + testRole + "-peer.shoot--test--test.svc.cluster.local",
						},
						CertType:                    secretsutils.ServerClientCert,
						SkipPublishingCACertificate: true,
					}, secretsmanager.SignedByCA(v1beta1constants.SecretNameCAETCD), secretsmanager.Rotate(secretsmanager.InPlace))
					Expect(err).ToNot(HaveOccurred())

					createExpectations(clientCASecret.Name, clientSecret.Name, serverSecret.Name, peerCASecret.Name, peerServerSecret.Name)

					Expect(etcd.Deploy(ctx)).To(Succeed())
				})
			})
		})

		When("etcd cluster is hibernated", func() {
			BeforeEach(func() {
				secretNamesToTimes := map[string]time.Time{}

				var err error
				sm, err = secretsmanager.New(
					ctx,
					logr.New(logf.NullLogSink{}),
					testclock.NewFakeClock(time.Now()),
					fakeClient,
					testNamespace,
					"",
					secretsmanager.Config{
						SecretNamesToTimes: secretNamesToTimes,
					})
				Expect(err).ToNot(HaveOccurred())

				// Create new etcd CA
				_, err = sm.Generate(ctx,
					&secretsutils.CertificateSecretConfig{Name: v1beta1constants.SecretNameCAETCD, CommonName: "etcd", CertType: secretsutils.CACert})
				Expect(err).ToNot(HaveOccurred())

				// Create new peer CA
				_, err = sm.Generate(ctx,
					&secretsutils.CertificateSecretConfig{Name: v1beta1constants.SecretNameCAETCDPeer, CommonName: "etcd-peer", CertType: secretsutils.CACert})
				Expect(err).ToNot(HaveOccurred())
			})

			JustBeforeEach(func() {
				etcd = New(log, c, testNamespace, sm, Values{
					Role:                    testRole,
					Class:                   class,
					Replicas:                ptr.To[int32](0),
					StorageCapacity:         storageCapacity,
					StorageClassName:        &storageClassName,
					DefragmentationSchedule: &defragmentationSchedule,
					CARotationPhase:         gardencorev1beta1.RotationCompleted,
					PriorityClassName:       priorityClassName,
					HighAvailabilityEnabled: true,
				})
			})

			Context("when peer url secrets are present in etcd CR", func() {
				It("should not remove peer URL secrets", func() {
					gomock.InOrder(
						c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
							(&druidv1alpha1.Etcd{
								ObjectMeta: metav1.ObjectMeta{
									Name:      etcdName,
									Namespace: testNamespace,
								},
								Spec: druidv1alpha1.EtcdSpec{
									Replicas: 3,
									Etcd: druidv1alpha1.EtcdConfig{
										PeerUrlTLS: &druidv1alpha1.TLSConfig{
											ServerTLSSecretRef: corev1.SecretReference{
												Name:      "peerServerSecretName",
												Namespace: testNamespace,
											},
										},
									},
								},
							}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
							return nil
						}),
						c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
						c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
								etcd.Spec.Etcd.PeerUrlTLS = &druidv1alpha1.TLSConfig{
									ServerTLSSecretRef: corev1.SecretReference{
										Name:      "peerServerSecretName",
										Namespace: testNamespace,
									},
								}
								return nil
							}),
						c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
							Expect(obj.(*druidv1alpha1.Etcd).Spec.Replicas).To(Equal(int32(0)))
							Expect(obj.(*druidv1alpha1.Etcd).Spec.Etcd.PeerUrlTLS).NotTo(BeNil())
						}),
						c.EXPECT().Delete(ctx, &hvpav1alpha1.Hvpa{ObjectMeta: metav1.ObjectMeta{Name: "etcd-" + testRole, Namespace: testNamespace}}),
					)

					Expect(etcd.Deploy(ctx)).To(Succeed())
				})
			})

			Context("when peer url secrets are not present in etcd CR", func() {
				It("should add peer url secrets", func() {
					gomock.InOrder(
						c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
							(&druidv1alpha1.Etcd{
								ObjectMeta: metav1.ObjectMeta{
									Name:      etcdName,
									Namespace: testNamespace,
								},
								Spec: druidv1alpha1.EtcdSpec{
									Replicas: 3,
									Etcd: druidv1alpha1.EtcdConfig{
										PeerUrlTLS: nil,
									},
								},
							}).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
							return nil
						}),
						c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
						c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, _ *druidv1alpha1.Etcd, _ ...client.GetOption) error {
								return nil
							}),
						c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
							Expect(obj.(*druidv1alpha1.Etcd).Spec.Replicas).To(Equal(int32(0)))
							Expect(obj.(*druidv1alpha1.Etcd).Spec.Etcd.PeerUrlTLS).NotTo(BeNil())
						}),
						c.EXPECT().Delete(ctx, &hvpav1alpha1.Hvpa{ObjectMeta: metav1.ObjectMeta{Name: "etcd-" + testRole, Namespace: testNamespace}}),
					)

					Expect(etcd.Deploy(ctx)).To(Succeed())
				})
			})
		})

		When("TopologyAwareRoutingEnabled=true", func() {
			It("should successfully deploy with expected etcd client service annotations and labels", func() {
				oldTimeNow := TimeNow
				defer func() { TimeNow = oldTimeNow }()
				TimeNow = func() time.Time { return now }

				class := ClassImportant
				updateMode := hvpav1alpha1.UpdateModeMaintenanceWindow

				replicas = ptr.To[int32](1)

				etcd = New(log, c, testNamespace, sm, Values{
					Role:                        testRole,
					Class:                       class,
					Replicas:                    replicas,
					StorageCapacity:             storageCapacity,
					StorageClassName:            &storageClassName,
					DefragmentationSchedule:     &defragmentationSchedule,
					CARotationPhase:             "",
					RuntimeKubernetesVersion:    semver.MustParse("1.26.1"),
					PriorityClassName:           priorityClassName,
					TopologyAwareRoutingEnabled: true,
				})
				newSetHVPAConfigFunc(updateMode)()

				gomock.InOrder(
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(etcdObjFor(
							class,
							1,
							nil,
							"",
							"",
							nil,
							nil,
							secretNameCA,
							secretNameClient,
							secretNameServer,
							nil,
							nil,
							true)))
					}),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(hvpaFor(class, 1, updateMode)))
					}),
				)

				Expect(etcd.Deploy(ctx)).To(Succeed())
			})
		})

		When("name prefix is set", func() {
			It("should successfully deploy the service monitor", func() {
				oldTimeNow := TimeNow
				defer func() { TimeNow = oldTimeNow }()
				TimeNow = func() time.Time { return now }

				class := ClassImportant
				updateMode := hvpav1alpha1.UpdateModeMaintenanceWindow

				replicas = ptr.To[int32](1)

				etcd = New(log, c, testNamespace, sm, Values{
					Role:                        testRole,
					Class:                       class,
					Replicas:                    replicas,
					StorageCapacity:             storageCapacity,
					StorageClassName:            &storageClassName,
					DefragmentationSchedule:     &defragmentationSchedule,
					CARotationPhase:             "",
					RuntimeKubernetesVersion:    semver.MustParse("1.26.1"),
					PriorityClassName:           priorityClassName,
					TopologyAwareRoutingEnabled: true,
					NamePrefix:                  "virtual-garden-",
				})
				newSetHVPAConfigFunc(updateMode)()

				DeferCleanup(test.WithVar(&etcdName, "virtual-garden-"+etcdName))
				DeferCleanup(test.WithVar(&hvpaName, "virtual-garden-"+hvpaName))

				etcdObj := etcdObjFor(
					class,
					1,
					nil,
					"",
					"",
					nil,
					nil,
					secretNameCA,
					secretNameClient,
					secretNameServer,
					nil,
					nil,
					true,
				)
				etcdObj.Name = etcdName
				etcdObj.Spec.VolumeClaimTemplate = ptr.To(testRole + "-virtual-garden-etcd")
				etcdObj.Spec.Etcd.ClientService.Annotations["networking.resources.gardener.cloud/from-all-garden-scrape-targets-allowed-ports"] = `[{"protocol":"TCP","port":2379},{"protocol":"TCP","port":8080}]`
				delete(etcdObj.Spec.Etcd.ClientService.Annotations, "networking.resources.gardener.cloud/from-all-scrape-targets-allowed-ports")
				delete(etcdObj.Spec.Etcd.ClientService.Annotations, "networking.resources.gardener.cloud/namespace-selectors")
				delete(etcdObj.Spec.Etcd.ClientService.Annotations, "networking.resources.gardener.cloud/pod-label-selector-namespace-alias")

				hvpaObj := hvpaFor(class, 1, updateMode)
				hvpaObj.Name = hvpaName

				gomock.InOrder(
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&appsv1.StatefulSet{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(etcdObj))
					}),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, hvpaName), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(hvpaObj))
					}),
					c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, "garden-virtual-garden-etcd-main"), gomock.AssignableToTypeOf(&monitoringv1.ServiceMonitor{})),
					c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&monitoringv1.ServiceMonitor{}), gomock.Any()).Do(func(_ context.Context, obj client.Object, _ client.Patch, _ ...client.PatchOption) {
						Expect(obj).To(DeepEqual(serviceMonitor))
					}),
				)

				Expect(etcd.Deploy(ctx)).To(Succeed())
			})
		})
	})

	Describe("#Destroy", func() {
		var (
			etcdRes *druidv1alpha1.Etcd
			nowFunc func() time.Time
		)

		JustBeforeEach(func() {
			etcd = New(log, c, testNamespace, sm, Values{
				Role:                    testRole,
				Class:                   class,
				Replicas:                ptr.To[int32](1),
				StorageCapacity:         storageCapacity,
				StorageClassName:        &storageClassName,
				DefragmentationSchedule: &defragmentationSchedule,
				CARotationPhase:         "",
				PriorityClassName:       priorityClassName,
			})
		})

		BeforeEach(func() {
			nowFunc = func() time.Time {
				return time.Date(1, 1, 1, 1, 1, 1, 1, time.UTC)
			}
			etcdRes = &druidv1alpha1.Etcd{ObjectMeta: metav1.ObjectMeta{
				Name:      "etcd-" + testRole,
				Namespace: testNamespace,
				Annotations: map[string]string{
					"confirmation.gardener.cloud/deletion": "true",
					"gardener.cloud/timestamp":             nowFunc().Format(time.RFC3339Nano),
				},
			}}
		})

		It("should properly delete all expected objects", func() {
			defer test.WithVar(&gardener.TimeNow, nowFunc)()
			gomock.InOrder(
				c.EXPECT().Patch(ctx, etcdRes, gomock.Any()),
				c.EXPECT().Delete(ctx, &hvpav1alpha1.Hvpa{ObjectMeta: metav1.ObjectMeta{Name: "etcd-" + testRole, Namespace: testNamespace}}),
				c.EXPECT().Delete(ctx, &monitoringv1.ServiceMonitor{ObjectMeta: metav1.ObjectMeta{Name: "garden-etcd-" + testRole, Namespace: testNamespace, Labels: map[string]string{"prometheus": "garden"}}}),
				c.EXPECT().Delete(ctx, etcdRes),
			)
			Expect(etcd.Destroy(ctx)).To(Succeed())
		})

		It("should fail when the hvpa deletion fails", func() {
			defer test.WithVar(&gardener.TimeNow, nowFunc)()

			gomock.InOrder(
				c.EXPECT().Patch(ctx, etcdRes, gomock.Any()),
				c.EXPECT().Delete(ctx, &hvpav1alpha1.Hvpa{ObjectMeta: metav1.ObjectMeta{Name: "etcd-" + testRole, Namespace: testNamespace}}).Return(fakeErr),
			)

			Expect(etcd.Destroy(ctx)).To(MatchError(fakeErr))
		})

		It("should fail when the service monitor deletion fails", func() {
			defer test.WithVar(&gardener.TimeNow, nowFunc)()

			gomock.InOrder(
				c.EXPECT().Patch(ctx, etcdRes, gomock.Any()),
				c.EXPECT().Delete(ctx, &hvpav1alpha1.Hvpa{ObjectMeta: metav1.ObjectMeta{Name: "etcd-" + testRole, Namespace: testNamespace}}),
				c.EXPECT().Delete(ctx, &monitoringv1.ServiceMonitor{ObjectMeta: metav1.ObjectMeta{Name: "garden-etcd-" + testRole, Namespace: testNamespace, Labels: map[string]string{"prometheus": "garden"}}}).Return(fakeErr),
			)

			Expect(etcd.Destroy(ctx)).To(MatchError(fakeErr))
		})

		It("should fail when the etcd deletion fails", func() {
			defer test.WithVar(&gardener.TimeNow, nowFunc)()

			gomock.InOrder(
				c.EXPECT().Patch(ctx, etcdRes, gomock.Any()),
				c.EXPECT().Delete(ctx, &hvpav1alpha1.Hvpa{ObjectMeta: metav1.ObjectMeta{Name: "etcd-" + testRole, Namespace: testNamespace}}),
				c.EXPECT().Delete(ctx, &monitoringv1.ServiceMonitor{ObjectMeta: metav1.ObjectMeta{Name: "garden-etcd-" + testRole, Namespace: testNamespace, Labels: map[string]string{"prometheus": "garden"}}}),
				c.EXPECT().Delete(ctx, etcdRes).Return(fakeErr),
			)

			Expect(etcd.Destroy(ctx)).To(MatchError(fakeErr))
		})
	})

	Describe("#Snapshot", func() {
		It("should return an error when the backup config is nil", func() {
			Expect(etcd.Snapshot(ctx, nil)).To(MatchError(ContainSubstring("no backup is configured")))
		})

		Context("w/ backup configuration", func() {
			var mockHttpClient *rest.MockHTTPClient

			BeforeEach(func() {
				mockHttpClient = rest.NewMockHTTPClient(ctrl)
				etcd.SetBackupConfig(&BackupConfig{})
			})

			It("should successfully execute the full snapshot command", func() {
				url := fmt.Sprintf("https://etcd-%s-client.%s:8080/snapshot/full?final=true", testRole, testNamespace)
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				Expect(err).ToNot(HaveOccurred())

				mockHttpClient.EXPECT().Do(request)

				Expect(etcd.Snapshot(ctx, mockHttpClient)).To(Succeed())
			})

			It("should return an error when the execution command fails", func() {
				url := fmt.Sprintf("https://etcd-%s-client.%s:8080/snapshot/full?final=true", testRole, testNamespace)
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				Expect(err).ToNot(HaveOccurred())

				mockHttpClient.EXPECT().Do(request).Return(nil, fakeErr)

				Expect(etcd.Snapshot(ctx, mockHttpClient)).To(MatchError(fakeErr))
			})
		})
	})

	Describe("#Scale", func() {
		var etcdObj *druidv1alpha1.Etcd

		BeforeEach(func() {
			etcdObj = &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd-" + testRole,
					Namespace: testNamespace,
				},
			}
		})

		It("should scale ETCD from 0 to 1", func() {
			etcdObj.Spec.Replicas = 0

			nowFunc := func() time.Time {
				return now
			}
			defer test.WithVar(&TimeNow, nowFunc)()

			c.EXPECT().Get(ctx, client.ObjectKeyFromObject(etcdObj), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
					*etcd = *etcdObj
					return nil
				},
			)

			c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).DoAndReturn(
				func(_ context.Context, etcd *druidv1alpha1.Etcd, patch client.Patch, _ ...client.PatchOption) error {
					data, err := patch.Data(etcd)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(data)).To(Equal(fmt.Sprintf(`{"metadata":{"annotations":{"gardener.cloud/operation":"reconcile","gardener.cloud/timestamp":"%s"}},"spec":{"replicas":1}}`, now.Format(time.RFC3339Nano))))
					return nil
				})

			Expect(etcd.Scale(ctx, 1)).To(Succeed())
		})

		It("should set operation annotation when replica count is unchanged", func() {
			etcdObj.Spec.Replicas = 1

			nowFunc := func() time.Time {
				return now
			}
			defer test.WithVar(&TimeNow, nowFunc)()

			c.EXPECT().Get(ctx, client.ObjectKeyFromObject(etcdObj), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
					*etcd = *etcdObj
					return nil
				},
			)

			c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).DoAndReturn(
				func(_ context.Context, etcd *druidv1alpha1.Etcd, patch client.Patch, _ ...client.PatchOption) error {
					data, err := patch.Data(etcd)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(data)).To(Equal(fmt.Sprintf(`{"metadata":{"annotations":{"gardener.cloud/operation":"reconcile","gardener.cloud/timestamp":"%s"}}}`, now.Format(time.RFC3339Nano))))
					return nil
				})

			Expect(etcd.Scale(ctx, 1)).To(Succeed())
		})

		It("should fail if GardenerTimestamp is unexpected", func() {
			nowFunc := func() time.Time {
				return now
			}
			defer test.WithVar(&TimeNow, nowFunc)()

			gomock.InOrder(
				c.EXPECT().Get(ctx, client.ObjectKeyFromObject(etcdObj), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
						*etcd = *etcdObj
						return nil
					},
				),
				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()),
				c.EXPECT().Get(ctx, client.ObjectKeyFromObject(etcdObj), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
						etcdObj.Annotations = map[string]string{
							v1beta1constants.GardenerTimestamp: "foo",
						}
						*etcd = *etcdObj
						return nil
					},
				),
			)

			Expect(etcd.Scale(ctx, 1)).To(Succeed())
			Expect(etcd.Scale(ctx, 1)).Should(MatchError(`object's "gardener.cloud/timestamp" annotation is not "0001-01-01T00:00:00Z" but "foo"`))
		})

		It("should fail because operation annotation is set", func() {
			c.EXPECT().Get(ctx, client.ObjectKeyFromObject(etcdObj), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
					etcdObj.Annotations = map[string]string{
						v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
					}
					*etcd = *etcdObj
					return nil
				},
			)

			Expect(etcd.Scale(ctx, 1)).Should(MatchError(`etcd object still has operation annotation set`))
		})

		It("should update HVPA with the new replica count if it is enabled", func() {
			etcd.SetHVPAConfig(&HVPAConfig{
				Enabled: true,
			})
			etcdObj.Spec.Replicas = 1

			c.EXPECT().Get(ctx, client.ObjectKeyFromObject(etcdObj), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, etcd *druidv1alpha1.Etcd, _ ...client.GetOption) error {
					*etcd = *etcdObj
					return nil
				},
			)
			c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any())

			hvpaObj := hvpaFor(ClassImportant, 1, hvpav1alpha1.UpdateModeDefault)
			c.EXPECT().Get(ctx, client.ObjectKeyFromObject(hvpaObj), gomock.AssignableToTypeOf(&hvpav1alpha1.Hvpa{})).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, hvpa *hvpav1alpha1.Hvpa, _ ...client.GetOptions) error {
					*hvpa = *hvpaObj
					return nil
				},
			)

			expectedHvpa := hvpaObj.DeepCopy()
			expectedHvpa.Spec.Hpa.Template.Spec.MaxReplicas = 3
			expectedHvpa.Spec.Hpa.Template.Spec.MinReplicas = ptr.To[int32](3)
			test.EXPECTPatch(ctx, c, expectedHvpa, hvpaObj, types.MergePatchType)

			Expect(etcd.Scale(ctx, 3)).To(Succeed())
		})
	})

	Describe("#RolloutPeerCA", func() {
		var highAvailability bool

		JustBeforeEach(func() {
			etcd = New(log, c, testNamespace, sm, Values{
				Role:                    testRole,
				Class:                   class,
				Replicas:                replicas,
				StorageCapacity:         storageCapacity,
				StorageClassName:        &storageClassName,
				DefragmentationSchedule: &defragmentationSchedule,
				CARotationPhase:         "",
				PriorityClassName:       priorityClassName,
				HighAvailabilityEnabled: highAvailability,
			})
		})

		Context("when HA control-plane is not requested", func() {
			BeforeEach(func() {
				replicas = ptr.To[int32](1)
			})

			It("should do nothing and succeed without expectations", func() {
				Expect(etcd.RolloutPeerCA(ctx)).To(Succeed())
			})
		})

		Context("when HA control-plane is requested", func() {
			BeforeEach(func() {
				highAvailability = true
			})

			createEtcdObj := func(caName string) *druidv1alpha1.Etcd {
				return &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:       etcdName,
						Namespace:  testNamespace,
						Generation: 1,
					},
					Spec: druidv1alpha1.EtcdSpec{
						Etcd: druidv1alpha1.EtcdConfig{
							PeerUrlTLS: &druidv1alpha1.TLSConfig{
								TLSCASecretRef: druidv1alpha1.SecretReference{
									SecretReference: corev1.SecretReference{
										Name:      caName,
										Namespace: testNamespace,
									},
									DataKey: ptr.To(secretsutils.DataKeyCertificateBundle),
								},
							},
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						ObservedGeneration: ptr.To[int64](1),
						Ready:              ptr.To(true),
					},
				}
			}

			BeforeEach(func() {
				replicas = ptr.To[int32](3)
				DeferCleanup(test.WithVar(&TimeNow, func() time.Time { return now }))
			})

			It("should patch the etcd resource with the new peer CA secret name", func() {
				Expect(fakeClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ca-etcd-peer", Namespace: testNamespace}})).To(Succeed())

				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					createEtcdObj("old-ca").DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					return nil
				})

				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, obj *druidv1alpha1.Etcd, patch client.Patch, _ ...client.PatchOption) error {
						data, err := patch.Data(obj)
						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(MatchJSON("{\"metadata\":{\"annotations\":{\"gardener.cloud/operation\":\"reconcile\",\"gardener.cloud/timestamp\":\"0001-01-01T00:00:00Z\"}},\"spec\":{\"etcd\":{\"peerUrlTls\":{\"tlsCASecretRef\":{\"name\":\"ca-etcd-peer\"}}}}}"))
						return nil
					})

				c.EXPECT().Get(gomock.Any(), kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					createEtcdObj("old-ca").DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					obj.(*druidv1alpha1.Etcd).ObjectMeta.Annotations = map[string]string{"gardener.cloud/timestamp": "0001-01-01T00:00:00Z"}
					return nil
				}).AnyTimes()

				Expect(etcd.RolloutPeerCA(ctx)).To(Succeed())
			})

			It("should only patch reconcile annotation data because the expected CA ref is already configured", func() {
				peerCAName := "ca-etcd-peer"

				Expect(fakeClient.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: peerCAName, Namespace: testNamespace}})).To(Succeed())

				c.EXPECT().Get(ctx, kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					createEtcdObj(peerCAName).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					return nil
				})

				c.EXPECT().Patch(ctx, gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, obj *druidv1alpha1.Etcd, patch client.Patch, _ ...client.PatchOption) error {
						data, err := patch.Data(obj)
						Expect(err).ToNot(HaveOccurred())
						Expect(data).To(MatchJSON("{\"metadata\":{\"annotations\":{\"gardener.cloud/operation\":\"reconcile\",\"gardener.cloud/timestamp\":\"0001-01-01T00:00:00Z\"}}}"))
						return nil
					})

				c.EXPECT().Get(gomock.Any(), kubernetesutils.Key(testNamespace, etcdName), gomock.AssignableToTypeOf(&druidv1alpha1.Etcd{})).Return(apierrors.NewNotFound(schema.GroupResource{}, "")).DoAndReturn(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					createEtcdObj(peerCAName).DeepCopyInto(obj.(*druidv1alpha1.Etcd))
					obj.(*druidv1alpha1.Etcd).ObjectMeta.Annotations = map[string]string{"gardener.cloud/timestamp": "0001-01-01T00:00:00Z"}
					return nil
				}).AnyTimes()

				Expect(etcd.RolloutPeerCA(ctx)).To(Succeed())
			})

			It("should fail because CA cannot be found", func() {
				Expect(etcd.RolloutPeerCA(ctx)).To(MatchError("secret \"ca-etcd-peer\" not found"))
			})
		})
	})
})
