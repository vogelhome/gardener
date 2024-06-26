// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package deletionconfirmation

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/hashicorp/go-multierror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/admission"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener/pkg/apis/core"
	admissioninitializer "github.com/gardener/gardener/pkg/apiserver/admission/initializer"
	"github.com/gardener/gardener/pkg/client/core/clientset/versioned"
	gardencoreinformers "github.com/gardener/gardener/pkg/client/core/informers/externalversions"
	gardencorev1beta1listers "github.com/gardener/gardener/pkg/client/core/listers/core/v1beta1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	plugin "github.com/gardener/gardener/plugin/pkg"
)

// Register registers a plugin.
func Register(plugins *admission.Plugins) {
	plugins.Register(plugin.PluginNameDeletionConfirmation, NewFactory)
}

// NewFactory creates a new PluginFactory.
func NewFactory(_ io.Reader) (admission.Interface, error) {
	return New()
}

// DeletionConfirmation contains an admission handler and listers.
type DeletionConfirmation struct {
	*admission.Handler
	gardenCoreClient versioned.Interface
	shootLister      gardencorev1beta1listers.ShootLister
	shootStateLister gardencorev1beta1listers.ShootStateLister
	projectLister    gardencorev1beta1listers.ProjectLister
	readyFunc        admission.ReadyFunc
}

var (
	_ = admissioninitializer.WantsCoreInformerFactory(&DeletionConfirmation{})
	_ = admissioninitializer.WantsCoreClientSet(&DeletionConfirmation{})

	readyFuncs []admission.ReadyFunc
)

// New creates a new DeletionConfirmation admission plugin.
func New() (*DeletionConfirmation, error) {
	return &DeletionConfirmation{
		Handler: admission.NewHandler(admission.Delete),
	}, nil
}

// AssignReadyFunc assigns the ready function to the admission handler.
func (d *DeletionConfirmation) AssignReadyFunc(f admission.ReadyFunc) {
	d.readyFunc = f
	d.SetReadyFunc(f)
}

// SetCoreInformerFactory gets Lister from SharedInformerFactory.
func (d *DeletionConfirmation) SetCoreInformerFactory(f gardencoreinformers.SharedInformerFactory) {
	shootInformer := f.Core().V1beta1().Shoots()
	d.shootLister = shootInformer.Lister()

	projectInformer := f.Core().V1beta1().Projects()
	d.projectLister = projectInformer.Lister()

	shootStateInformer := f.Core().V1beta1().ShootStates()
	d.shootStateLister = shootStateInformer.Lister()

	readyFuncs = append(
		readyFuncs,
		shootInformer.Informer().HasSynced,
		projectInformer.Informer().HasSynced,
		shootStateInformer.Informer().HasSynced,
	)
}

// SetCoreClientSet gets the clientset from the Kubernetes client.
func (d *DeletionConfirmation) SetCoreClientSet(c versioned.Interface) {
	d.gardenCoreClient = c
}

// ValidateInitialization checks whether the plugin was correctly initialized.
func (d *DeletionConfirmation) ValidateInitialization() error {
	if d.shootLister == nil {
		return errors.New("missing shoot lister")
	}
	if d.projectLister == nil {
		return errors.New("missing project lister")
	}
	if d.shootStateLister == nil {
		return errors.New("missing shootState lister")
	}
	if d.gardenCoreClient == nil {
		return errors.New("missing gardener internal core client")
	}
	return nil
}

var _ admission.ValidationInterface = &DeletionConfirmation{}

// Validate makes admissions decisions based on deletion confirmation annotation.
func (d *DeletionConfirmation) Validate(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	var (
		obj         client.Object
		listFunc    func() ([]client.Object, error)
		cacheLookup func() (client.Object, error)
		liveLookup  func() (client.Object, error)
		checkFunc   func(client.Object) error
	)

	switch a.GetKind().GroupKind() {
	case core.Kind("Shoot"):
		listFunc = func() ([]client.Object, error) {
			list, err := d.shootLister.Shoots(a.GetNamespace()).List(labels.Everything())
			if err != nil {
				return nil, err
			}
			result := make([]client.Object, 0, len(list))
			for _, obj := range list {
				result = append(result, obj)
			}
			return result, nil
		}
		cacheLookup = func() (client.Object, error) {
			return d.shootLister.Shoots(a.GetNamespace()).Get(a.GetName())
		}
		liveLookup = func() (client.Object, error) {
			return d.gardenCoreClient.CoreV1beta1().Shoots(a.GetNamespace()).Get(ctx, a.GetName(), kubernetes.DefaultGetOptions())
		}
		checkFunc = gardenerutils.CheckIfDeletionIsConfirmed

	case core.Kind("Project"):
		listFunc = func() ([]client.Object, error) {
			list, err := d.projectLister.List(labels.Everything())
			if err != nil {
				return nil, err
			}
			result := make([]client.Object, 0, len(list))
			for _, obj := range list {
				result = append(result, obj)
			}
			return result, nil
		}
		cacheLookup = func() (client.Object, error) {
			return d.projectLister.Get(a.GetName())
		}
		liveLookup = func() (client.Object, error) {
			return d.gardenCoreClient.CoreV1beta1().Projects().Get(ctx, a.GetName(), kubernetes.DefaultGetOptions())
		}
		checkFunc = gardenerutils.CheckIfDeletionIsConfirmed

	case core.Kind("ShootState"):
		listFunc = func() ([]client.Object, error) {
			list, err := d.shootStateLister.ShootStates(a.GetNamespace()).List(labels.Everything())
			if err != nil {
				return nil, err
			}
			result := make([]client.Object, 0, len(list))
			for _, obj := range list {
				result = append(result, obj)
			}
			return result, nil
		}
		cacheLookup = func() (client.Object, error) {
			return d.shootStateLister.ShootStates(a.GetNamespace()).Get(a.GetName())
		}
		liveLookup = func() (client.Object, error) {
			return d.gardenCoreClient.CoreV1beta1().ShootStates(a.GetNamespace()).Get(ctx, a.GetName(), kubernetes.DefaultGetOptions())
		}
		checkFunc = gardenerutils.CheckIfDeletionIsConfirmed

	default:
		return nil
	}

	// Wait until the caches have been synced
	if d.readyFunc == nil {
		d.AssignReadyFunc(func() bool {
			for _, readyFunc := range readyFuncs {
				if !readyFunc() {
					return false
				}
			}
			return true
		})
	}
	if !d.WaitForReady() {
		return admission.NewForbidden(a, errors.New("not yet ready to handle request"))
	}

	// The "delete endpoint" handler of the k8s.io/apiserver library calls the admission controllers
	// handling DELETECOLLECTION requests with empty resource names:
	// https://github.com/kubernetes/apiserver/blob/kubernetes-1.12.1/pkg/endpoints/handlers/delete.go#L265-L283
	// Consequently, a.GetName() equals "". This is for the admission controllers to know that all
	// resources of this kind shall be deleted. We only allow this request if all objects have been
	// properly annotated with the deletion confirmation.
	if a.GetName() == "" {
		objList, err := listFunc()
		if err != nil {
			return err
		}

		var (
			wg     sync.WaitGroup
			result error
			output = make(chan error)
		)

		for _, obj := range objList {
			wg.Add(1)

			go func(obj client.Object) {
				defer wg.Done()
				output <- d.Validate(ctx, admission.NewAttributesRecord(a.GetObject(), a.GetOldObject(), a.GetKind(), a.GetNamespace(), obj.GetName(), a.GetResource(), a.GetSubresource(), a.GetOperation(), a.GetOperationOptions(), a.IsDryRun(), a.GetUserInfo()), o)
			}(obj)
		}

		go func() {
			wg.Wait()
			close(output)
		}()

		for out := range output {
			if out != nil {
				result = multierror.Append(result, out)
			}
		}

		if result == nil {
			return nil
		}
		return admission.NewForbidden(a, result)
	}

	// Read the object from the cache
	obj, err := cacheLookup()
	if err == nil {
		if checkFunc(obj) == nil {
			return nil
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	// If the first try does not succeed we do a live lookup to really ensure that the deletion cannot be processed
	// (similar to what we do in the ResourceReferenceManager when ensuring the existence of a secret).
	// This is to allow clients to send annotate+delete requests subsequently very fast.
	obj, err = liveLookup()
	if err != nil {
		return err
	}

	if err := checkFunc(obj); err != nil {
		return admission.NewForbidden(a, err)
	}
	return nil
}
