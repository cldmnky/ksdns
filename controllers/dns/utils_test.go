package dns

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestCreateOrUpdateWithRetries(t *testing.T) {
	// Create a fake client to use in the test
	c := fake.NewClientBuilder().Build()

	// Create a sample object to use as the "obj" argument in the function
	obj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-object",
			UID:  "123456",
		},
	}

	// Define test cases
	testCases := []struct {
		name           string
		mutateFn       controllerutil.MutateFn
		expectedError  error
		expectedResult controllerutil.OperationResult
	}{
		{
			name: "Successful operation",
			mutateFn: func() error {
				// Set some field in the object
				obj.Labels = map[string]string{
					"test-label": "test-value",
				}
				return nil
			},
			expectedError:  nil,
			expectedResult: controllerutil.OperationResultCreated,
		},
		{
			name: "Failed operation",
			mutateFn: func() error {
				// Return a timeout error
				return apierrors.NewTimeoutError("timed out waiting for the condition", 0)
			},
			expectedError:  apierrors.NewTimeoutError("timed out waiting for the condition", 0),
			expectedResult: controllerutil.OperationResultNone,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operationResult, err := CreateOrUpdateWithRetries(context.TODO(), c, obj, tc.mutateFn)
			assert.Equal(t, tc.expectedError, err)
			assert.Equal(t, tc.expectedResult, operationResult)
		})
	}
}
