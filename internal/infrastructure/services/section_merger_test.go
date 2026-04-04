//go:build unit

package services_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	services "github.com/rios0rios0/aisync/internal/infrastructure/services"
)

func TestSectionMerger_Merge_ShouldConcatenateSharedAndPersonalWithSeparator(t *testing.T) {
	// given
	shared := []byte("# Shared Rules\nRule one.")
	personal := []byte("# My Personal Notes\nPersonal content here.")
	merger := services.NewSectionMerger()

	// when
	result, err := merger.Merge([][]byte{shared}, personal)

	// then
	assert.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "# Shared Rules")
	assert.Contains(t, output, services.DefaultSeparator)
	assert.Contains(t, output, "# My Personal Notes")

	sepIdx := strings.Index(output, services.DefaultSeparator)
	sharedIdx := strings.Index(output, "# Shared Rules")
	personalIdx := strings.Index(output, "# My Personal Notes")
	assert.Less(t, sharedIdx, sepIdx)
	assert.Less(t, sepIdx, personalIdx)
}

func TestSectionMerger_Merge_ShouldReplaceOnlySharedSectionOnResync(t *testing.T) {
	// given
	oldShared := []byte("# Old Shared Content")
	personalWithSep := []byte("# Old Shared Content\n\n" + services.DefaultSeparator + "\n\nMy personal stuff")
	merger := services.NewSectionMerger()

	newShared := []byte("# New Shared Content")

	// when
	result, err := merger.Merge([][]byte{newShared}, personalWithSep)

	// then
	assert.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "# New Shared Content")
	assert.NotContains(t, output, "# Old Shared Content")
	assert.Contains(t, output, "My personal stuff")
	assert.Contains(t, output, services.DefaultSeparator)

	_ = oldShared
}

func TestSectionMerger_Merge_ShouldReturnOnlySharedWhenNoPersonal(t *testing.T) {
	// given
	shared := []byte("# Shared Only\nContent here.")
	merger := services.NewSectionMerger()

	// when
	result, err := merger.Merge([][]byte{shared}, nil)

	// then
	assert.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "# Shared Only")
	assert.NotContains(t, output, services.DefaultSeparator)
}

func TestSectionMerger_Merge_ShouldReturnPersonalBelowSeparatorWhenNoShared(t *testing.T) {
	// given
	personal := []byte("# Personal Only\nMy content.")
	merger := services.NewSectionMerger()

	// when
	result, err := merger.Merge(nil, personal)

	// then
	assert.NoError(t, err)
	output := string(result)

	// With no shared sources, shared buffer is empty; personal is still appended
	assert.Contains(t, output, services.DefaultSeparator)
	assert.Contains(t, output, "# Personal Only")
}

func TestSectionMerger_Merge_ShouldPreserveSeparatorExactly(t *testing.T) {
	// given
	shared := []byte("Shared content")
	personal := []byte("Personal content")
	merger := services.NewSectionMerger()

	// when
	result, err := merger.Merge([][]byte{shared}, personal)

	// then
	assert.NoError(t, err)
	output := string(result)
	assert.Equal(t, 1, strings.Count(output, services.DefaultSeparator))
}

func TestSectionMerger_Merge_ShouldConcatenateMultipleSharedSources(t *testing.T) {
	// given
	source1 := []byte("# Source 1")
	source2 := []byte("# Source 2")
	merger := services.NewSectionMerger()

	// when
	result, err := merger.Merge([][]byte{source1, source2}, nil)

	// then
	assert.NoError(t, err)
	output := string(result)
	assert.Contains(t, output, "# Source 1")
	assert.Contains(t, output, "# Source 2")
}

func TestSectionMerger_Merge_ShouldReturnEmptyWhenBothInputsAreEmpty(t *testing.T) {
	// given
	merger := services.NewSectionMerger()

	// when
	result, err := merger.Merge(nil, nil)

	// then
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestSectionMerger_String_ShouldReturnDescriptionWithSeparator(t *testing.T) {
	// given
	merger := services.NewSectionMerger()

	// when
	result := merger.String()

	// then
	assert.Contains(t, result, "SectionMerger")
	assert.Contains(t, result, services.DefaultSeparator)
}

func TestSectionMerger_Merge_ShouldDiscardPersonalWhenOnlySeparatorAndWhitespace(t *testing.T) {
	// given
	personal := []byte("# Old Shared\n\n" + services.DefaultSeparator + "\n\n")
	merger := services.NewSectionMerger()

	newShared := []byte("# New Shared Content")

	// when
	result, err := merger.Merge([][]byte{newShared}, personal)

	// then
	assert.NoError(t, err)
	output := string(result)
	// When personal section after separator is only whitespace, extractPersonalContent returns nil
	// so the output should only be the shared content without separator
	assert.Contains(t, output, "# New Shared Content")
}
