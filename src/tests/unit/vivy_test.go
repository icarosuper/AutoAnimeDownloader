package unit_test

import (
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/nyaa"
)

// TestVivyTitleVariants testa a geração de variantes de busca para o caso do Vivy
// que tem caracteres especiais tanto no título em inglês quanto no romaji
func TestVivyTitleVariants(t *testing.T) {
	romaji := "Vivy: Fluorite Eye's Song"
	english := "Vivy -Fluorite Eye's Song-"

	variants := nyaa.GenerateSearchTitleVariants(romaji, english)

	// Deve gerar pelo menos 2 variantes (romaji limpo + romaji original)
	// Pode gerar até 4 se english != romaji
	if len(variants) < 2 {
		t.Errorf("Expected at least 2 variants, got %d", len(variants))
	}

	// A primeira variante deve ser o romaji limpo (sem caracteres especiais)
	firstVariant := variants[0]
	expectedFirst := "vivy fluorite eyes song"
	if firstVariant != expectedFirst {
		t.Errorf("First variant should be cleaned romaji.\nExpected: %s\nGot: %s", expectedFirst, firstVariant)
	}

	// Verifica que todas as variantes são únicas
	seen := make(map[string]bool)
	for _, v := range variants {
		if seen[v] {
			t.Errorf("Duplicate variant found: %s", v)
		}
		seen[v] = true
	}

	// Verifica que pelo menos uma variante não tem caracteres especiais
	hasCleanVariant := false
	for _, v := range variants {
		// Verifica se não tem :, ', ou - (caracteres especiais do título original)
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

// TestGenerateSearchTitleVariants_Ordering testa que as variantes são geradas na ordem correta
func TestGenerateSearchTitleVariants_Ordering(t *testing.T) {
	romaji := "Test Anime: Special"
	english := "Test Anime - Special"

	variants := nyaa.GenerateSearchTitleVariants(romaji, english)

	// A primeira variante deve ser sempre o romaji limpo (sem : ou -)
	firstVariant := variants[0]
	if strings.ContainsAny(firstVariant, ":-") {
		t.Errorf("First variant should be cleaned romaji (no special chars), got: %s", firstVariant)
	}

	// A segunda variante deve ser o romaji original
	if variants[1] != romaji {
		t.Errorf("Second variant should be original romaji, got: %s", variants[1])
	}

	// Como English != Romaji, deve ter pelo menos 3 variantes
	// Mas pode ter apenas 3 se English limpo == Romaji limpo (são removidas duplicatas)
	if len(variants) >= 3 {
		// A terceira variante deve ser o English original (ou limpo se for diferente)
		thirdVariant := variants[2]
		if thirdVariant != english {
			// Se não é o English original, verifica se é uma versão limpa válida
			if strings.ContainsAny(thirdVariant, ":-") {
				t.Errorf("Third variant should be cleaned english or english original, got: %s", thirdVariant)
			}
		}
	}
}

// TestGenerateSearchTitleVariants_DuplicateHandling testa que variantes duplicadas são removidas
func TestGenerateSearchTitleVariants_DuplicateHandling(t *testing.T) {
	// Caso onde romaji == english (após limpeza podem ser iguais)
	romaji := "Test Anime"
	english := "Test Anime"

	variants := nyaa.GenerateSearchTitleVariants(romaji, english)

	// Não deve ter duplicatas
	seen := make(map[string]bool)
	for _, v := range variants {
		if seen[v] {
			t.Errorf("Found duplicate variant: %s", v)
		}
		seen[v] = true
	}

	// Deve ter no máximo 2 variantes (romaji limpo + romaji original)
	if len(variants) > 2 {
		t.Errorf("Expected at most 2 variants when romaji == english, got %d", len(variants))
	}
}
