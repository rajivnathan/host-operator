package nstemplatetiers

import (
	"context"
	"fmt"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	applycl "github.com/codeready-toolchain/toolchain-common/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type tierConfigResourcesManager struct {
	client    client.Client
	scheme    *runtime.Scheme
	namespace string
}

// listExistingResources returns a list of comparable  ToolchainObjects representing existing resources in the cluster
type listExistingResources func(cl client.Client) ([]applycl.ComparableToolchainObject, error)

// toolchainObjectKind represents a resource kind that should be present in templates containing cluster resources.
// Such a kind should be watched by NSTempalateSet controller which means that every change of the
// resources of that kind triggers a new reconcile.
// It is expected that the template contains only the kinds that are being watched and there is an instance of
// toolchainObjectKind type created in tierConfigResourceKinds list
type toolchainObjectKind struct {
	gvk                   schema.GroupVersionKind
	objectType            runtime.Object
	listExistingResources listExistingResources
}

func newToolchainObjectKind(gvk schema.GroupVersionKind, emptyObject runtime.Object, listExistingResources listExistingResources) toolchainObjectKind {
	return toolchainObjectKind{
		gvk:                   gvk,
		objectType:            emptyObject,
		listExistingResources: listExistingResources,
	}
}

var compareNotSupported applycl.CompareToolchainObjects = func(firstObject, secondObject applycl.ToolchainObject) (bool, error) {
	return false, fmt.Errorf("objects comparison is not supported")
}

// tierConfigResourceKinds is a list that contains definitions for all tier configuration resource toolchainObjectKinds
var tierConfigResourceKinds = []toolchainObjectKind{
	newToolchainObjectKind(
		toolchainv1alpha1.SchemeGroupVersion.WithKind("Deactivation"),
		&toolchainv1alpha1.Deactivation{},
		func(cl client.Client) ([]applycl.ComparableToolchainObject, error) {
			itemList := &toolchainv1alpha1.DeactivationList{}
			if err := cl.List(context.TODO(), itemList); err != nil {
				return nil, err
			}
			list := make([]applycl.ComparableToolchainObject, len(itemList.Items))
			for index := range itemList.Items {
				toolchainObject, err := applycl.NewComparableToolchainObject(&itemList.Items[index], compareNotSupported)
				if err != nil {
					return nil, err
				}
				list[index] = toolchainObject
			}
			return list, nil
		}),
}

// ensure ensures that the tier config resources exist.
// Returns `true, nil` if something was changed, `false, nil` if nothing changed, `false, err` if an error occurred
// func (r *tierConfigResourcesManager) ensure(logger logr.Logger, tier *toolchainv1alpha1.NSTemplateTier) (bool, error) {
// 	tierConfigLogger := logger.WithValues("tier", tier.GetName)
// 	tierConfigLogger.Info("ensuring tier configuration resources")
// 	// var tierTemplate *tierTemplate

// 	tierTemplate, err := r.getTierTemplate(tier.Spec.TierConfigResources.TemplateRef)
// 	if err != nil {
// 		return false, err
// 	}
// 	// if nsTmplSet.Spec.ClusterResources != nil {

// 	// }
// 	// go though all tier config resource kinds
// 	for _, tierConfigResourceKind := range tierConfigResourceKinds {
// 		gvkLogger := tierConfigLogger.WithValues("gvk", tierConfigResourceKind.gvk)
// 		gvkLogger.Info("ensuring tier config resource kind")
// 		newObjs := make([]applycl.ToolchainObject, 0)

// 		// get all objects of the resource kind from the template (if the template is specified)
// 		if tierTemplate != nil {
// 			newObjs, err = tierTemplate.process(r.scheme, retainObjectsOfSameGVK(tierConfigResourceKind.gvk))
// 			if err != nil {
// 				return false, err
// 			}
// 		}

// 		// list all existing objects of the tier config resource kind
// 		currentObjects, err := tierConfigResourceKind.listExistingResources(r.client)
// 		if err != nil {
// 			return false, err
// 		}

// 		// if there are more than one existing, then check if there is any that should be updated or deleted
// 		if len(currentObjects) > 0 {
// 			updatedOrDeleted, err := r.updateOrDeleteRedundant(gvkLogger, currentObjects, newObjs, tierTemplate, nsTmplSet)
// 			if err != nil {
// 				return false, err
// 			}
// 			if updatedOrDeleted {
// 				return true, err
// 			}
// 		}
// 		// if none was found to be either updated or deleted or if there is no existing object available,
// 		// then check if there is any object to be created
// 		if len(newObjs) > 0 {
// 			anyCreated, err := r.createMissing(gvkLogger, currentObjects, newObjs, tierTemplate, nsTmplSet)
// 			if err != nil {
// 				return false, err
// 			}
// 			if anyCreated {
// 				return true, nil
// 			}
// 		} else {
// 			gvkLogger.Info("no new tier config resources to create")
// 		}
// 	}

// 	tierConfigLogger.Info("tier config resources already provisioned")
// 	return false, nil
// }

// func retainObjectsOfSameGVK(gvk schema.GroupVersionKind) template.FilterFunc {
// 	return func(obj runtime.RawExtension) bool {
// 		return obj.Object.GetObjectKind().GroupVersionKind() == gvk
// 	}
// }

// var tierTemplatesCache = newTierTemplateCache()

// // getTierTemplate retrieves the TierTemplate resource with the given name
// // and returns an instance of the tierTemplate type for it whose template content can be parsable.
// // The returned tierTemplate contains all data from TierTemplate including its name.
// func (r *tierConfigResourcesManager) getTierTemplate(templateRef string) (*tierTemplate, error) {
// 	if templateRef == "" {
// 		return nil, fmt.Errorf("templateRef is not provided - it's not possible to fetch related TierTemplate resource")
// 	}
// 	if tierTmpl, ok := tierTemplatesCache.get(templateRef); ok && tierTmpl != nil {
// 		return tierTmpl, nil
// 	}
// 	tmpl, err := r.getToolchainTierTemplate(templateRef)
// 	if err != nil {
// 		return nil, err
// 	}
// 	tierTmpl := &tierTemplate{
// 		templateRef: templateRef,
// 		tierName:    tmpl.Spec.TierName,
// 		typeName:    tmpl.Spec.Type,
// 		template:    tmpl.Spec.Template,
// 	}
// 	tierTemplatesCache.add(tierTmpl)

// 	return tierTmpl, nil
// }

// // getToolchainTierTemplate gets the TierTemplate resource
// func (r *tierConfigResourcesManager) getToolchainTierTemplate(templateRef string) (*toolchainv1alpha1.TierTemplate, error) {

// 	tierTemplate := &toolchainv1alpha1.TierTemplate{}
// 	err := r.client.Get(context.TODO(), types.NamespacedName{
// 		Namespace: r.namespace,
// 		Name:      templateRef,
// 	}, tierTemplate)
// 	if err != nil {
// 		return nil, errors.Wrapf(err, "unable to retrieve the TierTemplate '%s'", templateRef)
// 	}
// 	return tierTemplate, nil
// }

// // updateOrDeleteRedundant takes the given currentObjs and newObjs and compares them.
// //
// // If there is any existing redundant resource (exist in the currentObjs, but not in the newObjs), then it deletes the resource and returns 'true, nil'.
// //
// // If there is any resource that is outdated (exists in both currentObjs and newObjs but its templateref is not matching),
// // then it updates the resource and returns 'true, nil'
// //
// // If no resource to be updated or deleted was found then it returns 'false, nil'. In case of any errors 'false, error'
// func (r *tierConfigResourcesManager) updateOrDeleteRedundant(logger logr.Logger, currentObjs []applycl.ComparableToolchainObject, newObjs []applycl.ToolchainObject, tierTemplate *tierTemplate, nsTmplSet *toolchainv1alpha1.NSTemplateSet) (bool, error) {
// 	// go though all current objects so we can compare then with the set of the requested and thus update the obsolete ones or delete redundant ones
// CurrentObjects:
// 	for _, currentObject := range currentObjs {

// 		// if the template is not specified, then delete all cluster resources one by one
// 		if nsTmplSet.Spec.ClusterResources == nil || tierTemplate == nil {
// 			return r.deleteClusterResource(nsTmplSet, currentObject)
// 		}

// 		// check if the object should still exist and should be updated
// 		for _, newObject := range newObjs {
// 			if newObject.HasSameName(currentObject) {
// 				// is found then let's check if we need to update it
// 				if !isUpToDate(currentObject, newObject, tierTemplate) {
// 					// let's update it
// 					if err := r.setStatusUpdatingIfNotProvisioning(nsTmplSet); err != nil {
// 						return false, err
// 					}
// 					return r.apply(logger, nsTmplSet, tierTemplate, newObject)
// 				}
// 				continue CurrentObjects
// 			}
// 		}
// 		// is not found then let's delete it
// 		return r.deleteClusterResource(nsTmplSet, currentObject)
// 	}
// 	return false, nil
// }

// // deleteClusterResource sets status to updating, deletes the given resource and returns 'true, nil'. In case of any errors 'false, error'.
// func (r *tierConfigResourcesManager) deleteClusterResource(nsTmplSet *toolchainv1alpha1.NSTemplateSet, toDelete applycl.ToolchainObject) (bool, error) {
// 	if err := r.setStatusUpdatingIfNotProvisioning(nsTmplSet); err != nil {
// 		return false, err
// 	}
// 	if err := r.client.Delete(context.TODO(), toDelete.GetRuntimeObject()); err != nil {
// 		return false, errs.Wrapf(err, "failed to delete an existing redundant cluster resource of name '%s' and gvk '%v'",
// 			toDelete.GetName(), toDelete.GetGvk())
// 	}
// 	return true, nil
// }

// // createMissing takes the given currentObjs and newObjs and compares them if there is any that should be created.
// // If such a object is found, then it creates it and returns 'true, nil'. If no missing resource was found then returns 'false, nil'.
// // In case of any error 'false, error'
// func (r *tierConfigResourcesManager) createMissing(logger logr.Logger, currentObjs []applycl.ComparableToolchainObject, newObjs []applycl.ToolchainObject, tierTemplate *tierTemplate, nsTmplSet *toolchainv1alpha1.NSTemplateSet) (bool, error) {
// 	// go though all new (expected) objects to check if all of them already exist or not
// NewObjects:
// 	for _, newObject := range newObjs {

// 		// go through current objects to check if is one of the new (expected)
// 		for _, currentObject := range currentObjs {
// 			// if the name is the same, then it means that it already exist so just continue with the next new object
// 			if newObject.HasSameName(currentObject) {
// 				continue NewObjects
// 			}
// 		}
// 		// if there was no existing object found that would match with the new one, then set the status appropriately
// 		namespaces, err := fetchNamespaces(r.client, nsTmplSet.Name)
// 		if err != nil {
// 			return false, errs.Wrapf(err, "unable to fetch user's namespaces")
// 		}
// 		// if there is any existing namespace, then set the status to updating
// 		if len(namespaces) == 0 {
// 			if err := r.setStatusProvisioningIfNotUpdating(nsTmplSet); err != nil {
// 				return false, err
// 			}
// 		} else {
// 			// otherwise, to provisioning
// 			if err := r.setStatusUpdatingIfNotProvisioning(nsTmplSet); err != nil {
// 				return false, err
// 			}
// 		}
// 		// and create the object
// 		return r.apply(logger, nsTmplSet, tierTemplate, newObject)
// 	}
// 	return false, nil
// }

// // isUpToDate returns true if the currentObject uses the corresponding templateRef and equals to the new object
// func isUpToDate(currentObject, _ applycl.ToolchainObject, tierTemplate *tierTemplate) bool {
// 	return currentObject.GetLabels()[toolchainv1alpha1.TemplateRefLabelKey] != "" &&
// 		currentObject.GetLabels()[toolchainv1alpha1.TemplateRefLabelKey] == tierTemplate.templateRef
// 	// && currentObject.IsSame(newObject)  <-- TODO Uncomment when IsSame is implemented for all ToolchainObjects!
// }

// // tierTemplate contains all data from TierTemplate including its name
// type tierTemplate struct {
// 	templateRef string
// 	tierName    string
// 	typeName    string
// 	template    templatev1.Template
// }

// // process processes the template inside of the tierTemplate object and replaces the USERNAME variable with the given username.
// // Optionally, it also filters the result to return a subset of the template objects.
// func (t *tierTemplate) process(scheme *runtime.Scheme, filters ...template.FilterFunc) ([]applycl.ToolchainObject, error) {
// 	tmplProcessor := template.NewProcessor(scheme)
// 	params := map[string]string{}
// 	return tmplProcessor.Process(t.template.DeepCopy(), params, filters...)
// }

// type tierTemplateCache struct {
// 	sync.RWMutex
// 	// tierTemplatesByTemplateRef contains tierTemplatesByTemplateRef mapped by TemplateRef key
// 	tierTemplatesByTemplateRef map[string]*tierTemplate
// }

// func newTierTemplateCache() *tierTemplateCache {
// 	return &tierTemplateCache{
// 		tierTemplatesByTemplateRef: map[string]*tierTemplate{},
// 	}
// }

// func (c *tierTemplateCache) get(templateRef string) (*tierTemplate, bool) {
// 	c.RLock()
// 	defer c.RUnlock()
// 	tierTemplate, ok := c.tierTemplatesByTemplateRef[templateRef]
// 	return tierTemplate, ok
// }

// func (c *tierTemplateCache) add(tierTemplate *tierTemplate) {
// 	c.Lock()
// 	defer c.Unlock()
// 	c.tierTemplatesByTemplateRef[tierTemplate.templateRef] = tierTemplate
// }
