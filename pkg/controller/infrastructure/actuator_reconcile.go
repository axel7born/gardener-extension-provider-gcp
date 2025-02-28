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

package infrastructure

import (
	"context"
	"fmt"

	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/terraformer"
	"github.com/gardener/gardener/extensions/pkg/util"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/go-logr/logr"

	"github.com/gardener/gardener-extension-provider-gcp/pkg/apis/gcp/helper"
	"github.com/gardener/gardener-extension-provider-gcp/pkg/controller/infrastructure/infraflow"
)

// Reconcile implements infrastructure.Actuator.
func (a *actuator) Reconcile(ctx context.Context, log logr.Logger, infra *extensionsv1alpha1.Infrastructure, cluster *controller.Cluster) error {
	return util.DetermineError(a.reconcile(ctx, log, infra, cluster, terraformer.StateConfigMapInitializerFunc(terraformer.CreateState)), helper.KnownCodes)
}

func (a *actuator) reconcile(ctx context.Context, log logr.Logger, infra *extensionsv1alpha1.Infrastructure, cluster *controller.Cluster, terraformState terraformer.StateConfigMapInitializer) error {
	useFlow, err := shouldUseFlow(infra, cluster)
	if err != nil {
		return err
	}

	// Terraform case
	if !useFlow {
		reconciler := NewTerraformReconciler(a.client, a.restConfig, terraformState, a.disableProjectedTokenMount)
		status, state, err := reconciler.Reconcile(ctx, log, cluster, infra)
		if err != nil {
			return err
		}

		return a.updateProviderStatus(ctx, infra, status, state)
	}

	// Flow case
	if err := a.cleanupTerraformerResources(ctx, log, infra); err != nil {
		return fmt.Errorf("cleaning up terraformer resources failed: %w", err)
	}

	flow, err := infraflow.NewFlowReconciler(ctx, log, infra, cluster, a.client)
	if err != nil {
		return err
	}

	status, state, err := flow.Reconcile(ctx)
	if err != nil {
		return err
	}

	return a.updateProviderStatus(ctx, infra, status, state)
}
