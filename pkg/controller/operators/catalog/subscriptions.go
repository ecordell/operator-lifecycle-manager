package catalog

import (
	"errors"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
)

var (
	ErrNilSubscription = errors.New("invalid Subscription object: <nil>")
)

const (
	PackageLabel = "olm-package"
	CatalogLabel = "olm-catalog"
	ChannelLabel = "olm-channel"
)

func ensureLabels(sub *v1alpha1.Subscription) *v1alpha1.Subscription {
	labels := sub.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[PackageLabel] = sub.Spec.Package
	labels[CatalogLabel] = sub.Spec.CatalogSource
	labels[ChannelLabel] = sub.Spec.Channel
	sub.SetLabels(labels)
	return sub
}
