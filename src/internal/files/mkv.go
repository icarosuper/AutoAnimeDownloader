package files

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// errBadEBML marca um byte de largura variável inválido. Nunca vaza para fora do pacote: todo
// caminho de erro daqui vira "" em VideoCodec.
var errBadEBML = errors.New("invalid EBML variable-length integer")

// Leitura do codec de vídeo direto do arquivo, e não do nome dele.
//
// Por que existe: `nyaa.extractCodec` lê o codec do NOME do release, e boa parte dos uploads
// simplesmente não escreve — o Erai-raws não escreve, muito BD rip não escreve. O único lugar
// onde o codec sempre está é o cabeçalho do próprio arquivo, declarado por quem encodou.
//
// Matroska (`.mkv`) é um documento EBML (RFC 8794): uma árvore de elementos `ID + tamanho +
// conteúdo`, com tamanhos de largura variável — um XML binário. O CodecID vive num caminho
// fixo e CURTO, antes dos Clusters (o vídeo em si):
//
//	Segment → Tracks → TrackEntry → { TrackType (1 = vídeo), CodecID }
//
// Por isso a leitura custa alguns KB e alguns seeks, não o arquivo inteiro.
//
// Só Matroska: MP4 tem estrutura análoga (boxes) mas exigiria um segundo parser, e em anime é
// praticamente tudo mkv — quem não for casa em `""`, que a UI mostra como nada.

// IDs dos elementos EBML/Matroska, escritos COM os bits de marcação de tamanho, que é como a
// especificação da Matroska os tabela.
const (
	idEBMLHeader = 0x1A45DFA3
	idSegment    = 0x18538067
	idTracks     = 0x1654AE6B
	idCluster    = 0x1F43B675
	idTrackEntry = 0xAE
	idTrackType  = 0x83
	idCodecID    = 0x86

	trackTypeVideo = 1
	// sizeUnknown é o tamanho "desconhecido" do EBML (todos os bits de valor em 1). Um Segment
	// escrito ao vivo usa isso; descer nele é válido, pular não.
	sizeUnknown = ^uint64(0)
)

// mkvCodecLabels traduz os CodecID da Matroska para o MESMO vocabulário que
// `nyaa.extractCodec` usa no resto do app ("HEVC", "H.264", "AV1"), senão a tela de prioridades
// e este painel falariam de codec com nomes diferentes. O que não estiver no mapa sai como o
// CodecID sem o prefixo `V_` — feio, mas honesto: melhor do que chutar um rótulo bonito errado.
var mkvCodecLabels = map[string]string{
	"V_MPEGH/ISO/HEVC": "HEVC",
	"V_MPEG4/ISO/AVC":  "H.264",
	"V_AV1":            "AV1",
	"V_VP9":            "VP9",
	"V_VP8":            "VP8",
	"V_MPEG4/ISO/ASP":  "MPEG-4",
	"V_MPEG2":          "MPEG-2",
}

// VideoCodec devolve o codec da primeira trilha de vídeo de um `.mkv`, no vocabulário de
// `nyaa.extractCodec`. Devolve "" — nunca erro — para qualquer coisa que não dê para responder:
// arquivo que não abre, que não é Matroska, que ainda não tem o cabeçalho no disco, ou EBML que
// não bate. É dado de exibição: não saber é um estado normal, não uma falha.
func VideoCodec(path string) string {
	if !strings.EqualFold(filepath.Ext(path), ".mkv") {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// O cabeçalho EBML só diz "isto é EBML, versão X, DocType matroska" — serve de assinatura
	// e é pulado inteiro; o que interessa está no Segment.
	id, size, err := readElementHeader(f)
	if err != nil || id != idEBMLHeader || size == sizeUnknown {
		return ""
	}
	if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
		return ""
	}

	if _, ok := seekToChild(f, idSegment); !ok {
		return ""
	}
	tracksSize, ok := seekToChild(f, idTracks)
	if !ok {
		return ""
	}
	// LimitReader pelo tamanho declarado de Tracks: sem ele a varredura passaria do fim de
	// Tracks e sairia lendo Cluster como se fosse TrackEntry.
	return videoCodecFromTracks(io.LimitReader(f, int64(tracksSize)))
}

// seekToChild pula elementos irmãos até achar `want`, deixando o arquivo posicionado no
// CONTEÚDO dele. Para em Cluster: o vídeo começou e Tracks não vem depois disso.
func seekToChild(f *os.File, want uint32) (uint64, bool) {
	for {
		id, size, err := readElementHeader(f)
		if err != nil {
			return 0, false
		}
		if id == want {
			return size, size != sizeUnknown
		}
		if id == idCluster || size == sizeUnknown {
			// Cluster é o vídeo em si: Tracks vem antes, então passar daqui é desistir.
			// Tamanho desconhecido em algo que não é o alvo não dá para pular sem parsear o
			// conteúdo inteiro, e não vale — "não sei" também é uma resposta.
			return 0, false
		}
		if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
			return 0, false
		}
	}
}

// videoCodecFromTracks varre os TrackEntry e devolve o CodecID do primeiro que se declara
// vídeo. Um mkv de anime tem vídeo, áudio e várias legendas — pegar o primeiro TrackEntry sem
// olhar o TrackType devolveria o codec de áudio em arquivo com a ordem trocada.
func videoCodecFromTracks(r io.Reader) string {
	for {
		id, size, err := readElementHeader(r)
		if err != nil || size == sizeUnknown {
			return ""
		}
		if id != idTrackEntry {
			if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
				return ""
			}
			continue
		}

		entry := io.LimitReader(r, int64(size))
		codec, isVideo := parseTrackEntry(entry)
		// Drena o resto da entry para o próximo irmão começar no lugar certo, mesmo quando
		// parseTrackEntry parou cedo.
		if _, err := io.Copy(io.Discard, entry); err != nil {
			return ""
		}
		if isVideo && codec != "" {
			if label, ok := mkvCodecLabels[codec]; ok {
				return label
			}
			return strings.TrimPrefix(codec, "V_")
		}
	}
}

// parseTrackEntry lê TrackType e CodecID de dentro de UM TrackEntry.
func parseTrackEntry(r io.Reader) (codec string, isVideo bool) {
	for {
		id, size, err := readElementHeader(r)
		if err != nil || size == sizeUnknown {
			return codec, isVideo
		}
		switch id {
		case idTrackType:
			n, err := readUint(r, size)
			if err != nil {
				return codec, isVideo
			}
			isVideo = n == trackTypeVideo
		case idCodecID:
			b := make([]byte, size)
			if _, err := io.ReadFull(r, b); err != nil {
				return codec, isVideo
			}
			// CodecID é string ASCII, mas a especificação permite padding com NUL à direita.
			codec = strings.TrimRight(string(b), "\x00")
		default:
			if _, err := io.CopyN(io.Discard, r, int64(size)); err != nil {
				return codec, isVideo
			}
		}
	}
}

// readElementHeader lê o par (ID, tamanho) que abre todo elemento EBML.
func readElementHeader(r io.Reader) (uint32, uint64, error) {
	id, err := readID(r)
	if err != nil {
		return 0, 0, err
	}
	size, err := readSize(r)
	if err != nil {
		return 0, 0, err
	}
	return id, size, nil
}

// vintLen devolve o comprimento em bytes de um inteiro de largura variável do EBML, lido da
// posição do primeiro bit 1 do primeiro byte. 0 significa byte inválido (0x00).
func vintLen(b byte) int {
	for i := 0; i < 8; i++ {
		if b&(0x80>>i) != 0 {
			return i + 1
		}
	}
	return 0
}

// readID lê um ID de elemento (1 a 4 bytes), devolvido COM os bits de marcação — é assim que
// a especificação da Matroska tabela os IDs, e é o que as constantes acima esperam.
func readID(r io.Reader) (uint32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return 0, err
	}
	n := vintLen(b[0])
	if n == 0 || n > 4 {
		return 0, errBadEBML
	}
	if n > 1 {
		if _, err := io.ReadFull(r, b[1:n]); err != nil {
			return 0, err
		}
	}
	var id uint32
	for i := 0; i < n; i++ {
		id = id<<8 | uint32(b[i])
	}
	return id, nil
}

// readSize lê o tamanho de um elemento (1 a 8 bytes), já SEM o bit de marcação.
func readSize(r io.Reader) (uint64, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:1]); err != nil {
		return 0, err
	}
	n := vintLen(b[0])
	if n == 0 {
		return 0, errBadEBML
	}
	if n > 1 {
		if _, err := io.ReadFull(r, b[1:n]); err != nil {
			return 0, err
		}
	}
	value := uint64(b[0] &^ (0x80 >> (n - 1)))
	for i := 1; i < n; i++ {
		value = value<<8 | uint64(b[i])
	}
	// Todos os bits de valor em 1 = tamanho desconhecido.
	if value == (uint64(1)<<(7*n))-1 {
		return sizeUnknown, nil
	}
	return value, nil
}

// readUint lê um inteiro EBML de `size` bytes (big-endian, sem sinal).
func readUint(r io.Reader, size uint64) (uint64, error) {
	if size > 8 {
		return 0, errBadEBML
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return 0, err
	}
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v, nil
}
