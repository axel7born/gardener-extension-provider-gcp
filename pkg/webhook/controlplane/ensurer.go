// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package controlplane

import (
	"bytes"
	"context"
	"regexp"

	"github.com/Masterminds/semver"
	"github.com/coreos/go-systemd/v22/unit"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	gcontext "github.com/gardener/gardener/extensions/pkg/webhook/context"
	"github.com/gardener/gardener/extensions/pkg/webhook/controlplane/genericmutator"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/component/machinecontrollermanager"
	gutil "github.com/gardener/gardener/pkg/utils/gardener"
	versionutils "github.com/gardener/gardener/pkg/utils/version"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	vpaautoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	"k8s.io/utils/pointer"

	"github.com/gardener/gardener-extension-provider-gcp/pkg/gcp"
	"github.com/gardener/gardener-extension-provider-gcp/pkg/internal/imagevector"
)

// NewEnsurer creates a new controlplane ensurer.
func NewEnsurer(logger logr.Logger, gardenletManagesMCM bool) genericmutator.Ensurer {
	return &ensurer{
		logger:              logger.WithName("gcp-controlplane-ensurer"),
		gardenletManagesMCM: gardenletManagesMCM,
	}
}

type ensurer struct {
	genericmutator.NoopEnsurer
	logger              logr.Logger
	gardenletManagesMCM bool
}

// ImageVector is exposed for testing.
var ImageVector = imagevector.ImageVector()

// EnsureMachineControllerManagerDeployment ensures that the machine-controller-manager deployment conforms to the provider requirements.
func (e *ensurer) EnsureMachineControllerManagerDeployment(_ context.Context, _ gcontext.GardenContext, newObj, _ *appsv1.Deployment) error {
	if !e.gardenletManagesMCM {
		return nil
	}

	image, err := ImageVector.FindImage(gcp.MachineControllerManagerProviderGCPImageName)
	if err != nil {
		return err
	}

	newObj.Spec.Template.Spec.Containers = extensionswebhook.EnsureContainerWithName(
		newObj.Spec.Template.Spec.Containers,
		machinecontrollermanager.ProviderSidecarContainer(newObj.Namespace, gcp.Name, image.String()),
	)
	return nil
}

// EnsureMachineControllerManagerVPA ensures that the machine-controller-manager VPA conforms to the provider requirements.
func (e *ensurer) EnsureMachineControllerManagerVPA(_ context.Context, _ gcontext.GardenContext, newObj, _ *vpaautoscalingv1.VerticalPodAutoscaler) error {
	if !e.gardenletManagesMCM {
		return nil
	}

	var (
		minAllowed = corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("64Mi"),
		}
		maxAllowed = corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("5G"),
		}
	)

	if newObj.Spec.ResourcePolicy == nil {
		newObj.Spec.ResourcePolicy = &vpaautoscalingv1.PodResourcePolicy{}
	}

	newObj.Spec.ResourcePolicy.ContainerPolicies = extensionswebhook.EnsureVPAContainerResourcePolicyWithName(
		newObj.Spec.ResourcePolicy.ContainerPolicies,
		machinecontrollermanager.ProviderSidecarVPAContainerPolicy(gcp.Name, minAllowed, maxAllowed),
	)
	return nil
}

// EnsureKubeAPIServerDeployment ensures that the kube-apiserver deployment conforms to the provider requirements.
func (e *ensurer) EnsureKubeAPIServerDeployment(ctx context.Context, gctx gcontext.GardenContext, new, _ *appsv1.Deployment) error {
	template := &new.Spec.Template
	ps := &template.Spec

	// TODO: This label approach is deprecated and no longer needed in the future. Remove it as soon as gardener/gardener@v1.75 has been released.
	metav1.SetMetaDataLabel(&new.Spec.Template.ObjectMeta, gutil.NetworkPolicyLabel(gcp.CSISnapshotValidationName, 443), v1beta1constants.LabelNetworkPolicyAllowed)

	cluster, err := gctx.GetCluster(ctx)
	if err != nil {
		return err
	}

	k8sVersion, err := semver.NewVersion(cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return err
	}

	if c := extensionswebhook.ContainerWithName(ps.Containers, "kube-apiserver"); c != nil {
		ensureKubeAPIServerCommandLineArgs(c, k8sVersion)
	}

	return nil
}

// EnsureKubeControllerManagerDeployment ensures that the kube-controller-manager deployment conforms to the provider requirements.
func (e *ensurer) EnsureKubeControllerManagerDeployment(ctx context.Context, gctx gcontext.GardenContext, new, _ *appsv1.Deployment) error {
	template := &new.Spec.Template
	ps := &template.Spec

	cluster, err := gctx.GetCluster(ctx)
	if err != nil {
		return err
	}

	k8sVersion, err := semver.NewVersion(cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return err
	}

	if c := extensionswebhook.ContainerWithName(ps.Containers, "kube-controller-manager"); c != nil {
		ensureKubeControllerManagerCommandLineArgs(c, k8sVersion)
		ensureKubeControllerManagerVolumeMounts(c)
	}

	ensureKubeControllerManagerLabels(template)
	ensureKubeControllerManagerVolumes(ps)
	return nil
}

// EnsureKubeSchedulerDeployment ensures that the kube-scheduler deployment conforms to the provider requirements.
func (e *ensurer) EnsureKubeSchedulerDeployment(ctx context.Context, gctx gcontext.GardenContext, new, _ *appsv1.Deployment) error {
	template := &new.Spec.Template
	ps := &template.Spec

	cluster, err := gctx.GetCluster(ctx)
	if err != nil {
		return err
	}

	k8sVersion, err := semver.NewVersion(cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return err
	}

	if c := extensionswebhook.ContainerWithName(ps.Containers, "kube-scheduler"); c != nil {
		ensureKubeSchedulerCommandLineArgs(c, k8sVersion)
	}
	return nil
}

// EnsureClusterAutoscalerDeployment ensures that the cluster-autoscaler deployment conforms to the provider requirements.
func (e *ensurer) EnsureClusterAutoscalerDeployment(ctx context.Context, gctx gcontext.GardenContext, new, _ *appsv1.Deployment) error {
	template := &new.Spec.Template
	ps := &template.Spec

	cluster, err := gctx.GetCluster(ctx)
	if err != nil {
		return err
	}

	k8sVersion, err := semver.NewVersion(cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return err
	}

	if c := extensionswebhook.ContainerWithName(ps.Containers, "cluster-autoscaler"); c != nil {
		ensureClusterAutoscalerCommandLineArgs(c, k8sVersion)
	}
	return nil
}

func ensureKubeAPIServerCommandLineArgs(c *corev1.Container, k8sVersion *semver.Version) {
	if versionutils.ConstraintK8sLess127.Check(k8sVersion) {
		c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
			"CSIMigration=true", ",")
	}

	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"CSIMigrationGCE=true", ",")
	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"InTreePluginGCEUnregister"+"=true", ",")
	c.Command = extensionswebhook.EnsureNoStringWithPrefix(c.Command, "--cloud-provider=")
	c.Command = extensionswebhook.EnsureNoStringWithPrefix(c.Command, "--cloud-config=")
	c.Command = extensionswebhook.EnsureNoStringWithPrefixContains(c.Command, "--enable-admission-plugins=",
		"PersistentVolumeLabel", ",")
	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--disable-admission-plugins=",
		"PersistentVolumeLabel", ",")
}

func ensureKubeControllerManagerCommandLineArgs(c *corev1.Container, k8sVersion *semver.Version) {
	c.Command = extensionswebhook.EnsureStringWithPrefix(c.Command, "--cloud-provider=", "external")
	if versionutils.ConstraintK8sLess127.Check(k8sVersion) {
		c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
			"CSIMigration=true", ",")
	}

	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"CSIMigrationGCE=true", ",")
	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"InTreePluginGCEUnregister"+"=true", ",")
	c.Command = extensionswebhook.EnsureNoStringWithPrefix(c.Command, "--cloud-config=")
	c.Command = extensionswebhook.EnsureNoStringWithPrefix(c.Command, "--external-cloud-volume-plugin=")
}

func ensureKubeSchedulerCommandLineArgs(c *corev1.Container, k8sVersion *semver.Version) {
	if versionutils.ConstraintK8sLess127.Check(k8sVersion) {
		c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
			"CSIMigration=true", ",")
	}

	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"CSIMigrationGCE=true", ",")
	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"InTreePluginGCEUnregister"+"=true", ",")
}

// ensureClusterAutoscalerCommandLineArgs ensures the cluster-autoscaler command line args.
func ensureClusterAutoscalerCommandLineArgs(c *corev1.Container, k8sVersion *semver.Version) {
	if versionutils.ConstraintK8sLess127.Check(k8sVersion) {
		c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
			"CSIMigration=true", ",")
	}

	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"CSIMigrationGCE=true", ",")
	c.Command = extensionswebhook.EnsureStringWithPrefixContains(c.Command, "--feature-gates=",
		"InTreePluginGCEUnregister"+"=true", ",")
}

func ensureKubeControllerManagerLabels(t *corev1.PodTemplateSpec) {
	if t.Labels != nil {
		// make sure to always remove this label
		delete(t.Labels, v1beta1constants.LabelNetworkPolicyToBlockedCIDRs)

		delete(t.Labels, v1beta1constants.LabelNetworkPolicyToPublicNetworks)
		delete(t.Labels, v1beta1constants.LabelNetworkPolicyToPrivateNetworks)
	}
}

var (
	etcSSLName = "etc-ssl"

	etcSSLVolumeMount = corev1.VolumeMount{
		Name:      etcSSLName,
		MountPath: "/etc/ssl",
		ReadOnly:  true,
	}

	usrShareCaCerts            = "usr-share-cacerts"
	directoryOrCreate          = corev1.HostPathDirectoryOrCreate
	usrShareCaCertsVolumeMount = corev1.VolumeMount{
		Name:      usrShareCaCerts,
		MountPath: "/usr/share/ca-certificates",
		ReadOnly:  true,
	}
	usrShareCaCertsVolume = corev1.Volume{
		Name: usrShareCaCerts,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/usr/share/ca-certificates",
				Type: &directoryOrCreate,
			},
		},
	}
	etcSSLVolume = corev1.Volume{
		Name: etcSSLName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/etc/ssl",
				Type: &directoryOrCreate,
			},
		},
	}
)

func ensureKubeControllerManagerVolumeMounts(c *corev1.Container) {
	c.VolumeMounts = extensionswebhook.EnsureNoVolumeMountWithName(c.VolumeMounts, etcSSLVolumeMount.Name)
	c.VolumeMounts = extensionswebhook.EnsureNoVolumeMountWithName(c.VolumeMounts, usrShareCaCertsVolumeMount.Name)
}

func ensureKubeControllerManagerVolumes(ps *corev1.PodSpec) {
	ps.Volumes = extensionswebhook.EnsureNoVolumeWithName(ps.Volumes, etcSSLVolume.Name)
	ps.Volumes = extensionswebhook.EnsureNoVolumeWithName(ps.Volumes, usrShareCaCertsVolume.Name)
}

// EnsureKubeletServiceUnitOptions ensures that the kubelet.service unit options conform to the provider requirements.
func (e *ensurer) EnsureKubeletServiceUnitOptions(_ context.Context, _ gcontext.GardenContext, _ *semver.Version, new, _ []*unit.UnitOption) ([]*unit.UnitOption, error) {
	if opt := extensionswebhook.UnitOptionWithSectionAndName(new, "Service", "ExecStart"); opt != nil {
		command := extensionswebhook.DeserializeCommandLine(opt.Value)
		command = ensureKubeletCommandLineArgs(command)
		opt.Value = extensionswebhook.SerializeCommandLine(command, 1, " \\\n    ")
	}

	newOption := extensionswebhook.EnsureUnitOption(new, &unit.UnitOption{
		Section: "Service",
		Name:    "ExecStartPre",
		Value:   `/bin/sh -c 'hostnamectl set-hostname $(wget -q -O- --header "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/hostname | cut -d '.' -f 1)'`,
	})

	return newOption, nil
}

func ensureKubeletCommandLineArgs(command []string) []string {
	return extensionswebhook.EnsureStringWithPrefix(command, "--cloud-provider=", "external")
}

// EnsureKubeletConfiguration ensures that the kubelet configuration conforms to the provider requirements.
func (e *ensurer) EnsureKubeletConfiguration(_ context.Context, _ gcontext.GardenContext, kubeletVersion *semver.Version, new, _ *kubeletconfigv1beta1.KubeletConfiguration) error {
	if new.FeatureGates == nil {
		new.FeatureGates = make(map[string]bool)
	}

	if versionutils.ConstraintK8sLess127.Check(kubeletVersion) {
		new.FeatureGates["CSIMigration"] = true
	}

	new.FeatureGates["CSIMigrationGCE"] = true
	// kubelets of new worker nodes can directly be started with the <csiMigrationCompleteFeatureGate> feature gate
	new.FeatureGates["InTreePluginGCEUnregister"] = true

	new.EnableControllerAttachDetach = pointer.Bool(true)

	return nil
}

var regexFindProperty = regexp.MustCompile("net.ipv4.ip_forward[[:space:]]*=[[:space:]]*([[:alnum:]]+)")

// EnsureKubernetesGeneralConfiguration ensures that the kubernetes general configuration conforms to the provider requirements.
func (e *ensurer) EnsureKubernetesGeneralConfiguration(_ context.Context, _ gcontext.GardenContext, new, _ *string) error {
	// If the needed property exists, ensure the correct value
	if regexFindProperty.MatchString(*new) {
		res := regexFindProperty.ReplaceAll([]byte(*new), []byte("net.ipv4.ip_forward = 1"))
		*new = string(res)
		return nil
	}

	// If the property do not exist, append it in the end of the string
	buf := bytes.Buffer{}
	buf.WriteString(*new)
	buf.WriteString("\n")
	buf.WriteString("# GCE specific settings\n")
	buf.WriteString("net.ipv4.ip_forward = 1")

	*new = buf.String()
	return nil
}
