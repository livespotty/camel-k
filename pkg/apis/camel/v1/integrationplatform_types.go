/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make generate-deepcopy" to regenerate code after modifying this file

// IntegrationPlatformSpec defines the desired state of IntegrationPlatform.
type IntegrationPlatformSpec struct {
	// what kind of cluster you're running (ie, plain Kubernetes or OpenShift)
	Cluster IntegrationPlatformCluster `json:"cluster,omitempty"`
	// the profile you wish to use. It will apply certain traits which are required by the specific profile chosen.
	// It usually relates the Cluster with the optional definition of special profiles (ie, Knative)
	Profile TraitProfile `json:"profile,omitempty"`
	// specify how to build the Integration/IntegrationKits
	Build IntegrationPlatformBuildSpec `json:"build,omitempty"`
	// list of traits to be executed for all the Integration/IntegrationKits built from this IntegrationPlatform
	Traits Traits `json:"traits,omitempty"`
	// Deprecated:
	// Use camel trait (camel.properties) to manage properties
	// Use mount trait (mount.configs) to manage configs
	// Use mount trait (mount.resources) to manage resources
	// Use mount trait (mount.volumes) to manage volumes
	// list of configuration properties to be attached to all the Integration/IntegrationKits built from this IntegrationPlatform
	Configuration []ConfigurationSpec `json:"configuration,omitempty"`
	// configuration to be executed to all Kamelets controlled by this IntegrationPlatform
	Kamelet IntegrationPlatformKameletSpec `json:"kamelet,omitempty"`
}

// IntegrationPlatformStatus defines the observed state of IntegrationPlatform.
type IntegrationPlatformStatus struct {
	IntegrationPlatformSpec `json:",inline"`
	// ObservedGeneration is the most recent generation observed for this IntegrationPlatform.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// defines in what phase the IntegrationPlatform is found
	Phase IntegrationPlatformPhase `json:"phase,omitempty"`
	// which are the conditions met (particularly useful when in ERROR phase)
	Conditions []IntegrationPlatformCondition `json:"conditions,omitempty"`
	// the Camel K operator version controlling this IntegrationPlatform
	Version string `json:"version,omitempty"`
	// generic information related to the build of Camel K operator software
	Info map[string]string `json:"info,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=integrationplatforms,scope=Namespaced,shortName=itp,categories=kamel;camel
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="The integration platform phase"
// +kubebuilder:printcolumn:name="Build strategy",type=string,JSONPath=`.status.build.buildConfiguration.strategy`,description="The default build strategy"
// +kubebuilder:printcolumn:name="Publish strategy",type=string,JSONPath=`.status.build.publishStrategy`,description="The default publish strategy"
// +kubebuilder:printcolumn:name="Registry address",type=string,JSONPath=`.status.build.registry.address`,description="The container registry address"
// +kubebuilder:printcolumn:name="Default runtime",type=string,JSONPath=`.status.build.runtimeVersion`,description="The default runtime version"
// +kubebuilder:printcolumn:name="Camel version",type=string,JSONPath=`.status.build.runtimeCoreVersion`,description="The default Camel core version"

// IntegrationPlatform is the resource used to drive the Camel K operator behavior.
// It defines the behavior of all Custom Resources (`IntegrationKit`, `Integration`, `Kamelet`) in the given namespace.
// When the Camel K operator is installed in `global` mode,
// you will need to specify an `IntegrationPlatform` in each namespace where you want the Camel K operator to be executed.
type IntegrationPlatform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IntegrationPlatformSpec   `json:"spec,omitempty"`
	Status IntegrationPlatformStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IntegrationPlatformList contains a list of IntegrationPlatform.
type IntegrationPlatformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IntegrationPlatform `json:"items"`
}

// IntegrationPlatformCluster is the kind of orchestration cluster the platform is installed into.
type IntegrationPlatformCluster string

const (
	// IntegrationPlatformClusterOpenShift is used when targeting an OpenShift cluster.
	IntegrationPlatformClusterOpenShift IntegrationPlatformCluster = "OpenShift"
	// IntegrationPlatformClusterKubernetes is used when targeting a Kubernetes cluster.
	IntegrationPlatformClusterKubernetes IntegrationPlatformCluster = "Kubernetes"
)

// AllIntegrationPlatformClusters --.
var AllIntegrationPlatformClusters = []IntegrationPlatformCluster{IntegrationPlatformClusterOpenShift, IntegrationPlatformClusterKubernetes}

// IntegrationPlatformBuildSpec contains platform related build information.
// This configuration can be used to tune the behavior of the Integration/IntegrationKit image builds.
// You can define the build strategy, the image registry to use and the Maven configuration to adopt.
type IntegrationPlatformBuildSpec struct {
	// the configuration required to build an Integration container image
	BuildConfiguration BuildConfiguration `json:"buildConfiguration,omitempty"`
	// the strategy to adopt for publishing an Integration container image
	PublishStrategy IntegrationPlatformBuildPublishStrategy `json:"publishStrategy,omitempty"`
	// the Camel K Runtime dependency version
	RuntimeVersion string `json:"runtimeVersion,omitempty"`
	// the runtime used. Likely Camel Quarkus (we used to have main runtime which has been discontinued since version 1.5)
	RuntimeProvider RuntimeProvider `json:"runtimeProvider,omitempty"`
	// the Camel core version used by this IntegrationPlatform
	RuntimeCoreVersion string `json:"runtimeCoreVersion,omitempty"`
	// a base image that can be used as base layer for all images.
	// It can be useful if you want to provide some custom base image with further utility software
	BaseImage string `json:"baseImage,omitempty"`
	// the image registry used to push/pull Integration images
	Registry RegistrySpec `json:"registry,omitempty"`
	// the timeout (in seconds) to use when creating the build tools container image
	// Deprecated: no longer in use
	BuildCatalogToolTimeout *metav1.Duration `json:"buildCatalogToolTimeout,omitempty"`
	// how much time to wait before time out the pipeline process
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	// Maven configuration used to build the Camel/Camel-Quarkus applications
	Maven MavenSpec `json:"maven,omitempty"`
	// Deprecated: no longer in use
	PublishStrategyOptions map[string]string `json:"PublishStrategyOptions,omitempty"`
	// the maximum amount of parallel running pipelines started by this operator instance
	MaxRunningBuilds int32 `json:"maxRunningBuilds,omitempty"`
}

// IntegrationPlatformKameletSpec define the behavior for all the Kamelets controller by the IntegrationPlatform.
type IntegrationPlatformKameletSpec struct {
	// remote repository used to retrieve Kamelet catalog
	Repositories []KameletRepositorySpec `json:"repositories,omitempty"`
}

// IntegrationPlatformBuildPublishStrategy defines the strategy used to package and publish an Integration base image.
type IntegrationPlatformBuildPublishStrategy string

const (
	// IntegrationPlatformBuildPublishStrategyJib uses the Source to Images (S2I) feature
	// (https://docs.openshift.com/container-platform/4.9/openshift_images/create-images.html#images-create-s2i_create-images)
	// provided by an OpenShift cluster in order to create and push the images to the registry. It is the default choice on OpenShift cluster.
	// Deprecated: use jib or a custom publishing strategy instead.
	IntegrationPlatformBuildPublishStrategyS2I IntegrationPlatformBuildPublishStrategy = "S2I"
	// IntegrationPlatformBuildPublishStrategySpectrum uses Spectrum project (https://github.com/container-tools/spectrum)
	// in order to push the incremental images to the image repository. It is the default choice on vanilla Kubernetes cluster.
	// Deprecated: use jib or a custom publishing strategy instead.
	IntegrationPlatformBuildPublishStrategySpectrum IntegrationPlatformBuildPublishStrategy = "Spectrum"
	// IntegrationPlatformBuildPublishStrategyJib uses Jib maven plugin (https://github.com/GoogleContainerTools/jib)
	// in order to push the incremental images to the image repository.
	IntegrationPlatformBuildPublishStrategyJib IntegrationPlatformBuildPublishStrategy = "Jib"
)

// IntegrationPlatformBuildPublishStrategies the list of all available publish strategies.
var IntegrationPlatformBuildPublishStrategies = []IntegrationPlatformBuildPublishStrategy{
	IntegrationPlatformBuildPublishStrategyS2I,
	IntegrationPlatformBuildPublishStrategySpectrum,
	IntegrationPlatformBuildPublishStrategyJib,
}

// IntegrationPlatformPhase is the phase of an IntegrationPlatform.
type IntegrationPlatformPhase string

// IntegrationPlatformConditionType defines the type of condition.
type IntegrationPlatformConditionType string

const (
	// IntegrationPlatformKind is the Kind name of the IntegrationPlatform CR.
	IntegrationPlatformKind string = "IntegrationPlatform"

	// IntegrationPlatformPhaseNone when the IntegrationPlatform does not exist.
	IntegrationPlatformPhaseNone IntegrationPlatformPhase = ""
	// IntegrationPlatformPhaseCreating when the IntegrationPlatform is under creation process.
	IntegrationPlatformPhaseCreating IntegrationPlatformPhase = "Creating"
	// IntegrationPlatformPhaseWarming when the IntegrationPlatform is warming (ie, creating Kaniko cache).
	// Deprecated no longer in use.
	IntegrationPlatformPhaseWarming IntegrationPlatformPhase = "Warming"
	// IntegrationPlatformPhaseReady when the IntegrationPlatform is ready.
	IntegrationPlatformPhaseReady IntegrationPlatformPhase = "Ready"
	// IntegrationPlatformPhaseError when the IntegrationPlatform had some error (see Conditions).
	IntegrationPlatformPhaseError IntegrationPlatformPhase = "Error"
	// IntegrationPlatformPhaseCreateCatalog when the IntegrationPlatform creates a new CamelCatalog.
	// Deprecated no longer in use.
	IntegrationPlatformPhaseCreateCatalog IntegrationPlatformPhase = "CreateCatalog"

	// IntegrationPlatformConditionReady is the condition if the IntegrationPlatform is ready.
	// Deprecated: use IntegrationPlatformConditionTypeCreated.
	IntegrationPlatformConditionReady = "Ready"
	// IntegrationPlatformConditionTypeCreated is the condition if the IntegrationPlatform has been created.
	IntegrationPlatformConditionTypeCreated IntegrationPlatformConditionType = "Created"
	// IntegrationPlatformConditionTypeRegistryAvailable is the condition for the availability of a container registry.
	IntegrationPlatformConditionTypeRegistryAvailable IntegrationPlatformConditionType = "RegistryAvailable"
	// IntegrationPlatformConditionCamelCatalogAvailable is the condition for the availability of a the CamelCatalog.
	IntegrationPlatformConditionCamelCatalogAvailable IntegrationPlatformConditionType = "CamelCatalogAvailable"
	// IntegrationPlatformConditionKameletCatalogAvailable is the condition for the availability of a Kamelet catalog.
	IntegrationPlatformConditionKameletCatalogAvailable IntegrationPlatformConditionType = "KameletCatalogAvailable"
	// IntegrationPlatformConditionMavenSettingsAvailable is the condition for the availability of a Maven settings configuration.
	IntegrationPlatformConditionMavenSettingsAvailable IntegrationPlatformConditionType = "MavenSettingsAvailable"

	// IntegrationPlatformConditionCreatedReason represents the reason that the IntegrationPlatform is created.
	IntegrationPlatformConditionCreatedReason = "IntegrationPlatformCreated"
	// IntegrationPlatformConditionTypeRegistryAvailableReason represents the reason that the IntegrationPlatform Registry is available.
	IntegrationPlatformConditionTypeRegistryAvailableReason = "IntegrationPlatformRegistryAvailable"
	// IntegrationPlatformConditionCamelCatalogAvailableReason represents the reason that the IntegrationPlatform is created.
	IntegrationPlatformConditionCamelCatalogAvailableReason = "IntegrationPlatformCamelCatalogAvailable"
)

// IntegrationPlatformCondition describes the state of a resource at a certain point.
type IntegrationPlatformCondition struct {
	// Type of integration condition.
	Type IntegrationPlatformConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human-readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}
