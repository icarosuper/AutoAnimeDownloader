package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"AutoAnimeDownloader/src/internal/files"
)

// configFields devolve a config default no mesmo formato que o handleConfigSet manipula: o mapa
// cru do JSON. Ele e a fonte da verdade dos dois testes abaixo — nenhuma lista de campo escrita
// a mao, entao um campo novo em files.Config entra aqui de graca.
func configFields(t *testing.T) map[string]any {
	t.Helper()
	raw, err := json.Marshal(files.Config{AnilistUsernames: []string{"a"}})
	if err != nil {
		t.Fatalf("nao consegui serializar a config: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("nao consegui desserializar a config: %v", err)
	}
	return fields
}

// O switch antigo cobria 7 chaves e respondia "unknown config key" para o resto. O passthrough tem
// de aceitar TODA chave que a config serializa — e este teste quebra se alguem reintroduzir uma
// lista fixa.
func TestMatchConfigKey_ResolveTodaChaveDaConfig(t *testing.T) {
	fields := configFields(t)
	if len(fields) < 20 {
		t.Fatalf("esperava a config inteira, vieram %d chaves", len(fields))
	}
	for name := range fields {
		if got, ok := matchConfigKey(fields, name); !ok || got != name {
			t.Errorf("chave %q nao resolveu (got=%q ok=%v)", name, got, ok)
		}
	}
}

func TestMatchConfigKey_IgnoraCaixaEUnderscore(t *testing.T) {
	fields := configFields(t)
	for _, typed := range []string{"max_search_pages", "maxSearchPages", "MAX_SEARCH_PAGES", "MaxSearchPages"} {
		if got, ok := matchConfigKey(fields, typed); !ok || got != "max_search_pages" {
			t.Errorf("%q deveria resolver para max_search_pages, veio (%q, %v)", typed, got, ok)
		}
	}
	if _, ok := matchConfigKey(fields, "nao_existe"); ok {
		t.Error("chave inexistente deveria falhar em vez de casar com alguma coisa")
	}
}

func TestParseConfigValue(t *testing.T) {
	tests := []struct {
		name    string
		current any
		value   string
		want    any
	}{
		// O valor salvo e o oraculo de tipo: so quem ja e lista aceita a forma separada por virgula.
		{"lista por virgula", []any{"a"}, "x, y ,z", []any{"x", "y", "z"}},
		{"lista descarta vazio", []any{"a"}, "x,,y", []any{"x", "y"}},
		{"lista em JSON explicito", []any{"a"}, `["x","y"]`, []any{"x", "y"}},
		{"int", float64(5), "12", float64(12)},
		{"bool", false, "true", true},
		{"float", float64(1), "2.5", 2.5},
		// String com virgula NAO vira lista quando o campo nao e lista — era o unico jeito de o
		// passthrough estragar um caminho de biblioteca.
		{"string com virgula", "", "/home/eu/Animes, Series", "/home/eu/Animes, Series"},
		{"string simples", "", "/home/eu/Animes", "/home/eu/Animes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseConfigValue(tt.current, tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseConfigValue(%v, %q) = %#v, queria %#v", tt.current, tt.value, got, tt.want)
			}
		})
	}
}
