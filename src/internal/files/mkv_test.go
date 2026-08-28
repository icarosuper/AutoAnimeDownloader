package files

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// --- construção de um .mkv mínimo, para o teste não depender de arquivo binário no repo ---

// ebmlID escreve um ID já com os bits de marcação (as constantes do mkv.go usam essa forma).
func ebmlID(id uint32) []byte {
	switch {
	case id <= 0xFF:
		return []byte{byte(id)}
	case id <= 0xFFFF:
		return []byte{byte(id >> 8), byte(id)}
	case id <= 0xFFFFFF:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	default:
		return []byte{byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id)}
	}
}

// ebmlSize escreve um tamanho em 4 bytes (marcador 0b0001xxxx). Largura fixa de propósito: o
// que este teste exercita é a navegação, não o empacotamento mais compacto possível.
func ebmlSize(n uint64) []byte {
	return []byte{0x10 | byte(n>>24), byte(n >> 16), byte(n >> 8), byte(n)}
}

func elem(id uint32, payload []byte) []byte {
	out := append([]byte{}, ebmlID(id)...)
	out = append(out, ebmlSize(uint64(len(payload)))...)
	return append(out, payload...)
}

// buildMKV monta um Matroska com um cabeçalho EBML, um elemento gordo antes de Tracks (para o
// pulo por tamanho ser realmente exercitado) e as trilhas na ordem pedida.
func buildMKV(tracks ...[]byte) []byte {
	var segment []byte
	// Um "SeekHead" de 100 KB que o parser tem que pular pelo tamanho declarado, sem lê-lo.
	segment = append(segment, elem(0x114D9B74, bytes.Repeat([]byte{0xAA}, 100*1024))...)
	var trackList []byte
	for _, t := range tracks {
		trackList = append(trackList, t...)
	}
	segment = append(segment, elem(idTracks, trackList)...)
	// Cluster depois de Tracks, como num arquivo real.
	segment = append(segment, elem(idCluster, bytes.Repeat([]byte{0xBB}, 4096))...)

	var out []byte
	out = append(out, elem(idEBMLHeader, []byte{0x42, 0x82, 0x88, 'm', 'a', 't', 'r', 'o', 's', 'k', 'a'})...)
	return append(out, elem(idSegment, segment)...)
}

func track(trackType byte, codecID string) []byte {
	var body []byte
	body = append(body, elem(idTrackType, []byte{trackType})...)
	body = append(body, elem(idCodecID, []byte(codecID))...)
	// Um elemento desconhecido no meio da entry: o parser tem que pulá-lo, não engasgar.
	body = append(body, elem(0xD7, []byte{0x01})...)
	return elem(idTrackEntry, body)
}

func writeTemp(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestVideoCodecReadsTheCodecIDFromTheHeader(t *testing.T) {
	cases := []struct {
		codecID string
		want    string
	}{
		{"V_MPEGH/ISO/HEVC", "HEVC"},
		{"V_MPEG4/ISO/AVC", "H.264"},
		{"V_AV1", "AV1"},
		// Fora do mapa: sai o CodecID sem o prefixo, não um rótulo bonito chutado.
		{"V_QUICKTIME", "QUICKTIME"},
	}
	for _, tc := range cases {
		path := writeTemp(t, "ep.mkv", buildMKV(track(1, tc.codecID)))
		if got := VideoCodec(path); got != tc.want {
			t.Errorf("VideoCodec(%s) = %q, want %q", tc.codecID, got, tc.want)
		}
	}
}

// Um mkv de anime tem vídeo, áudio e várias legendas. Pegar o primeiro TrackEntry sem olhar o
// TrackType devolveria FLAC como se fosse o codec de vídeo.
func TestVideoCodecSkipsNonVideoTracks(t *testing.T) {
	path := writeTemp(t, "ep.mkv", buildMKV(
		track(2, "A_FLAC"),
		track(17, "S_TEXT/ASS"),
		track(1, "V_MPEGH/ISO/HEVC"),
	))
	if got := VideoCodec(path); got != "HEVC" {
		t.Errorf("VideoCodec = %q, want HEVC", got)
	}
}

// Todo caminho de "não dá para responder" devolve "" — nunca erro, nunca pânico. É dado de
// exibição: não saber é estado normal.
func TestVideoCodecUnknownCasesReturnEmpty(t *testing.T) {
	mkv := buildMKV(track(1, "V_AV1"))

	cases := []struct {
		name string
		path string
	}{
		{"extensão diferente de mkv", writeTemp(t, "ep.mp4", mkv)},
		{"arquivo inexistente", filepath.Join(t.TempDir(), "sumiu.mkv")},
		{"arquivo vazio", writeTemp(t, "vazio.mkv", nil)},
		{"não é EBML", writeTemp(t, "lixo.mkv", bytes.Repeat([]byte{0x00}, 512))},
		// Cabeçalho ainda não baixado: o rain não baixa sequencial, então um arquivo em voo
		// pode ser um buraco de zeros com a assinatura ausente.
		{"truncado antes de Tracks", writeTemp(t, "parcial.mkv", mkv[:64])},
		{"só áudio", writeTemp(t, "audio.mkv", buildMKV(track(2, "A_FLAC")))},
	}
	for _, tc := range cases {
		if got := VideoCodec(tc.path); got != "" {
			t.Errorf("%s: VideoCodec = %q, want \"\"", tc.name, got)
		}
	}
}
