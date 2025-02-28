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

package client

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-provider-gcp/pkg/gcp"
)

var (
	_ Factory = &factory{}
)

// Factory is a factory that can produce clients for various GCP Services.
type Factory interface {
	// DNS returns a GCP cloud DNS service client.
	DNS(context.Context, client.Client, corev1.SecretReference) (DNSClient, error)
	// Storage returns a GCP (blob) storage client.
	Storage(context.Context, client.Client, corev1.SecretReference) (StorageClient, error)
	// Compute returns a GCP compute client.
	Compute(context.Context, client.Client, corev1.SecretReference) (ComputeClient, error)
	// IAM returns a GCP compute client.
	IAM(context.Context, client.Client, corev1.SecretReference) (IAMClient, error)
}

type factory struct{}

// New returns a new instance of Factory.
func New() Factory {
	return &factory{}
}

// DNS returns a GCP cloud DNS service client.
func (f factory) DNS(ctx context.Context, c client.Client, sr corev1.SecretReference) (DNSClient, error) {
	serviceAccount, err := gcp.GetServiceAccountFromSecretReference(ctx, c, sr)
	if err != nil {
		return nil, err
	}
	return NewDNSClient(ctx, serviceAccount)
}

// Storage reads the secret from the passed reference and returns a GCP (blob) storage client.
func (f factory) Storage(ctx context.Context, c client.Client, sr corev1.SecretReference) (StorageClient, error) {
	serviceAccount, err := gcp.GetServiceAccountFromSecretReference(ctx, c, sr)
	if err != nil {
		return nil, err
	}
	return NewStorageClient(ctx, serviceAccount)
}

// Compute reads the secret from the passed reference and returns a GCP compute client.
func (f factory) Compute(ctx context.Context, c client.Client, sr corev1.SecretReference) (ComputeClient, error) {
	serviceAccount, err := gcp.GetServiceAccountFromSecretReference(ctx, c, sr)
	if err != nil {
		return nil, err
	}
	return NewComputeClient(ctx, serviceAccount)
}

// IAM reads the secret from the passed reference and returns a GCP compute client.
func (f factory) IAM(ctx context.Context, c client.Client, sr corev1.SecretReference) (IAMClient, error) {
	serviceAccount, err := gcp.GetServiceAccountFromSecretReference(ctx, c, sr)
	if err != nil {
		return nil, err
	}
	return NewIAMClient(ctx, serviceAccount)
}
