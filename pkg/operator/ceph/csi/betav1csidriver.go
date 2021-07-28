/*
Copyright 2021 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
TODO: storage.k8s.io/v1beta1 CSIDriver is deprecated in Kubernetes v1.19+, unavailable in v1.22+;
Once the support of older Kubernetes releases are removed in Rook, delete the file to
remove the support for the betav1 CSIDriver object.
*/

package csi

import (
	"context"

	"github.com/pkg/errors"
	betav1k8scsi "k8s.io/api/storage/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/typed/storage/v1beta1"
)

type beta1CsiDriver struct {
	csiDriver *betav1k8scsi.CSIDriver
	csiClient v1beta1.CSIDriverInterface
}

// createCSIDriverInfo Registers CSI driver by creating a CSIDriver object
func (d beta1CsiDriver) createCSIDriverInfo(ctx context.Context, clientset kubernetes.Interface, name, fsGroupPolicy string) error {
	attach := true
	mountInfo := false
	// Create CSIDriver object
	csiDriver := &betav1k8scsi.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: betav1k8scsi.CSIDriverSpec{
			AttachRequired: &attach,
			PodInfoOnMount: &mountInfo,
		},
	}
	if fsGroupPolicy != "" {
		policy := betav1k8scsi.FSGroupPolicy(fsGroupPolicy)
		csiDriver.Spec.FSGroupPolicy = &policy
	}
	csidrivers := clientset.StorageV1beta1().CSIDrivers()
	driver, err := csidrivers.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = csidrivers.Create(ctx, csiDriver, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			logger.Infof("CSIDriver object created for driver %q", name)
		}
		return err
	}

	// As FSGroupPolicy field is immutable, should be set only during create time.
	// if the request is to change the FSGroupPolicy, we are deleting the CSIDriver object and creating it.
	if driver.Spec.FSGroupPolicy != nil && csiDriver.Spec.FSGroupPolicy != nil && *driver.Spec.FSGroupPolicy != *csiDriver.Spec.FSGroupPolicy {
		return d.reCreateCSIDriverInfo(ctx)
	}

	// For csidriver we need to provide the resourceVersion when updating the object.
	// From the docs (https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#metadata)
	// > "This value MUST be treated as opaque by clients and passed unmodified back to the server"
	csiDriver.ObjectMeta.ResourceVersion = driver.ObjectMeta.ResourceVersion
	_, err = csidrivers.Update(ctx, csiDriver, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	logger.Infof("CSIDriver object updated for driver %q", name)
	return nil
}

func (d beta1CsiDriver) reCreateCSIDriverInfo(ctx context.Context) error {
	csiDriver := d.csiDriver
	csiClient := d.csiClient
	err := csiClient.Delete(ctx, csiDriver.Name, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to delete CSIDriver object for driver %q", csiDriver.Name)
	}
	logger.Infof("CSIDriver object deleted for driver %q", csiDriver.Name)
	_, err = csiClient.Create(ctx, csiDriver, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to recreate CSIDriver object for driver %q", csiDriver.Name)
	}
	logger.Infof("CSIDriver object recreated for driver %q", csiDriver.Name)
	return nil
}

// deleteCSIDriverInfo deletes CSIDriverInfo and returns the error if any
func (d beta1CsiDriver) deleteCSIDriverInfo(ctx context.Context, clientset kubernetes.Interface, name string) error {
	err := clientset.StorageV1beta1().CSIDrivers().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debugf("%q CSIDriver not found; skipping deletion.", name)
			return nil
		}
	}
	return err
}
