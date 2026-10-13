package pythonconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNonEmptyVisibilityLabels(t *testing.T) {
	assert.Equal(t, []string{"//a:b", "//c:d"}, nonEmptyVisibilityLabels([]string{
		"//a:b",
		"",
		" //c:d ",
		"",
	}))
	assert.Nil(t, nonEmptyVisibilityLabels(nil))
	assert.Empty(t, nonEmptyVisibilityLabels([]string{"", "  "}))
}

func TestSetDefaultVisibilityDropsEmptyLabels(t *testing.T) {
	cfg := New("", "")
	cfg.SetDefaultVisibility([]string{"//benchling:__subpackages__", ""})
	assert.Equal(t, []string{"//benchling:__subpackages__"}, cfg.Visibility())
}
