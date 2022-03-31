// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package botanist

import (
	"context"
	"fmt"
	"net"

	gardencorev1alpha1 "github.com/gardener/gardener/pkg/apis/core/v1alpha1"
	gardencorev1alpha1helper "github.com/gardener/gardener/pkg/apis/core/v1alpha1/helper"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	gardencorev1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/client/kubernetes/clientmap/keys"
	"github.com/gardener/gardener/pkg/features"
	gardenletfeatures "github.com/gardener/gardener/pkg/gardenlet/features"
	"github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/operation/botanist/component/etcd"
	"github.com/gardener/gardener/pkg/operation/botanist/component/kubeapiserver"
	"github.com/gardener/gardener/pkg/operation/botanist/component/vpnseedserver"
	"github.com/gardener/gardener/pkg/utils"
	gutil "github.com/gardener/gardener/pkg/utils/gardener"
	"github.com/gardener/gardener/pkg/utils/images"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	secretutils "github.com/gardener/gardener/pkg/utils/secrets"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
)

// DefaultKubeAPIServer returns a deployer for the kube-apiserver.
func (b *Botanist) DefaultKubeAPIServer(ctx context.Context) (kubeapiserver.Interface, error) {
	images, err := b.computeKubeAPIServerImages()
	if err != nil {
		return nil, err
	}

	var (
		apiServerConfig = b.Shoot.GetInfo().Spec.Kubernetes.KubeAPIServer

		admissionPlugins = kutil.GetAdmissionPluginsForVersion(b.Shoot.GetInfo().Spec.Kubernetes.Version)
		apiAudiences     = []string{"kubernetes", "gardener"}
		auditConfig      *kubeapiserver.AuditConfig
		eventTTL         *metav1.Duration
		featureGates     map[string]bool
		oidcConfig       *gardencorev1beta1.OIDCConfig
		requests         *gardencorev1beta1.KubeAPIServerRequests
		runtimeConfig    map[string]bool
		watchCacheSizes  *gardencorev1beta1.WatchCacheSizes
	)

	if apiServerConfig != nil {
		admissionPlugins = b.computeKubeAPIServerAdmissionPlugins(admissionPlugins, apiServerConfig.AdmissionPlugins)

		if apiServerConfig.APIAudiences != nil {
			apiAudiences = apiServerConfig.APIAudiences
			if !utils.ValueExists(v1beta1constants.GardenerAudience, apiAudiences) {
				apiAudiences = append(apiAudiences, v1beta1constants.GardenerAudience)
			}
		}

		auditConfig, err = b.computeKubeAPIServerAuditConfig(ctx, apiServerConfig.AuditConfig)
		if err != nil {
			return nil, err
		}

		eventTTL = apiServerConfig.EventTTL
		featureGates = apiServerConfig.FeatureGates
		oidcConfig = apiServerConfig.OIDCConfig
		requests = apiServerConfig.Requests
		runtimeConfig = apiServerConfig.RuntimeConfig

		watchCacheSizes = apiServerConfig.WatchCacheSizes
	}

	return kubeapiserver.New(
		b.K8sSeedClient,
		b.Shoot.SeedNamespace,
		b.SecretsManager,
		kubeapiserver.Values{
			AdmissionPlugins:               admissionPlugins,
			AnonymousAuthenticationEnabled: gardencorev1beta1helper.ShootWantsAnonymousAuthentication(b.Shoot.GetInfo().Spec.Kubernetes.KubeAPIServer),
			APIAudiences:                   apiAudiences,
			Audit:                          auditConfig,
			Autoscaling:                    b.computeKubeAPIServerAutoscalingConfig(),
			BasicAuthenticationEnabled:     gardencorev1beta1helper.ShootWantsBasicAuthentication(b.Shoot.GetInfo()),
			EventTTL:                       eventTTL,
			FeatureGates:                   featureGates,
			Images:                         images,
			OIDC:                           oidcConfig,
			Requests:                       requests,
			RuntimeConfig:                  runtimeConfig,
			Version:                        b.Shoot.KubernetesVersion,
			VPN: kubeapiserver.VPNConfig{
				ReversedVPNEnabled: b.Shoot.ReversedVPNEnabled,
				PodNetworkCIDR:     b.Shoot.Networks.Pods.String(),
				ServiceNetworkCIDR: b.Shoot.Networks.Services.String(),
				NodeNetworkCIDR:    b.Shoot.GetInfo().Spec.Networking.Nodes,
			},
			WatchCacheSizes: watchCacheSizes,
		},
	), nil
}

func (b *Botanist) computeKubeAPIServerAdmissionPlugins(defaultPlugins, configuredPlugins []gardencorev1beta1.AdmissionPlugin) []gardencorev1beta1.AdmissionPlugin {
	for _, plugin := range configuredPlugins {
		pluginOverwritesDefault := false

		for i, defaultPlugin := range defaultPlugins {
			if defaultPlugin.Name == plugin.Name {
				pluginOverwritesDefault = true
				defaultPlugins[i] = plugin
				break
			}
		}

		if !pluginOverwritesDefault {
			defaultPlugins = append(defaultPlugins, plugin)
		}
	}

	return defaultPlugins
}

func (b *Botanist) computeKubeAPIServerAuditConfig(ctx context.Context, config *gardencorev1beta1.AuditConfig) (*kubeapiserver.AuditConfig, error) {
	if config == nil || config.AuditPolicy == nil || config.AuditPolicy.ConfigMapRef == nil {
		return nil, nil
	}

	out := &kubeapiserver.AuditConfig{}

	configMap := &corev1.ConfigMap{}
	if err := b.K8sGardenClient.Client().Get(ctx, kutil.Key(b.Shoot.GetInfo().Namespace, config.AuditPolicy.ConfigMapRef.Name), configMap); err != nil {
		// Ignore missing audit configuration on shoot deletion to prevent failing redeployments of the
		// kube-apiserver in case the end-user deleted the configmap before/simultaneously to the shoot
		// deletion.
		if !apierrors.IsNotFound(err) || b.Shoot.GetInfo().DeletionTimestamp == nil {
			return nil, fmt.Errorf("retrieving audit policy from the ConfigMap '%v' failed with reason '%w'", config.AuditPolicy.ConfigMapRef.Name, err)
		}
	} else {
		policy, ok := configMap.Data["policy"]
		if !ok {
			return nil, fmt.Errorf("missing '.data.policy' in audit policy configmap %v/%v", b.Shoot.GetInfo().Namespace, config.AuditPolicy.ConfigMapRef.Name)
		}
		out.Policy = &policy
	}

	return out, nil
}

func (b *Botanist) computeKubeAPIServerAutoscalingConfig() kubeapiserver.AutoscalingConfig {
	var (
		hvpaEnabled               = gardenletfeatures.FeatureGate.Enabled(features.HVPA)
		useMemoryMetricForHvpaHPA = false
		scaleDownDisabledForHvpa  = false
		defaultReplicas           *int32
		minReplicas               int32 = 1
		maxReplicas               int32 = 4
		apiServerResources        corev1.ResourceRequirements
	)

	if b.Shoot.Purpose == gardencorev1beta1.ShootPurposeProduction {
		minReplicas = 2
	}

	if metav1.HasAnnotation(b.Shoot.GetInfo().ObjectMeta, v1beta1constants.ShootAlphaControlPlaneScaleDownDisabled) {
		minReplicas = 4
		scaleDownDisabledForHvpa = true
	}

	nodeCount := b.Shoot.GetMinNodeCount()
	if hvpaEnabled {
		nodeCount = b.Shoot.GetMaxNodeCount()
	}
	apiServerResources = resourcesRequirementsForKubeAPIServer(nodeCount, b.Shoot.GetInfo().Annotations[v1beta1constants.ShootAlphaScalingAPIServerClass])

	if b.ManagedSeed != nil {
		hvpaEnabled = gardenletfeatures.FeatureGate.Enabled(features.HVPAForShootedSeed)
		useMemoryMetricForHvpaHPA = true

		if b.ManagedSeedAPIServer != nil {
			minReplicas = *b.ManagedSeedAPIServer.Autoscaler.MinReplicas
			maxReplicas = b.ManagedSeedAPIServer.Autoscaler.MaxReplicas

			if !hvpaEnabled {
				defaultReplicas = b.ManagedSeedAPIServer.Replicas
				apiServerResources = corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1750m"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				}
			}
		}
	}

	return kubeapiserver.AutoscalingConfig{
		APIServerResources:        apiServerResources,
		HVPAEnabled:               hvpaEnabled,
		Replicas:                  defaultReplicas,
		MinReplicas:               minReplicas,
		MaxReplicas:               maxReplicas,
		UseMemoryMetricForHvpaHPA: useMemoryMetricForHvpaHPA,
		ScaleDownDisabledForHvpa:  scaleDownDisabledForHvpa,
	}
}

func resourcesRequirementsForKubeAPIServer(nodeCount int32, scalingClass string) corev1.ResourceRequirements {
	var (
		validScalingClasses = sets.NewString("small", "medium", "large", "xlarge", "2xlarge")
		cpuRequest          string
		memoryRequest       string
	)

	if !validScalingClasses.Has(scalingClass) {
		switch {
		case nodeCount <= 2:
			scalingClass = "small"
		case nodeCount <= 10:
			scalingClass = "medium"
		case nodeCount <= 50:
			scalingClass = "large"
		case nodeCount <= 100:
			scalingClass = "xlarge"
		default:
			scalingClass = "2xlarge"
		}
	}

	switch {
	case scalingClass == "small":
		cpuRequest = "800m"
		memoryRequest = "800Mi"

	case scalingClass == "medium":
		cpuRequest = "1000m"
		memoryRequest = "1100Mi"

	case scalingClass == "large":
		cpuRequest = "1200m"
		memoryRequest = "1600Mi"

	case scalingClass == "xlarge":
		cpuRequest = "2500m"
		memoryRequest = "5200Mi"

	case scalingClass == "2xlarge":
		cpuRequest = "3000m"
		memoryRequest = "5200Mi"
	}

	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpuRequest),
			corev1.ResourceMemory: resource.MustParse(memoryRequest),
		},
	}
}

func (b *Botanist) computeKubeAPIServerImages() (kubeapiserver.Images, error) {
	imageAlpineIPTables, err := b.ImageVector.FindImage(images.ImageNameAlpineIptables, imagevector.RuntimeVersion(b.SeedVersion()), imagevector.TargetVersion(b.ShootVersion()))
	if err != nil {
		return kubeapiserver.Images{}, err
	}

	imageApiserverProxyPodWebhook, err := b.ImageVector.FindImage(images.ImageNameApiserverProxyPodWebhook, imagevector.RuntimeVersion(b.SeedVersion()), imagevector.TargetVersion(b.ShootVersion()))
	if err != nil {
		return kubeapiserver.Images{}, err
	}

	imageKubeAPIServer, err := b.ImageVector.FindImage(images.ImageNameKubeApiserver, imagevector.RuntimeVersion(b.SeedVersion()), imagevector.TargetVersion(b.ShootVersion()))
	if err != nil {
		return kubeapiserver.Images{}, err
	}

	imageVPNSeed, err := b.ImageVector.FindImage(images.ImageNameVpnSeed, imagevector.RuntimeVersion(b.SeedVersion()), imagevector.TargetVersion(b.ShootVersion()))
	if err != nil {
		return kubeapiserver.Images{}, err
	}

	return kubeapiserver.Images{
		AlpineIPTables:           imageAlpineIPTables.String(),
		APIServerProxyPodWebhook: imageApiserverProxyPodWebhook.String(),
		KubeAPIServer:            imageKubeAPIServer.String(),
		VPNSeed:                  imageVPNSeed.String(),
	}, nil
}

func (b *Botanist) computeKubeAPIServerServerCertificateConfig() kubeapiserver.ServerCertificateConfig {
	var (
		ipAddresses = []net.IP{
			b.Shoot.Networks.APIServer,
		}
		dnsNames = []string{
			gutil.GetAPIServerDomain(b.Shoot.InternalClusterDomain),
			b.Shoot.GetInfo().Status.TechnicalID,
		}
	)

	if !b.Seed.GetInfo().Spec.Settings.ShootDNS.Enabled {
		if addr := net.ParseIP(b.APIServerAddress); addr != nil {
			ipAddresses = append(ipAddresses, addr)
		} else {
			dnsNames = append(dnsNames, b.APIServerAddress)
		}
	}

	if b.Shoot.ExternalClusterDomain != nil {
		dnsNames = append(dnsNames, *(b.Shoot.GetInfo().Spec.DNS.Domain), gutil.GetAPIServerDomain(*b.Shoot.ExternalClusterDomain))
	}

	return kubeapiserver.ServerCertificateConfig{
		ExtraIPAddresses: ipAddresses,
		ExtraDNSNames:    dnsNames,
	}
}

func (b *Botanist) computeKubeAPIServerServiceAccountConfig(ctx context.Context, config *gardencorev1beta1.KubeAPIServerConfig, externalHostname string) (kubeapiserver.ServiceAccountConfig, error) {
	out := kubeapiserver.ServiceAccountConfig{Issuer: "https://" + externalHostname}

	if config == nil || config.ServiceAccountConfig == nil {
		return out, nil
	}

	out.ExtendTokenExpiration = config.ServiceAccountConfig.ExtendTokenExpiration
	out.MaxTokenExpiration = config.ServiceAccountConfig.MaxTokenExpiration

	if config.ServiceAccountConfig.Issuer != nil {
		out.Issuer = *config.ServiceAccountConfig.Issuer
	}
	out.AcceptedIssuers = config.ServiceAccountConfig.AcceptedIssuers

	if signingKeySecret := config.ServiceAccountConfig.SigningKeySecret; signingKeySecret != nil {
		secret := &corev1.Secret{}
		if err := b.K8sGardenClient.Client().Get(ctx, kutil.Key(b.Shoot.GetInfo().Namespace, signingKeySecret.Name), secret); err != nil {
			return out, err
		}

		data, ok := secret.Data[kubeapiserver.SecretServiceAccountSigningKeyDataKeySigningKey]
		if !ok {
			return out, fmt.Errorf("no signing key in secret %s/%s at .data.%s", secret.Namespace, secret.Name, kubeapiserver.SecretServiceAccountSigningKeyDataKeySigningKey)
		}
		out.SigningKey = data
	}

	return out, nil
}

func (b *Botanist) computeKubeAPIServerSNIConfig() kubeapiserver.SNIConfig {
	var config kubeapiserver.SNIConfig

	if b.APIServerSNIEnabled() {
		config.Enabled = true
		config.AdvertiseAddress = b.APIServerClusterIP

		if b.APIServerSNIPodMutatorEnabled() {
			config.PodMutatorEnabled = true
			config.APIServerFQDN = b.Shoot.ComputeOutOfClusterAPIServerAddress(b.APIServerAddress, true)
		}
	}

	return config
}

func (b *Botanist) computeKubeAPIServerReplicas(autoscalingConfig kubeapiserver.AutoscalingConfig, deployment *appsv1.Deployment) *int32 {
	switch {
	case autoscalingConfig.Replicas != nil:
		// If the replicas were already set then don't change them.
		return autoscalingConfig.Replicas
	case deployment == nil && !b.Shoot.HibernationEnabled:
		// If the Deployment does not yet exist then set the desired replicas to the minimum replicas.
		return &autoscalingConfig.MinReplicas
	case deployment != nil && deployment.Spec.Replicas != nil && *deployment.Spec.Replicas > 0:
		// If the Deployment exists then don't interfere with the replicas because they are controlled via HVPA or HPA.
		return deployment.Spec.Replicas
	case b.Shoot.HibernationEnabled && (deployment == nil || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas == 0):
		// If the Shoot is hibernated and the deployment has already been scaled down then we want to keep it scaled
		// down. If it has not yet been scaled down then above case applies (replicas are kept) - the scale-down will
		// happen at a later point in the flow.
		return pointer.Int32(0)
	default:
		// If none of the above cases applies then a default value has to be returned.
		return pointer.Int32(1)
	}
}

// DeployKubeAPIServer deploys the Kubernetes API server.
func (b *Botanist) DeployKubeAPIServer(ctx context.Context) error {
	values := b.Shoot.Components.ControlPlane.KubeAPIServer.GetValues()

	deployment := &appsv1.Deployment{}
	if err := b.K8sSeedClient.Client().Get(ctx, kutil.Key(b.Shoot.SeedNamespace, v1beta1constants.DeploymentNameKubeAPIServer), deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		deployment = nil
	}

	b.Shoot.Components.ControlPlane.KubeAPIServer.SetAutoscalingReplicas(b.computeKubeAPIServerReplicas(values.Autoscaling, deployment))

	if deployment != nil && values.Autoscaling.HVPAEnabled {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Name == kubeapiserver.ContainerNameKubeAPIServer {
				b.Shoot.Components.ControlPlane.KubeAPIServer.SetAutoscalingAPIServerResources(container.Resources)
				break
			}
		}
	}

	secrets := kubeapiserver.Secrets{
		CA:           component.Secret{Name: v1beta1constants.SecretNameCACluster, Checksum: b.LoadCheckSum(v1beta1constants.SecretNameCACluster)},
		CAEtcd:       component.Secret{Name: etcd.SecretNameCA, Checksum: b.LoadCheckSum(etcd.SecretNameCA)},
		CAFrontProxy: component.Secret{Name: v1beta1constants.SecretNameCAFrontProxy, Checksum: b.LoadCheckSum(v1beta1constants.SecretNameCAFrontProxy)},
		Etcd:         component.Secret{Name: etcd.SecretNameClient, Checksum: b.LoadCheckSum(etcd.SecretNameClient)},
	}

	if values.VPN.ReversedVPNEnabled {
		secrets.VPNSeedServerTLSAuth = &component.Secret{Name: vpnseedserver.VpnSeedServerTLSAuth, Checksum: b.LoadCheckSum(vpnseedserver.VpnSeedServerTLSAuth)}
	} else {
		secrets.VPNSeed = &component.Secret{Name: kubeapiserver.SecretNameVPNSeed, Checksum: b.LoadCheckSum(kubeapiserver.SecretNameVPNSeed)}
		secrets.VPNSeedTLSAuth = &component.Secret{Name: kubeapiserver.SecretNameVPNSeedTLSAuth, Checksum: b.LoadCheckSum(kubeapiserver.SecretNameVPNSeedTLSAuth)}
	}

	b.Shoot.Components.ControlPlane.KubeAPIServer.SetSecrets(secrets)
	b.Shoot.Components.ControlPlane.KubeAPIServer.SetServerCertificateConfig(b.computeKubeAPIServerServerCertificateConfig())
	b.Shoot.Components.ControlPlane.KubeAPIServer.SetSNIConfig(b.computeKubeAPIServerSNIConfig())

	externalHostname := b.Shoot.ComputeOutOfClusterAPIServerAddress(b.APIServerAddress, true)
	b.Shoot.Components.ControlPlane.KubeAPIServer.SetExternalHostname(externalHostname)

	externalServer := b.Shoot.ComputeOutOfClusterAPIServerAddress(b.APIServerAddress, false)
	b.Shoot.Components.ControlPlane.KubeAPIServer.SetExternalServer(externalServer)

	serviceAccountConfig, err := b.computeKubeAPIServerServiceAccountConfig(ctx, b.Shoot.GetInfo().Spec.Kubernetes.KubeAPIServer, externalHostname)
	if err != nil {
		return err
	}
	b.Shoot.Components.ControlPlane.KubeAPIServer.SetServiceAccountConfig(serviceAccountConfig)

	if err := b.Shoot.Components.ControlPlane.KubeAPIServer.Deploy(ctx); err != nil {
		return err
	}

	userKubeconfigSecret, found := b.SecretsManager.Get(kubeapiserver.SecretNameUserKubeconfig)
	if !found {
		return fmt.Errorf("secret %q not found", kubeapiserver.SecretNameUserKubeconfig)
	}

	// add CA bundle as ca.crt to kubeconfig secret for backwards-compatibility
	caBundleSecret, found := b.SecretsManager.Get(v1beta1constants.SecretNameCACluster)
	if !found {
		return fmt.Errorf("secret %q not found", v1beta1constants.SecretNameCACluster)
	}

	kubeconfigSecretData := userKubeconfigSecret.DeepCopy().Data
	kubeconfigSecretData[secretutils.DataKeyCertificateCA] = caBundleSecret.Data[secretutils.DataKeyCertificateBundle]

	if err := b.syncShootCredentialToGarden(
		ctx,
		gutil.ShootProjectSecretSuffixKubeconfig,
		map[string]string{v1beta1constants.GardenRole: v1beta1constants.GardenRoleKubeconfig},
		map[string]string{"url": "https://" + externalServer},
		kubeconfigSecretData,
	); err != nil {
		return err
	}

	// TODO(rfranzke): Remove in a future release.
	if err := b.SaveGardenerResourceDataInShootState(ctx, func(gardenerResourceData *[]gardencorev1alpha1.GardenerResourceData) error {
		gardenerResourceDataList := gardencorev1alpha1helper.GardenerResourceDataList(*gardenerResourceData)
		gardenerResourceDataList.Delete("static-token")
		gardenerResourceDataList.Delete("kube-apiserver-basic-auth")
		gardenerResourceDataList.Delete("etcdEncryptionConfiguration")
		gardenerResourceDataList.Delete("service-account-key")
		*gardenerResourceData = gardenerResourceDataList
		return nil
	}); err != nil {
		return err
	}

	return kutil.DeleteObjects(ctx, b.K8sSeedClient.Client(),
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: b.Shoot.SeedNamespace, Name: "audit-policy-config"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: b.Shoot.SeedNamespace, Name: "kube-apiserver-admission-config"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: b.Shoot.SeedNamespace, Name: "kube-apiserver-egress-selector-configuration"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: b.Shoot.SeedNamespace, Name: "kube-apiserver-oidc-cabundle"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: b.Shoot.SeedNamespace, Name: "kube-apiserver-service-account-signing-key"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: b.Shoot.SeedNamespace, Name: "etcd-encryption-secret"}},
	)
}

// DeleteKubeAPIServer deletes the kube-apiserver deployment in the Seed cluster which holds the Shoot's control plane.
func (b *Botanist) DeleteKubeAPIServer(ctx context.Context) error {
	// invalidate shoot client here before deleting API server
	if err := b.ClientMap.InvalidateClient(keys.ForShoot(b.Shoot.GetInfo())); err != nil {
		return err
	}
	b.K8sShootClient = nil

	return b.Shoot.Components.ControlPlane.KubeAPIServer.Destroy(ctx)
}

// WakeUpKubeAPIServer creates a service and ensures API Server is scaled up
func (b *Botanist) WakeUpKubeAPIServer(ctx context.Context) error {
	sniPhase := b.Shoot.Components.ControlPlane.KubeAPIServerSNIPhase.Done()

	if err := b.DeployKubeAPIService(ctx, sniPhase); err != nil {
		return err
	}
	if err := b.Shoot.Components.ControlPlane.KubeAPIServerService.Wait(ctx); err != nil {
		return err
	}
	if b.APIServerSNIEnabled() {
		if err := b.DeployKubeAPIServerSNI(ctx); err != nil {
			return err
		}
	}
	if err := b.DeployKubeAPIServer(ctx); err != nil {
		return err
	}
	if err := kubernetes.ScaleDeployment(ctx, b.K8sSeedClient.Client(), kutil.Key(b.Shoot.SeedNamespace, v1beta1constants.DeploymentNameKubeAPIServer), 1); err != nil {
		return err
	}
	return b.Shoot.Components.ControlPlane.KubeAPIServer.Wait(ctx)
}

// ScaleKubeAPIServerToOne scales kube-apiserver replicas to one.
func (b *Botanist) ScaleKubeAPIServerToOne(ctx context.Context) error {
	return kubernetes.ScaleDeployment(ctx, b.K8sSeedClient.Client(), kutil.Key(b.Shoot.SeedNamespace, v1beta1constants.DeploymentNameKubeAPIServer), 1)
}
