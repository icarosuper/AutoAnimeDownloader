package daemon

import "errors"

// Causas de um passe abortado. Sao CODIGOS que o frontend traduz — a mesma fronteira que
// lib/domain/checkIssue.ts ja defende: o backend manda codigo, o frontend monta a frase. Antes
// disso o banner da tela de Status exibia o `error.Error()` cru, o que na pratica significava
// despejar um JSON de resposta da AniList na cara do usuario.
const (
	PassErrConfig  = "config"          // nao deu para ler ou gravar a configuracao
	PassErrSetup   = "setup"           // falta configurar (pasta da biblioteca)
	PassErrLibrary = "library"         // a pasta da biblioteca nao aceita hardlink
	PassErrTorrent = "torrent_backend" // o cliente de torrent nao subiu
	PassErrAnilist = "anilist"         // a AniList nao respondeu
	PassErrStorage = "storage"         // dados do proprio app (episodes.json, migracoes)
	PassErrUnknown = "unknown"
)

// Sentinelas embrulhadas no erro do sitio de aborto. O codigo viaja DENTRO do erro em vez de num
// parametro novo de SetLastCheckError: sao ~15 chamadas hoje, a maioria com nil para limpar, e
// nenhuma delas precisaria mudar. Um sitio novo que esqueca de embrulhar cai em PassErrUnknown,
// que tem frase propria — degrada, nao quebra.
var (
	errCauseConfig  = errors.New("configuração")
	errCauseSetup   = errors.New("configuração incompleta")
	errCauseLibrary = errors.New("biblioteca")
	errCauseTorrent = errors.New("cliente de torrent")
	errCauseAnilist = errors.New("anilist")
	errCauseStorage = errors.New("dados do app")
)

// passErrorCode traduz o erro de um passe abortado no codigo que o frontend entende.
func passErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errCauseConfig):
		return PassErrConfig
	case errors.Is(err, errCauseSetup):
		return PassErrSetup
	case errors.Is(err, errCauseLibrary):
		return PassErrLibrary
	case errors.Is(err, errCauseTorrent):
		return PassErrTorrent
	case errors.Is(err, errCauseAnilist):
		return PassErrAnilist
	case errors.Is(err, errCauseStorage):
		return PassErrStorage
	default:
		return PassErrUnknown
	}
}
