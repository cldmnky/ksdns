package dns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreDNSTsigTemplate(t *testing.T) {
	s, err := renderCoreFileTsigSecret("foo", "bar")
	require.NoError(t, err)
	expected := `key "foo" {
  secret "bar";
};
`
	assert.Equal(t, s, expected)
}
