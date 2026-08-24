package daemon

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestPassErrorCode trava a classificacao das causas de aborto. E o que decide a frase que o
// usuario le no banner da Status: uma causa mal classificada manda ele conferir a pasta da
// biblioteca quando quem caiu foi a AniList.
func TestPassErrorCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"sem erro", nil, ""},
		{"anilist embrulhado", fmt.Errorf("%w: %w", errCauseAnilist, errors.New("anilist returned 403: disabled")), PassErrAnilist},
		{"configuracao incompleta", fmt.Errorf("%w: falta a pasta", errCauseSetup), PassErrSetup},
		{"biblioteca sem hardlink", fmt.Errorf("%w: %w", errCauseLibrary, errors.New("cross-device link")), PassErrLibrary},
		{"cliente de torrent", fmt.Errorf("%w: não inicializado", errCauseTorrent), PassErrTorrent},
		{"dados do app", fmt.Errorf("%w: %w", errCauseStorage, errors.New("episodes.json corrompido")), PassErrStorage},
		{"config ilegivel", fmt.Errorf("%w: %w", errCauseConfig, errors.New("permission denied")), PassErrConfig},
		// Um sitio de aborto novo que esqueca de embrulhar tem de cair aqui, e nao virar uma
		// causa errada: "unknown" tem frase propria no frontend.
		{"erro nao embrulhado", errors.New("qualquer coisa"), PassErrUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := passErrorCode(tc.err); got != tc.want {
				t.Errorf("passErrorCode = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPassErrorKeepsOriginalMessage: embrulhar nao pode engolir o texto original — ele continua
// indo para o `pass_error` cru, que a tela mostra recolhido para quem for abrir uma issue.
func TestPassErrorKeepsOriginalMessage(t *testing.T) {
	err := fmt.Errorf("%w: %w", errCauseAnilist, errors.New("anilist returned 403: temporarily disabled"))
	if got := err.Error(); got == "" || !errors.Is(err, errCauseAnilist) {
		t.Fatalf("erro embrulhado perdeu informação: %q", got)
	}
	if want := "temporarily disabled"; !strings.Contains(err.Error(), want) {
		t.Errorf("mensagem = %q, esperava conter %q", err.Error(), want)
	}
}
