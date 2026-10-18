package unit_test

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/nyaa"
)

// TestVivyTitleVariants tests search-variant generation for the Vivy case,
// which has special characters in both the English title and the romaji.
func TestVivyTitleVariants(t *testing.T) {
	romaji := "Vivy: Fluorite Eye's Song"
	english := "Vivy -Fluorite Eye's Song-"

	variants := nyaa.GenerateSearchTitleVariants(romaji, english)

	// Must generate at least 2 variants (cleaned romaji + original romaji)
	// May generate up to 4 if english != romaji
	if len(variants) < 2 {
		t.Errorf("Expected at least 2 variants, got %d", len(variants))
	}

	// The first variant must be the cleaned romaji (no special characters)
	firstVariant := variants[0]
	expectedFirst := "vivy fluorite eyes song"
	if firstVariant != expectedFirst {
		t.Errorf("First variant should be cleaned romaji.\nExpected: %s\nGot: %s", expectedFirst, firstVariant)
	}

	// Check that every variant is unique
	seen := make(map[string]bool)
	for _, v := range variants {
		if seen[v] {
			t.Errorf("Duplicate variant found: %s", v)
		}
		seen[v] = true
	}

	// Check that at least one variant has no special characters
	hasCleanVariant := false
	for _, v := range variants {
		// Check it has no :, ', or - (special characters from the original title)
		if !strings.ContainsAny(v, ":'-") {
			hasCleanVariant = true
			break
		}
	}
	if !hasCleanVariant {
		t.Error("Expected at least one variant without special characters")
	}

	t.Logf("Generated %d variants for Vivy:", len(variants))
	for i, v := range variants {
		t.Logf("%d. %s", i+1, v)
	}
}

// TestGenerateSearchTitleVariants_Ordering tests that variants are generated in the correct order
func TestGenerateSearchTitleVariants_Ordering(t *testing.T) {
	romaji := "Test Anime: Special"
	english := "Test Anime - Special"

	variants := nyaa.GenerateSearchTitleVariants(romaji, english)

	// The first variant must always be the cleaned romaji (no : or -)
	firstVariant := variants[0]
	if strings.ContainsAny(firstVariant, ":-") {
		t.Errorf("First variant should be cleaned romaji (no special chars), got: %s", firstVariant)
	}

	// The second variant must be the original romaji
	if variants[1] != romaji {
		t.Errorf("Second variant should be original romaji, got: %s", variants[1])
	}

	// Because English != Romaji, there must be at least 3 variants
	// But there may be only 3 if cleaned English == cleaned Romaji (duplicates are removed)
	if len(variants) >= 3 {
		// The third variant must be the original English (or cleaned if that is different)
		thirdVariant := variants[2]
		if thirdVariant != english {
			// If it is not the original English, check it is a valid cleaned version
			if strings.ContainsAny(thirdVariant, ":-") {
				t.Errorf("Third variant should be cleaned english or english original, got: %s", thirdVariant)
			}
		}
	}
}

// TestGenerateSearchTitleVariants_DuplicateHandling tests that duplicate variants are removed
func TestGenerateSearchTitleVariants_DuplicateHandling(t *testing.T) {
	// Case where romaji == english (after cleaning they may be equal)
	romaji := "Test Anime"
	english := "Test Anime"

	variants := nyaa.GenerateSearchTitleVariants(romaji, english)

	// Must not have duplicates
	seen := make(map[string]bool)
	for _, v := range variants {
		if seen[v] {
			t.Errorf("Found duplicate variant: %s", v)
		}
		seen[v] = true
	}

	// Must have at most 2 variants (cleaned romaji + original romaji)
	if len(variants) > 2 {
		t.Errorf("Expected at most 2 variants when romaji == english, got %d", len(variants))
	}
}
