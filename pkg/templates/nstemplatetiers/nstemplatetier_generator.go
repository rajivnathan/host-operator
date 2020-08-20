package nstemplatetiers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"
	commonTemplate "github.com/codeready-toolchain/toolchain-common/pkg/template"

	templatev1 "github.com/openshift/api/template/v1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("templates")

// CreateOrUpdateResources generates the NSTemplateTier resources from the namespace templates,
// then uses the manager's client to create or update the resources on the cluster.
func CreateOrUpdateResources(s *runtime.Scheme, client client.Client, namespace string, assets assets.Assets) error {

	// load templates from assets
	templatesByTier, err := loadTemplatesByTiers(assets)
	if err != nil {
		return err
	}

	// create the TierTemplates
	err = newTierTemplates(s, namespace, templatesByTier)
	if err != nil {
		return errors.Wrap(err, "unable to create TierTemplates")
	}

	for _, tierTmpls := range templatesByTier {
		for _, tierTmpl := range tierTmpls.tierTemplates {
			// using the "standard" client since we don't need to support updates on such resources, they should be immutable
			if err := client.Create(context.TODO(), tierTmpl); err != nil && !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "unable to create the '%s' TierTemplate in namespace '%s'", tierTmpl.Name, tierTmpl.Namespace)
			}
			log.Info("TierTemplate resource created", "namespace", tierTmpl.Namespace, "name", tierTmpl.Name)
		}
	}
	// create the NSTemplateTiers
	nstmplTiersByTier, err := newNSTemplateTiers(s, namespace, templatesByTier)
	if err != nil {
		return errors.Wrap(err, "unable to create NSTemplateTiers")
	}

	for tierName, nstmplTierObjs := range nstmplTiersByTier {
		labels := map[string]string{
			toolchainv1alpha1.ProviderLabelKey: toolchainv1alpha1.ProviderLabelValue,
		}
		_, err = commonclient.NewApplyClient(client, s).Apply(nstmplTierObjs, labels)
		if err != nil {
			return errors.Wrapf(err, "unable to create the '%s' NSTemplateTier", tierName)
		}
	}

	return nil
}

type tierContents struct {
	templates     *templates
	tierTemplates []*toolchainv1alpha1.TierTemplate
}

// templates: namespaces and other cluster-scoped resources belonging to a given tier ("advanced", "basic", "team", etc.)
type templates struct {
	namespaceTemplates map[string]template // namespace templates (including roles, etc.) indexed by type ("dev", "code", "stage")
	clusterTemplate    *template           // other cluster-scoped resources, in a single template file
	nsTemplateTier     *template           // NSTemplateTier resource with tier-scoped configuration in its spec, in a single template file
}

// template: a template content and its latest git revision
type template struct {
	revision string
	content  []byte
}

// loadAssetsByTiers loads the assets and dispatches them by tiers, assuming the given `assets` has the following structure:
//
// metadata.yaml
// advanced/
//   cluster.yaml
//   ns_code.yaml
//   ns_xyz.yaml
// basic/
//   cluster.yaml
//   ns_code.yaml
//   ns_xyz.yaml
// team/
//   ns_code.yaml
//   ns_xyz.yaml
//
// The output is a map of `templates` indexed by tier.
// Each `templates` object contains itself a map of `template` objects indexed by the namespace type (`namespaceTemplates`)
// and an optional `template` for the cluster resources (`clusterTemplate`).
// Each `template` object contains a `revision` (`string`) and the `content` of the template to apply (`[]byte`)
func loadTemplatesByTiers(assets assets.Assets) (map[string]*tierContents, error) {
	metadataContent, err := assets.Asset("metadata.yaml")
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load templates")
	}
	metadata := make(map[string]string)
	err = yaml.Unmarshal([]byte(metadataContent), &metadata)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to load templates")
	}

	results := make(map[string]*tierContents)
	for _, name := range assets.Names() {
		if name == "metadata.yaml" {
			continue
		}
		// split the name using the `/` separator
		parts := strings.Split(name, "/")
		// skip any name that does not have 2 parts
		if len(parts) != 2 {
			return nil, errors.Wrapf(err, "unable to load templates: invalid name format for file '%s'", name)
		}
		tier := parts[0]
		filename := parts[1]
		if _, exists := results[tier]; !exists {
			results[tier] = &tierContents{
				templates: &templates{
					namespaceTemplates: map[string]template{},
				},
			}
		}
		content, err := assets.Asset(name)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to load templates")
		}
		tmpl := template{
			revision: metadata[strings.TrimSuffix(name, ".yaml")],
			content:  content,
		}
		switch {
		case strings.HasPrefix(filename, "ns_"):
			kind := strings.TrimSuffix(strings.TrimPrefix(filename, "ns_"), ".yaml")
			results[tier].templates.namespaceTemplates[kind] = tmpl
		case filename == "cluster.yaml":
			results[tier].templates.clusterTemplate = &tmpl
		case filename == "tier.yaml":
			results[tier].templates.nsTemplateTier = &tmpl
		default:
			return nil, errors.Errorf("unable to load templates: unknown scope for file '%s'", name)
		}
	}

	return results, nil
}

// newTierTemplates generates all TierTemplate resources, and adds them to the tier map indexed by tier name
func newTierTemplates(s *runtime.Scheme, namespace string, templatesByTier map[string]*tierContents) error {
	decoder := serializer.NewCodecFactory(s).UniversalDeserializer()

	// process tiers in alphabetical order
	tiers := make([]string, 0, len(templatesByTier))
	for tier := range templatesByTier {
		tiers = append(tiers, tier)
	}
	sort.Strings(tiers)
	for _, tier := range tiers {
		tierTmpls := []*toolchainv1alpha1.TierTemplate{}
		tierData := templatesByTier[tier]
		// namespace templates
		kinds := make([]string, 0, len(tierData.templates.namespaceTemplates))
		for kind := range tierData.templates.namespaceTemplates {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			tmpl := tierData.templates.namespaceTemplates[kind]
			tierTmpl, err := newTierTemplate(decoder, namespace, tier, kind, tmpl)
			if err != nil {
				return err
			}
			tierTmpls = append(tierTmpls, tierTmpl)
		}
		// cluster resources templates
		if tierData.templates.clusterTemplate != nil {
			tierTmpl, err := newTierTemplate(decoder, namespace, tier, toolchainv1alpha1.ClusterResourcesTemplateType, *tierData.templates.clusterTemplate)
			if err != nil {
				return err
			}
			tierTmpls = append(tierTmpls, tierTmpl)
		}
		templatesByTier[tier].tierTemplates = tierTmpls
	}
	return nil
}

// newTierTemplate generates a TierTemplate resource for a given tier and kind
func newTierTemplate(decoder runtime.Decoder, namespace, tier, kind string, tmpl template) (*toolchainv1alpha1.TierTemplate, error) {
	name := NewTierTemplateName(tier, kind, tmpl.revision)
	tmplObj := &templatev1.Template{}
	_, _, err := decoder.Decode(tmpl.content, nil, tmplObj)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to generate '%s' TierTemplate manifest", name)
	}
	return &toolchainv1alpha1.TierTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name, // link to the TierTemplate resource, whose name is: `<tierName>-<nsType>-<revision>`,
		},
		Spec: toolchainv1alpha1.TierTemplateSpec{
			Revision: tmpl.revision,
			TierName: tier,
			Type:     kind,
			Template: *tmplObj,
		},
	}, nil
}

// NewTierTemplateName a utility func to generate a TierTemplate name, based on the given tier, kind and revision.
// note: the resource name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character
func NewTierTemplateName(tier, kind, revision string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s-%s", tier, kind, revision))
}

// newNSTemplateTiers generates all NSTemplateTier resources, indexed by their associated tier
func newNSTemplateTiers(s *runtime.Scheme, namespace string, templatesByTier map[string]*tierContents) (map[string][]commonclient.ToolchainObject, error) {

	tiers := make(map[string][]commonclient.ToolchainObject, len(templatesByTier))
	for tier, tmpls := range templatesByTier {
		tmpl, err := newNSTemplateTier(s, namespace, tier, tmpls)
		if err != nil {
			return nil, err
		}
		tiers[tier] = tmpl
	}
	return tiers, nil
}

// NewNSTemplateTier initializes a complete NSTemplateTier object via Openshift Template
// by embedding the `<tier>-code.yml`, `<tier>-dev.yml` and `<tier>-stage.yml` references.
//
// Something like:
// ------
// kind: NSTemplateTier
//   metadata:
//     name: basic
//   spec:
//     clusterResources:
//       templateRef: basic-clusterresources-07cac69
//     deactivationTimeoutDays: 30
//     namespaces:
//     - templateRef: basic-code-cb6fbd2
//     - templateRef: basic-dev-4d49fe0
//     - templateRef: basic-stage-4d49fe0
// ------
func newNSTemplateTier(s *runtime.Scheme, namespace, tier string, contents *tierContents) ([]commonclient.ToolchainObject, error) {
	decoder := serializer.NewCodecFactory(s).UniversalDeserializer()
	if contents.templates.nsTemplateTier == nil {
		return nil, fmt.Errorf("tier %s is missing a tier.yaml file", tier)
	}

	tmplObj := &templatev1.Template{}
	_, _, err := decoder.Decode(contents.templates.nsTemplateTier.content, nil, tmplObj)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to generate '%s' NSTemplateTier manifest", tier)
	}

	tmplProcessor := commonTemplate.NewProcessor(s)
	params := map[string]string{"NAMESPACE": namespace}

	for _, tierTmpl := range contents.tierTemplates {
		switch tierTmpl.Spec.Type {
		// ClusterResources
		case toolchainv1alpha1.ClusterResourcesTemplateType:
			params["CLUSTER_TEMPL_REF"] = tierTmpl.Name
		// Namespaces
		default:
			tmplType := strings.ToUpper(tierTmpl.Spec.Type) // code, dev, stage
			key := tmplType + "_TEMPL_REF"                  // eg. CODE_TEMPL_REF
			params[key] = tierTmpl.Name
			fmt.Printf("key %s value: %s\n", key, tierTmpl.Name)
		}
	}
	return tmplProcessor.Process(tmplObj.DeepCopy(), params)
}
