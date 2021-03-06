package kustomize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	v1 "github.com/quay/quay-operator/api/v1"
)

var kustomizationForTests = []struct {
	name         string
	quayRegistry *v1.QuayRegistry
	expected     *types.Kustomization
	expectedErr  string
}{
	{
		"InvalidQuayRegistry",
		nil,
		nil,
		"given QuayRegistry should not be nil",
	},
	{
		"AllComponents",
		&v1.QuayRegistry{
			Spec: v1.QuayRegistrySpec{
				ManagedComponents: []v1.ManagedComponent{
					{Kind: "postgres"},
					{Kind: "clair"},
					{Kind: "redis"},
					{Kind: "storage"},
				},
			},
		},
		&types.Kustomization{
			TypeMeta: types.TypeMeta{
				APIVersion: types.KustomizationVersion,
				Kind:       types.KustomizationKind,
			},
			Resources:       []string{},
			Components:      []string{},
			SecretGenerator: []types.SecretArgs{},
		},
		"",
	},
}

func encode(value interface{}) []byte {
	yamlified, _ := yaml.Marshal(value)

	return yamlified
}

func decode(bytes []byte) interface{} {
	var value interface{}
	_ = yaml.Unmarshal(bytes, &value)

	return value
}

func TestKustomizationFor(t *testing.T) {
	assert := assert.New(t)

	for _, test := range kustomizationForTests {
		kustomization, err := KustomizationFor(test.quayRegistry, &corev1.Secret{})

		if test.expectedErr != "" {
			assert.EqualError(err, test.expectedErr)
			assert.Nil(kustomization)
		} else {
			assert.NotNil(kustomization)
		}
	}
}

func TestFlattenSecret(t *testing.T) {
	assert := assert.New(t)

	config := map[string]interface{}{
		"ENTERPRISE_LOGO_URL": "/static/img/quay-horizontal-color.svg",
		"FEATURE_SUPER_USERS": true,
		"SERVER_HOSTNAME":     "quay-app.quay-enterprise",
	}

	secret := &corev1.Secret{
		Data: map[string][]byte{
			"config.yaml":                  encode(config),
			"FEATURE_SECURITY_SCANNER":     encode(true),
			"SECURITY_SCANNER_V4_ENDPOINT": encode("http://clair"),
			"ssl.key":                      encode("abcd1234"),
		},
	}

	flattenedSecret, err := flattenSecret(secret)

	assert.Nil(err)
	assert.Equal(2, len(flattenedSecret.Data))
	assert.NotNil(flattenedSecret.Data["config.yaml"])

	flattenedConfig := decode(flattenedSecret.Data["config.yaml"])
	for key, value := range config {
		assert.Equal(value, flattenedConfig.(map[string]interface{})[key])
	}
	assert.Equal(decode(secret.Data["FEATURE_SECURITY_SCANNER"]), flattenedConfig.(map[string]interface{})["FEATURE_SECURITY_SCANNER"])
	assert.Equal(decode(secret.Data["SECURITY_SCANNER_V4_ENDPOINT"]), flattenedConfig.(map[string]interface{})["SECURITY_SCANNER_V4_ENDPOINT"])
}

var quayComponents = map[string][]runtime.Object{
	"base": {
		&rbac.Role{ObjectMeta: metav1.ObjectMeta{Name: "quay-serviceaccount"}},
		&rbac.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "quay-secret-writer"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "quay-app"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "quay-app"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "quay-config-secret"}},
	},
	"clair": {
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "clair-config-secret"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "clair"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "clair"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "clair-postgres"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "clair-postgres"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "clair-postgres"}},
	},
	"postgres": {
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "postgres-bootstrap"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "quay-postgres"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "quay-postgres"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "quay-postgres"}},
	},
	"redis": {
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "quay-redis"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "quay-redis"}},
	},
	"storage": {
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "quay-storage"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "quay-datastore"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "quay-datastore"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "minio-pv-claim"}},
	},
}

func withComponents(components []string) []runtime.Object {
	selectedComponents := []runtime.Object{}
	for _, component := range components {
		selectedComponents = append(selectedComponents, quayComponents[component]...)
	}

	return selectedComponents
}

var inflateTests = []struct {
	name         string
	quayRegistry *v1.QuayRegistry
	configBundle *corev1.Secret
	expected     []runtime.Object
	expectedErr  error
}{
	{
		"AllComponents",
		&v1.QuayRegistry{
			Spec: v1.QuayRegistrySpec{
				ManagedComponents: []v1.ManagedComponent{
					{Kind: "postgres"},
					{Kind: "clair"},
					{Kind: "redis"},
					{Kind: "storage"},
				},
			},
		},
		&corev1.Secret{
			Data: map[string][]byte{
				"config.yaml": encode(map[string]interface{}{"SERVER_HOSTNAME": "quay.io"}),
			},
		},
		withComponents([]string{"base", "clair", "postgres", "redis", "storage"}),
		nil,
	},
	{
		"OnlyBaseComponent",
		&v1.QuayRegistry{},
		&corev1.Secret{
			Data: map[string][]byte{
				"config.yaml": encode(map[string]interface{}{"SERVER_HOSTNAME": "quay.io"}),
			},
		},
		withComponents([]string{"base"}),
		nil,
	},
}

func TestInflate(t *testing.T) {
	assert := assert.New(t)

	for _, test := range inflateTests {
		pieces, err := Inflate(test.quayRegistry, test.configBundle)

		assert.NotNil(pieces)
		assert.Equal(len(test.expected), len(pieces))
		assert.Nil(err)

		for _, obj := range pieces {
			objectMeta, _ := meta.Accessor(obj)

			assert.Contains(objectMeta.GetName(), test.quayRegistry.GetName()+"-")
		}
	}
}
