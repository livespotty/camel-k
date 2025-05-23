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

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

// IntegrationPlatformKameletSpecApplyConfiguration represents a declarative configuration of the IntegrationPlatformKameletSpec type for use
// with apply.
type IntegrationPlatformKameletSpecApplyConfiguration struct {
	Repositories []KameletRepositorySpecApplyConfiguration `json:"repositories,omitempty"`
}

// IntegrationPlatformKameletSpecApplyConfiguration constructs a declarative configuration of the IntegrationPlatformKameletSpec type for use with
// apply.
func IntegrationPlatformKameletSpec() *IntegrationPlatformKameletSpecApplyConfiguration {
	return &IntegrationPlatformKameletSpecApplyConfiguration{}
}

// WithRepositories adds the given value to the Repositories field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Repositories field.
func (b *IntegrationPlatformKameletSpecApplyConfiguration) WithRepositories(values ...*KameletRepositorySpecApplyConfiguration) *IntegrationPlatformKameletSpecApplyConfiguration {
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithRepositories")
		}
		b.Repositories = append(b.Repositories, *values[i])
	}
	return b
}
