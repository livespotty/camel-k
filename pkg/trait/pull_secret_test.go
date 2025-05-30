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

package trait

import (
	"context"
	"testing"

	v1 "github.com/apache/camel-k/v2/pkg/apis/camel/v1"
	"github.com/apache/camel-k/v2/pkg/util/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/apache/camel-k/v2/pkg/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullSecret(t *testing.T) {
	e, deployment := getEnvironmentAndDeployment(t)

	trait, _ := newPullSecretTrait().(*pullSecretTrait)
	trait.SecretName = "xxxy"
	enabled, condition, err := trait.Configure(e)
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.Nil(t, condition)

	err = trait.Apply(e)
	require.NoError(t, err)
	assert.Contains(t, deployment.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: "xxxy"})
}

func TestPullSecretDoesNothingWhenNotSetOnPlatform(t *testing.T) {
	e, _ := getEnvironmentAndDeployment(t)
	e.Platform = &v1.IntegrationPlatform{}

	trait := newPullSecretTrait()
	enabled, condition, err := trait.Configure(e)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Nil(t, condition)
}

func TestPullSecretAuto(t *testing.T) {
	e, _ := getEnvironmentAndDeployment(t)

	trait, _ := newPullSecretTrait().(*pullSecretTrait)
	trait.Auto = ptr.To(false)
	enabled, condition, err := trait.Configure(e)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Nil(t, condition)
}

func TestPullSecretImagePullerDelegation(t *testing.T) {
	e, _ := getEnvironmentAndDeployment(t)

	trait, _ := newPullSecretTrait().(*pullSecretTrait)
	trait.Auto = ptr.To(false)
	trait.ImagePullerDelegation = ptr.To(true)
	enabled, condition, err := trait.Configure(e)
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.Nil(t, condition)
	assert.True(t, *trait.ImagePullerDelegation)

	err = trait.Apply(e)
	require.NoError(t, err)

	var roleBinding rbacv1.RoleBinding
	roleBindingKey := client.ObjectKey{
		Namespace: "test",
		Name:      "camel-k-puller-test-default",
	}
	err = e.Client.Get(e.Ctx, roleBindingKey, &roleBinding)
	require.NoError(t, err)
	assert.Len(t, roleBinding.Subjects, 1)
}

func getEnvironmentAndDeployment(t *testing.T) (*Environment, *appsv1.Deployment) {
	t.Helper()

	e := &Environment{}
	e.Integration = &v1.Integration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "myit",
		},
		Status: v1.IntegrationStatus{
			Phase: v1.IntegrationPhaseDeploying,
		},
	}

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "myit",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{},
			},
		},
	}
	e.Resources = kubernetes.NewCollection(&deployment)

	var err error
	e.Ctx = context.TODO()
	e.Client, err = internal.NewFakeClient(e.Integration, &deployment)
	require.NoError(t, err)

	return e, &deployment
}
