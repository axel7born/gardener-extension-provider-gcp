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

package backupbucket

import (
	"context"

	"github.com/gardener/gardener/extensions/pkg/controller/backupbucket"
	"github.com/gardener/gardener/extensions/pkg/util"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-extension-provider-gcp/pkg/apis/gcp/helper"
	gcpclient "github.com/gardener/gardener-extension-provider-gcp/pkg/gcp/client"
)

type actuator struct {
	backupbucket.Actuator
	client client.Client
}

func newActuator(mgr manager.Manager) backupbucket.Actuator {
	return &actuator{
		client: mgr.GetClient(),
	}
}

func (a *actuator) Reconcile(ctx context.Context, _ logr.Logger, bb *extensionsv1alpha1.BackupBucket) error {
	storageClient, err := gcpclient.NewStorageClientFromSecretRef(ctx, a.client, bb.Spec.SecretRef)
	if err != nil {
		return util.DetermineError(err, helper.KnownCodes)
	}

	return util.DetermineError(storageClient.CreateBucketIfNotExists(ctx, bb.Name, bb.Spec.Region), helper.KnownCodes)
}

func (a *actuator) Delete(ctx context.Context, _ logr.Logger, bb *extensionsv1alpha1.BackupBucket) error {
	storageClient, err := gcpclient.NewStorageClientFromSecretRef(ctx, a.client, bb.Spec.SecretRef)
	if err != nil {
		return util.DetermineError(err, helper.KnownCodes)
	}

	return util.DetermineError(storageClient.DeleteBucketIfExists(ctx, bb.Name), helper.KnownCodes)
}
